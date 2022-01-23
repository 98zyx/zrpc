package main

import (
	"log"
	"net"
	"net/http"
	"sync"
	"time"
	"zrpc/client"
	"zrpc/registry"
	"zrpc/service"
)

type Foo int

type Args struct {
	Num1, Num2 int
}

func (f Foo) Sum(args Args, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}
func (f Foo) Sleep(args Args, reply *int) error {
	time.Sleep(time.Second * 1)
	*reply = args.Num1 + args.Num2
	return nil
}

func startRegistry(wg *sync.WaitGroup) {
	l, _ := net.Listen("tcp", ":9999")
	registry.HandleHTTP()
	wg.Done()
	_ = http.Serve(l, nil)
}

func foo(xc *client.XClient, timeout time.Duration, typ, serviceMethod string, args *Args) {
	var reply int
	var err error
	switch typ {
	case "call":
		err = xc.Call(serviceMethod, args, &reply, timeout)
	case "broadcast":
		//err = xc.Broadcast(serviceMethod, args, &reply, timeout)
	}
	if err != nil {
		log.Printf("%s %s error: %v", typ, serviceMethod, err)
	} else {
		log.Printf("%s %s success: %d + %d = %d", typ, serviceMethod, args.Num1, args.Num2, reply)
	}
}

func call(registryAddr string) {
	xc := client.NewXClient(registryAddr, "ConsistentHash", nil, 0)
	//conn, _ := net.Dial("tcp", addr1)
	//c, _ := client.NewClient(conn, service.DefaultOption)
	defer func() { xc.Close() }()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			foo(xc, 0, "call", "Foo.Sum", &Args{i, i * i})
			//foo(xc, 2*time.Second, "broadcast", "Foo.Sleep", &Args{Num1: i, Num2: i * i})
			//var reply int
			//c.Call("Foo.Sum", &Args{i, i * i}, &reply, context.Background())
		}(i)
	}
	wg.Wait()
}

//func broadcast(registryAddr string) {
//	d := registry.NewZRegistryDiscovery(registryAddr, 0)
//	xc := client.NewXClient(d, registry.RoundRobinSelect, nil, 0)
//	defer func() { xc.Close() }()
//	var wg sync.WaitGroup
//	for i := 0; i < 10; i++ {
//		wg.Add(1)
//		go func(i int) {
//			defer wg.Done()
//			foo(xc, 0, "broadcast", "Foo.Sum", &Args{Num1: i, Num2: i * i})
//			//foo(xc, 2*time.Second, "broadcast", "Foo.Sleep", &Args{Num1: i, Num2: i * i})
//		}(i)
//	}
//	wg.Wait()
//}
func startServer(registryAddr string, wg *sync.WaitGroup) {
	var foo Foo
	l, err := net.Listen("tcp", ":0") //开启监听器
	if err != nil {
		log.Fatalf("network error:", err)
	}
	server := service.NewServer(registryAddr, "tcp@"+l.Addr().String())
	err = server.Register(&foo)
	if err != nil {
		log.Fatalf("register error:", err)
	}
	//service.Heartbeat(registryAddr, "tcp@"+l.Addr().String(), 0)
	wg.Done()
	log.Println("start rpc server on ", l.Addr())
	server.Listen(l, 0) // 注册监听器
}

func main() {
	log.SetFlags(0)
	registryAddr := "http://localhost:9999/registry"
	var wg sync.WaitGroup
	wg.Add(1)
	go startRegistry(&wg)
	wg.Wait()
	time.Sleep(time.Second)
	wg.Add(4)
	go startServer(registryAddr, &wg)
	go startServer(registryAddr, &wg)
	go startServer(registryAddr, &wg)
	go startServer(registryAddr, &wg)
	wg.Wait()

	// 传入服务器端监听的端口
	time.Sleep(1 * time.Second)
	// 多次发送请求
	//call(add1, add2)
	call(registryAddr)
	//broadcast(registryAddr)
}

//func main() {
//	TestString := "Hi, pandaman!"
//
//	Md5Inst := md5.New()
//	Md5Inst.Write([]byte(TestString))
//	Result := Md5Inst.Sum([]byte(""))
//	fmt.Printf("%x\n\n", Result)
//
//	Sha1Inst := sha1.New()
//	Sha1Inst.Write([]byte(TestString))
//	Result = Sha1Inst.Sum([]byte(""))
//	fmt.Printf("%x\n\n", Result)
//}
func mostPoints(questions [][]int) int64 {
	dp := make([][]int64, len(questions))
	for i := range dp {
		dp[i] = make([]int64, 2)
	}
	cant := make([]map[int]bool, len(questions))
	for i := range cant {
		cant[i] = map[int]bool{}
	}
	can := make([]int, len(questions))
	for i := range questions {
		for j := i + 1; j <= i+questions[i][1] && j < len(questions); j++ {
			cant[j][i] = true
		}
	}
	for i := range questions {
		can[i] = i - 1
		for {
			_, ok := cant[i][can[i]]
			if !ok {
				break
			}
			can[i]--
		}
	}
	var ans int64 = 0
	for i := range questions {
		//[[21,5],[92,3],[74,2],[39,4],[58,2],[5,5],[49,4],[65,3]]
		if i == 0 {
			dp[i][0] = int64(questions[0][0])
		} else {
			if can[i] >= 0 {
				dp[i][0] = dp[can[i]][0] + int64(questions[i][0])
				dp[i][0] = dp[can[i]][1] + int64(questions[i][0])
			} else {
				dp[i][0] = int64(questions[i][0])
			}
			for k, _ := range cant[i] {
				dp[i][1] = max(dp[i][1], dp[k][0])
				dp[i][1] = max(dp[i][1], dp[k][1])
			}
			min := len(questions)
			for k, _ := range cant[i] {
				if k < min {
					min = k
				}
			}
			min = min - 1
			if min-1 >= 0 {
				dp[i][0] = max(dp[i][0], dp[min][0]+int64(questions[i][0]))
				dp[i][0] = max(dp[i][0], dp[min][0]+int64(questions[i][1]))
			}
		}
		ans = max(dp[i][0], ans)
		ans = max(dp[i][1], ans)
	}
	return ans
}

//如果我做这道题，那么我

func max(i, j int64) int64 {
	if i < j {
		return j
	} else {
		return i
	}
}
