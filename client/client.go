package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
	"zrpc/codec"
	"zrpc/service"
)

type Call struct {
	Seq           uint64
	ServiceMethod string
	Args          interface{}
	Reply         interface{}
	Error         error
	Done          chan *Call
}

func (call *Call) done() {
	call.Done <- call
}

type Client struct {
	cc       codec.Codec
	opt      *service.Option
	sending  sync.Mutex
	header   codec.Header
	mu       sync.Mutex
	seq      uint64
	pending  sync.Map
	closing  bool
	shutdown bool
}

var _ io.Closer = (*Client)(nil)

var ErrClosing = errors.New("connection is closing")

func (client *Client) Close() error {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.closing {
		return ErrClosing
	}
	client.closing = true
	return client.cc.Close()
}

func (client *Client) IsAvailable() bool {
	client.mu.Lock()
	defer client.mu.Unlock()
	return !client.closing && !client.shutdown
}

func (client *Client) registerCall(call *Call) (uint64, error) {
	if !client.IsAvailable() {
		return 0, ErrClosing
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	call.Seq = client.seq
	client.seq++
	client.pending.Store(call.Seq, call)
	return call.Seq, nil
}

func (client *Client) removeCall(seq uint64) *Call {
	value, ok := client.pending.Load(seq)
	if !ok {
		return nil
	}
	client.pending.Delete(seq)
	call, _ := value.(*Call)
	return call
}

func (client *Client) terminateCalls(err error) {
	client.sending.Lock()
	defer client.sending.Unlock()
	client.mu.Lock()
	defer client.mu.Unlock()
	client.shutdown = true
	client.pending.Range(func(_, value interface{}) bool {
		call, _ := value.(*Call)
		call.Error = err
		call.done()
		return true
	})
}

func (client *Client) receive() {
	var err error
	for err == nil {
		var h codec.Header
		if err = client.cc.ReadHeader(&h); err != nil {
			break
		}
		call := client.removeCall(h.Seq)
		switch {
		case call == nil:
			err = client.cc.ReadBody(nil)
		case h.Error != "":
			call.Error = fmt.Errorf(h.Error)
			err = client.cc.ReadHeader(nil)
			call.done()
		default:
			err = client.cc.ReadBody(call.Reply)
			if err != nil {
				call.Error = errors.New("reading body error: " + err.Error())
			}
			call.done()
		}
	}
	client.terminateCalls(err)
}

func NewClient(conn net.Conn, opt *service.Option) (*Client, error) {
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		err := fmt.Errorf("invalid codec type %s", opt.CodecType)
		log.Println("rpc client: codec error:", err)
		return nil, err
	}
	if err := json.NewEncoder(conn).Encode(opt); err != nil {
		log.Println("rpc client: options error:", err)
		_ = conn.Close()
		return nil, err
	}
	return newClientCodec(f(conn), opt), nil
}

func NewHTTPClient(conn net.Conn, option *service.Option) (*Client, error) {
	io.WriteString(conn, fmt.Sprintf("CONNECT %s HTTP/1.0\n\n", service.DefaultRPCPath))
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodConnect})
	if err == nil && resp.StatusCode == http.StatusOK {
		return NewClient(conn, option)
	}
	if err == nil {
		err = errors.New("unexpected HTTP response: " + resp.Status)
	}
	return nil, err
}

func newClientCodec(cc codec.Codec, opt *service.Option) *Client {
	client := &Client{
		seq:     1,
		cc:      cc,
		opt:     opt,
		pending: sync.Map{},
	}
	go client.receive()
	return client
}

func parseOptions(opts ...*service.Option) (*service.Option, error) {
	if len(opts) == 0 || opts[0] == nil {
		return service.DefaultOption, nil
	}
	if len(opts) != 1 {
		return nil, errors.New("the number of options is more than one")
	}
	opt := opts[0]
	opt.MagicNumber = service.DefaultOption.MagicNumber
	if opt.CodecType == "" {
		opt.CodecType = service.DefaultOption.CodecType
	}
	return opt, nil
}

type newClientFunc func(conn net.Conn, opt *service.Option) (*Client, error)

func Dial(network, address string, timeout time.Duration, opts ...*service.Option) (client *Client, err error) {
	var f newClientFunc
	if network == "http" {
		f = NewHTTPClient
		network = "tcp"
	} else {
		f = NewClient
	}
	opt, err := parseOptions(opts...)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialTimeout(network, address, timeout)
	if err != nil {
		return nil, err
	}
	defer func() {
		if client == nil {
			_ = conn.Close()
		}
	}()
	if timeout == 0 {
		return f(conn, opt)
	}
	success := make(chan bool, 1) // 为了避免泄露
	go func() {
		client, err = f(conn, opt)
		success <- true
	}()
	select {
	case <-time.After(timeout):
		return nil, fmt.Errorf("rpc client: connect timeout: expect within %s", timeout)
	case <-success:
		return client, err
	}
}

func (client *Client) send(call *Call) {
	client.sending.Lock()
	defer client.sending.Unlock()
	seq, err := client.registerCall(call)
	if err != nil {
		call.Error = err
		call.done()
		return
	}

	client.header.ServiceMethod = call.ServiceMethod
	client.header.Seq = seq
	client.header.Error = ""

	if err := client.cc.Write(&client.header, call.Args); err != nil {
		call = client.removeCall(seq)
		if call != nil {
			call.Error = err
			call.done()
		}
	}
}

func (client *Client) Go(serviceMethod string, args, reply interface{}, done chan *Call) *Call {
	if done == nil {
		done = make(chan *Call, 1)
	} else if cap(done) == 0 {
		log.Panic("rpc client: done channel is unbuffered")
	}
	call := &Call{
		ServiceMethod: serviceMethod,
		Args:          args,
		Reply:         reply,
		Done:          done,
	}
	go client.send(call)
	return call
}

func (client *Client) Call(serviceMethod string, args, reply interface{}, ctx context.Context) error {
	call := client.Go(serviceMethod, args, reply, make(chan *Call, 1))
	select {
	case <-call.Done:
		return call.Error
	case <-ctx.Done():
		client.removeCall(call.Seq)
		return fmt.Errorf("rpc client: call timeout: " + ctx.Err().Error())
	}
}
