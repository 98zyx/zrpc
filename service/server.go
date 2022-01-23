package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
	"zrpc/codec"
	"zrpc/registry"
)

const MagicNumber = 0x3bef5c

//用于建立连接后双方达成的协议选项，该option默认用json编码
type Option struct {
	MagicNumber int        // 标识该消息为zrpc请求
	CodecType   codec.Type // 表示请求的编码方式
	MaxCallTime time.Duration
}

// 默认协议选项
var DefaultOption = &Option{
	MagicNumber: MagicNumber,
	CodecType:   codec.GobType,
	MaxCallTime: 10 * time.Second,
}

// 表示服务器
type Server struct {
	registerAddr string
	addr         string
	serviceMap   sync.Map
	methodMap    sync.Map
}

func NewServer(registerAddr, serverAddr string) *Server {
	return &Server{
		registerAddr: registerAddr,
		addr:         serverAddr,
		serviceMap:   sync.Map{},
		methodMap:    sync.Map{},
	}
}

//var DefaultServer = NewServer()

func (server *Server) Register(rcvr interface{}) error {
	s := newService(rcvr)
	if _, dup := server.serviceMap.LoadOrStore(s.name, s); dup {
		return fmt.Errorf("rpc: service already defined:" + s.name)
	}
	for k, _ := range s.methods {
		server.methodMap.Store(s.name+"."+k, true)
	}
	return nil
}

//func Register(rcvr interface{}) error {
//	return DefaultServer.Register(rcvr)
//}

func (server *Server) findService(serviceMethod string) (svc *service, mtype *methodType, err error) {
	dot := strings.LastIndex(serviceMethod, ".")
	if dot < 0 {
		err = errors.New("rpc server: service/method request ill-formed:" + serviceMethod)
		return
	}
	serviceName, methodName := serviceMethod[:dot], serviceMethod[dot+1:]
	svci, ok := server.serviceMap.Load(serviceName)
	if !ok {
		err = errors.New("rpc server: can't find service:" + serviceName)
		return
	}
	svc = svci.(*service)
	mtype = svc.methods[methodName]
	if mtype == nil {
		err = errors.New("rpc server: can't find method:" + methodName)
	}
	return
}

// server注册传入的端口/listener
func (server *Server) Listen(listener net.Listener, heartbeatPeriod time.Duration) {
	go server.Heartbeat(heartbeatPeriod)
	for {
		conn, err := listener.Accept() // 建立tcp连接，conn
		if err != nil {
			log.Println("rpc server: accept error:", err)
			return
		}
		// 处理该连接
		go server.ServeConn(conn)
	}
}

//func Listen(listener net.Listener) {
//	DefaultServer.Listen(listener)
//}

const (
	connected        = "200 Connected to Gee RPC"
	DefaultRPCPath   = "/zrpc"
	DefaultDebugPath = "/debug/zrpc"
)

func (server *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodConnect {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusMethodNotAllowed)
		io.WriteString(w, "405 must CONNECT\n")
	}
	conn, _, err := w.(http.Hijacker).Hijack()
	if err != nil {
		log.Print("rpc hijacking", req.RemoteAddr, ":", err.Error())
		return
	}
	_, _ = io.WriteString(conn, "HTTP/1.0 "+connected+"\n\n")
	server.ServeConn(conn)
}

func (server *Server) RegisterHTTPInterface() {
	http.Handle(DefaultRPCPath, server)
	http.Handle(DefaultDebugPath, debugHTTP{server})
}

//func RegisterHTTPInterface() {
//	DefaultServer.RegisterHTTPInterface()
//}
func (server *Server) ServeConn(conn io.ReadWriteCloser) {
	defer func() { _ = conn.Close() }() // 关闭连接
	var opt Option
	// 从连接中解析opt，确定此次rpc通信的协议选项
	if err := json.NewDecoder(conn).Decode(&opt); err != nil {
		log.Println("rpc server: options error:", err)
		return
	}
	if opt.MagicNumber != MagicNumber {
		log.Printf("rpc server: invalid magic number: %x\n", opt.MagicNumber)
		return
	}
	// 根据此次rpc映射的序列化类型，返回相应的编解码器
	f := codec.NewCodecFuncMap[opt.CodecType]
	if f == nil {
		log.Printf("rpc server: invalid codec type %s", opt.CodecType)
	}
	server.serveCodec(f(conn), opt.MaxCallTime) // 服务器正式与客户端开始沟通
}

