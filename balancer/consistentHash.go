package balancer

import (
	"crypto"
	"errors"
	"hash"
	"sort"
	"strconv"
	"sync"
)

type ConsistentHashBalancer struct {
	mu      sync.Mutex
	ringMap map[string]*ring
}

func (c *ConsistentHashBalancer) Next(mth, clientAddr string, addrs []string) string {
	//TODO implement me
	c.mu.Lock()
	defer c.mu.Unlock()
	err := c.refresh(mth, addrs)
	if err != nil {
		return ""
	}
	ring := c.ringMap[mth]
	target, err := ring.Find(clientAddr)
	if err != nil {
		return ""
	}
	return target
}

func (c *ConsistentHashBalancer) refresh(mth string, addrs []string) error {
	ring, ok := c.ringMap[mth]
	if !ok {
		c.ringMap[mth] = newRing(crypto.MD5.New(), 32)
		ring = c.ringMap[mth]
	}
	err := ring.Update(addrs)
	if err != nil {
		return err
	}
	return nil
}

var _ Balancer = (*ConsistentHashBalancer)(nil)

type ring struct {
	mu      sync.Mutex
	hash    hash.Hash
	virtual []string
	ori     map[string]bool
	numO2V  int                 // 每个真实节点对应多少个虚拟节点
	v2o     map[string]string   // 虚拟节点映射到真实节点
	o2v     map[string][]string // 真实节点映射到虚拟节点
}

func newRing(hash hash.Hash, num int) *ring {
	return &ring{
		hash:    hash,
		virtual: make([]string, 0),
		numO2V:  num,
		v2o:     map[string]string{},
		o2v:     map[string][]string{},
		ori:     map[string]bool{},
	}
}

func (r *ring) reset() {
	r.o2v = map[string][]string{}
	r.v2o = map[string]string{}
	r.ori = map[string]bool{}
	r.virtual = make([]string, 0)
}

func (r *ring) Find(clientAddr string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.virtual) == 0 {
		return "", errors.New("no node in ring")
	}
	_, err := r.hash.Write([]byte(clientAddr))
	if err != nil {
		return "", err
	}
	clientHash := string(r.hash.Sum(nil))
	r.hash.Reset()
	idx := findUpper(r.virtual, clientHash)
	if idx == -1 {
		return r.v2o[r.virtual[0]], nil
	} else {
		return r.v2o[r.virtual[idx]], nil
	}
}

func (r *ring) Update(addrs []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	sort.Strings(addrs)
	for k, _ := range r.ori {
		idx := find(addrs, k)
		if idx == -1 {
			if err := r.DeleteNode(k); err != nil {
				return err
			}
		}
	}
	for _, item := range addrs {
		if !r.ori[item] {
			err := r.AddNode(item)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *ring) AddNode(addr string) error {
	r.ori[addr] = true
	for i := 0; i < r.numO2V; i++ {
		cur := addr + "#" + strconv.Itoa(i)
		_, err := r.hash.Write([]byte(cur))
		if err != nil {
			return err
		}
		a := r.hash.Sum(nil)
		curHash := string(a)
		r.hash.Reset()
		r.virtual = addHash(r.virtual, curHash)
		r.v2o[curHash] = addr
		if _, ok := r.o2v[addr]; ok {
			r.o2v[addr] = append(r.o2v[addr], curHash)
		} else {
			r.o2v[addr] = append([]string{}, curHash)
		}
	}
	return nil
}

func (r *ring) DeleteNode(addr string) error {
	if !r.ori[addr] {
		return errors.New("addr dose not exist")
	}
	delete(r.ori, addr)
	for _, item := range r.o2v[addr] {
		delete(r.v2o, item)
		r.virtual = deleteSlice(r.virtual, item)
	}
	delete(r.o2v, addr)
	return nil
}

func deleteSlice(nums []string, cur string) []string {
	idx := find(nums, cur)
	if idx == -1 {
		return nums
	}
	if idx == len(nums)-1 {
		return nums[:idx]
	}
	res := append([]string{}, nums[idx+1:]...)
	ans := append(nums[:idx], res...)
	return ans
}

func find(nums []string, cur string) int {
	if len(nums) == -1 {
		return -1
	}
	left, right := 0, len(nums)-1
	for left < right {
		mid := left + (right-left)/2
		if nums[mid] == cur {
			return mid
		} else if nums[mid] < cur {
			left = mid + 1
		} else {
			right = mid - 1
		}
	}
	if nums[left] != cur {
		return -1
	}
	return right
}
func addHash(nums []string, cur string) []string {
	idx := findUpper(nums, cur)
	if idx == -1 {
		nums = append(nums, cur)
		return nums
	}
	res := append([]string{}, nums[idx:]...)
	nums = append(append(nums[:idx], cur), res...)
	return nums
}

func findUpper(nums []string, cur string) int {
	if len(nums) == 0 {
		return -1
	}
	left, right := 0, len(nums)-1
	for left < right {
		mid := left + (right-left)/2
		if nums[mid] <= cur {
			left = mid + 1
		} else {
			right = mid
		}
	}
	if right == len(nums)-1 && nums[right] <= cur {
		return -1
	}
	return right
}
