package balancer

import (
	"math/rand"
	"sync"
)

type SelectMode int

const (
	RandomSelect SelectMode = iota
	RoundRobinSelect
	ConsistentHashSelect
)

type Balancer interface {
	Next(mth string, clientAddr string, addrs []string) string
	refresh(mth string, addrs []string) error
}

type BalancerX struct {
	mu        sync.Mutex
	balancers []Balancer
	Strategy  map[string]SelectMode
}

func (bx *BalancerX) Next(strategy, method, clientAddr string, addrs []string) string {
	bx.mu.Lock()
	defer bx.mu.Unlock()
	mode := bx.Strategy[strategy]
	return bx.balancers[mode].Next(method, clientAddr, addrs)
}

func (bx *BalancerX) Register(name string, balancer Balancer) SelectMode {
	bx.mu.Lock()
	defer bx.mu.Unlock()
	bx.balancers = append(bx.balancers, balancer)
	idx := len(bx.balancers) - 1
	bx.Strategy[name] = SelectMode(idx)
	return SelectMode(idx)
}
func (bx *BalancerX) GetALL() []string {
	strategys := make([]string, 0)
	for k, _ := range bx.Strategy {
		strategys = append(strategys, k)
	}
	return strategys
}

var DefaultBalancerX *BalancerX = NewBalancerX()

func NewBalancerX() *BalancerX {
	b := &BalancerX{
		balancers: make([]Balancer, 0),
		Strategy:  map[string]SelectMode{},
	}
	b.Register("RandomSelect", &RandomBalancer{
		r: rand.Rand{},
	})
	b.Register("RoundRobin", &RoundRobinBalancer{
		mu:   sync.Mutex{},
		last: map[string]int{},
	})
	b.Register("ConsistentHash", &ConsistentHashBalancer{
		ringMap: map[string]*ring{},
	})
	return b
}

type RandomBalancer struct {
	r rand.Rand
}

func (b *RandomBalancer) Next(mth string, clientAddr string, addrs []string) string {
	return addrs[b.r.Intn(len(addrs))]
}

func (b *RandomBalancer) refresh(mth string, addrs []string) error {
	return nil
}

type RoundRobinBalancer struct {
	mu   sync.Mutex
	last map[string]int
}

func (b *RoundRobinBalancer) Next(mth string, clientAddr string, addrs []string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	next := b.last[mth]
	b.last[mth] = next + 1
	return addrs[next%len(addrs)]
}

func (b *RoundRobinBalancer) refresh(mth string, addrs []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.last[mth] = 0
	return nil
}