var invalidRequest = struct{}{}

// 使用选定的编解码器，正式与客户端开始沟通
func (server *Server) serveCodec(cc codec.Codec, timeout time.Duration) {
	sending := new(sync.Mutex) // 每次只能发送一条回应，不能同时发送多条回应
	wg := new(sync.WaitGroup)  // 可同时处理多次请求，不需要等上一条请求处理完成后再处理新的请求
	for {
		req, err := server.readRequest(cc) // 读取请求
		if err != nil {
			if req == nil {
				break
			}
			req.h.Error = err.Error()
			server.sendResponse(cc, req.h, invalidRequest, sending) // 发送回应
			continue
		}
		wg.Add(1)
		go server.handleRequest(cc, req, sending, wg, timeout) // 处理该请求
	}
	wg.Wait()
	_ = cc.Close()
}

type request struct {
	h            *codec.Header
	argv, replyv reflect.Value
	mtype        *methodType
	svc          *service
}

// 读取请求头
func (server *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	var h codec.Header
	if err := cc.ReadHeader(&h); err != nil { // 读取头部
		if err != io.EOF && err != io.ErrUnexpectedEOF {
			log.Println("rpc server: read header error:", err)
		}
		return nil, err
	}
	return &h, nil
}

// 读取请求
func (server *Server) readRequest(cc codec.Codec) (*request, error) {
	// 读取请求的头部
	h, err := server.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}
	req := &request{h: h}
	// TypeOf返回的是Type类型，reflect.New返回一个指向某类型的零值的指针
	req.svc, req.mtype, err = server.findService(h.ServiceMethod)
	req.argv, req.replyv = req.mtype.newArgv(), req.mtype.newRpleyv()
	argvi := req.argv.Interface()
	if req.argv.Type().Kind() != reflect.Ptr {
		argvi = req.argv.Addr().Interface()
	}
	// Interface()将该值作为Interface{}返回
	if err = cc.ReadBody(argvi); err != nil {
		log.Println("rpc server: read argv err:", err)
		return req, err
	}
	return req, nil
}

func (server *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	// 发送消息
	if err := cc.Write(h, body); err != nil {
		log.Println("rpc server: write response error:", err)
	}
}

func (server *Server) handleRequest(cc codec.Codec, req *request, sending *sync.Mutex, wg *sync.WaitGroup, timeout time.Duration) {
	defer wg.Done()
	called := make(chan bool, 1)
	sent := make(chan bool, 1)
	go func() {
		err := req.svc.call(req.mtype, req.argv, req.replyv)
		called <- true
		if err != nil {
			req.h.Error = err.Error()
			server.sendResponse(cc, req.h, invalidRequest, sending)
			sent <- true
			return
		}
		sent <- true
		server.sendResponse(cc, req.h, req.replyv.Interface(), sending)
	}()
	if timeout == 0 {
		<-called
		<-sent
		return
	}
	select {
	case <-time.After(timeout):
		req.h.Error = fmt.Sprintf("rpc server: request handle timeout: expect within %s", timeout)
		server.sendResponse(cc, req.h, invalidRequest, sending)
	case <-called:
		<-sent
	}
}

func (s *Server) Heartbeat(period time.Duration) {
	if period == 0 {
		period = registry.DefaultTimeout - time.Minute
	}
	var err error
	err = s.sendHeartbeat()
	go func() {
		t := time.NewTicker(period)
		for err == nil {
			<-t.C
			err = s.sendHeartbeat()
		}
	}()
}

func (s *Server) sendHeartbeat() error {
	log.Println(s.addr, "send heart beat to registry ", s.registerAddr)
	httpClient := &http.Client{}
	req, _ := http.NewRequest(http.MethodPost, s.registerAddr, nil)
	services := make([]string, 0)
	s.methodMap.Range(func(key, value interface{}) bool {
		services = append(services, key.(string))
		return true
	})
	req.Header.Set("X-Zrpc-Servers", s.addr)
	req.Header.Set("X-Zrpc-Services", strings.Join(services, ","))
	if _, err := httpClient.Do(req); err != nil {
		log.Println("rpc server: heart beat err:", err)
		return err
	}
	return nil
}
