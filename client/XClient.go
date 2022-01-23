package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"zrpc/service"
)

type XClient struct {
	mode           string
	opt            *service.Option
	mu             sync.Mutex
	clients        map[string]*Client
	timeout        time.Duration
	registerClient *http.Client
	registerAddr   string
}

var _ io.Closer = (*XClient)(nil)

func NewXClient(registerAddr string, mode string, opt *service.Option, dialTimeout time.Duration) *XClient {
	return &XClient{
		mode:           mode,
		opt:            opt,
		clients:        make(map[string]*Client),
		timeout:        dialTimeout,
		registerClient: &http.Client{},
		registerAddr:   registerAddr,
	}
}

func (xc *XClient) Close() error {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	for key, client := range xc.clients {
		err := client.Close()
		if err != nil {
			return err
		}
		delete(xc.clients, key)
	}
	return nil
}

func (xc *XClient) dial(rpcAddr string) (*Client, error) {
	xc.mu.Lock()
	defer xc.mu.Unlock()
	client, ok := xc.clients[rpcAddr]
	if ok && !client.IsAvailable() {
		err := client.Close()
		if err != nil {
			return nil, err
		}
		delete(xc.clients, rpcAddr)
		client = nil
	}
	if client == nil {
		var err error
		addr := strings.Split(rpcAddr, "@")
		if len(addr) != 2 {
			err = errors.New("rpc client: wrong rpcAddr")
			return nil, err
		}
		client, err = Dial(addr[0], addr[1], xc.timeout, xc.opt)
		xc.clients[rpcAddr] = client
	}
	return client, nil
}

func (xc *XClient) call(rpcAddr string, serviceMethod string, args, reply interface{}, ctx context.Context) error {
	client, err := xc.dial(rpcAddr)
	if err != nil {
		return err
	}
	return client.Call(serviceMethod, args, reply, ctx)
}

func (xc *XClient) Discover(serviceMethod string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, xc.registerAddr, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Zrpc-Services", serviceMethod)
	req.Header.Set("X-Zrpc-Mode", xc.mode)
	resp, err := xc.registerClient.Do(req)
	if err != nil {
		return "", err
	}
	return resp.Header.Get("X-Zrpc-Servers"), nil
}

func (xc *XClient) Call(serviceMethod string, args, reply interface{}, timeout time.Duration) error {
	rpcAddr, err := xc.Discover(serviceMethod)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if timeout != 0 {
		ctx, _ = context.WithTimeout(context.Background(), timeout)
	}
	return xc.call(rpcAddr, serviceMethod, args, reply, ctx)
}

//func (xc *XClient) Broadcast(serviceMethod string, args, reply interface{}, timeout time.Duration) error {
//	servers, err := xc.d.GetAll()
//	if err != nil {
//		return err
//	}
//	var wg sync.WaitGroup
//	var mu sync.Mutex
//	var e error
//	replyDone := reply == nil
//	ctx, cancel := context.WithCancel(context.Background())
//	for _, rpcAddr := range servers {
//		wg.Add(1)
//		go func(rpcAddr string) {
//			defer wg.Done()
//			var curReply interface{}
//			if reply != nil {
//				curReply = reflect.New(reflect.ValueOf(reply).Elem().Type()).Interface()
//			}
//			childCtx, _ := context.WithCancel(ctx)
//			if timeout != 0 {
//				childCtx, _ = context.WithTimeout(ctx, timeout)
//			}
//			mu.Lock()
//			err := xc.call(rpcAddr, serviceMethod, args, curReply, childCtx)
//			if err != nil && e == nil {
//				e = err
//				cancel()
//			}
//			if err == nil && !replyDone {
//				reflect.ValueOf(reply).Elem().Set(reflect.ValueOf(curReply).Elem())
//				replyDone = true
//			}
//			mu.Unlock()
//		}(rpcAddr)
//	}
//	wg.Wait()
//	cancel()
//	return e
//}
