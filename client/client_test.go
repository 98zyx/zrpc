package client

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
	"zrpc/service"
)

func _assert(condition bool, msg string, v ...interface{}) {
	if !condition {
		panic(fmt.Sprintf("assertion failed:"+msg, v...))
	}
}

func TestClient_Dial(t *testing.T) {
	t.Parallel()
	l, _ := net.Listen("tcp", ":0")
	_ = func(conn net.Conn, opt *service.Option) (client *Client, err error) {
		_ = conn.Close()
		time.Sleep(time.Second * 2)
		return nil, nil
	}
	t.Run("timeout", func(t *testing.T) {
		_, err := Dial("tcp", l.Addr().String(), time.Second)
		_assert(err != nil && strings.Contains(err.Error(), "connect timeout"), "expect a timeout error")
	})

	t.Run("0", func(t *testing.T) {
		_, err := Dial("tcp", l.Addr().String(), 3*time.Second)
		_assert(err == nil, "0 means no limit")
	})
}

type Bar int

func (b Bar) Timeout(argv int, reply *int) error {
	time.Sleep(time.Second * 2)
	return nil
}
func startServer(addr chan string) {
	var b Bar
	_ = service.Register(&b)
	l, _ := net.Listen("tcp", ":0")
	addr <- l.Addr().String()
	service.Listen(l)
}

//func TestClient_Call(t *testing.T) {
//	t.Parallel()
//	addrch := make(chan string)
//	go startServer(addrch)
//	addr := <-addrch
//	time.Sleep(time.Second)
//	t.Run("client timeout", func(t *testing.T) {
//		client, _ := Dial("tcp", addr, 0)
//		var reply int
//		err := client.Call("Bar.Timeout", 1, &reply, 1*time.Second)
//		_assert(err != nil && strings.Contains(err.Error(), "call timeout"), "expect a timeout error")
//	})
//
//	t.Run("client timeout", func(t *testing.T) {
//		client, _ := Dial("tcp", addr, 0, &service.Option{MaxCallTime: 1 * time.Second})
//		var reply int
//		err := client.Call("Bar.Timeout", 1, &reply, 0)
//		_assert(err != nil && strings.Contains(err.Error(), "handle timeout"), "expect a timeout error")
//	})
//}
