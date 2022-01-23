package registry

import (
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"zrpc/balancer"
)

type ZRegistry struct {
	timeout        time.Duration
	mu             sync.Mutex
	servers        map[string]*ServerItem
	services       map[string]map[string]bool
	server2service map[string][]string
	bx             *balancer.BalancerX
}

type ServerItem struct {
	Addr  string
	start time.Time
}

const (
	defaultPath    = "/registry"
	DefaultTimeout = time.Minute * 5
)

func New(timeout time.Duration) *ZRegistry {
	return &ZRegistry{
		servers:        make(map[string]*ServerItem),
		timeout:        timeout,
		bx:             balancer.DefaultBalancerX,
		services:       map[string]map[string]bool{},
		server2service: map[string][]string{},
	}
}

var DefaultZRegister = New(DefaultTimeout)

func (r *ZRegistry) putServer(addr string, methods []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.servers[addr]
	if s == nil {
		r.servers[addr] = &ServerItem{
			addr,
			time.Now(),
		}
	} else {
		s.start = time.Now()
	}
	for i := range methods {
		if v, ok := r.services[methods[i]]; ok {
			if !v[addr] {
				v[addr] = true
			}
		} else {
			r.services[methods[i]] = make(map[string]bool)
			r.services[methods[i]][addr] = true
		}
	}
	r.server2service[addr] = methods
}

func (r *ZRegistry) aliveServers(method string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var alive []string
	if servers, ok := r.services[method]; ok {
		for server, _ := range servers {
			if r.timeout == 0 || r.servers[server].start.Add(r.timeout).After(time.Now()) {
				alive = append(alive, server)
			} else {
				for _, v := range r.server2service[server] {
					delete(r.services[v], server)
				}
				delete(r.server2service, server)
				delete(r.servers, server)
			}
		}
	}
	sort.Strings(alive)
	return alive
}

func (r *ZRegistry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		mth := req.Header.Get("X-Zrpc-Services")
		alive := r.aliveServers(mth)
		mode := req.Header.Get("X-Zrpc-Mode")
		target := r.bx.Next(mode, mth, req.RemoteAddr, alive)
		log.Println("remote addr:" + req.RemoteAddr + ", target addr:" + target)
		w.Header().Set("X-Zrpc-Servers", target)
		w.WriteHeader(http.StatusOK)
	case http.MethodPost:
		addr := req.Header.Get("X-Zrpc-Servers")
		mths := strings.Split(req.Header.Get("X-Zrpc-Services"), ",")
		if addr == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		r.putServer(addr, mths)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (r *ZRegistry) HandleHTTP(registryPath string) {
	http.Handle(registryPath, r)
	log.Println("rpc registry path:", registryPath)
}

func HandleHTTP() {
	DefaultZRegister.HandleHTTP(defaultPath)
}
