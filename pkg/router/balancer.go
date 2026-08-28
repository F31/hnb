package router

import (
	"sync"
	"sync/atomic"
	"time"
)

type BalancerType string

const (
	BalancerRoundRobin    BalancerType = "round_robin"
	BalancerLeastConn     BalancerType = "least_connections"
	BalancerRandom        BalancerType = "random"
)

type Backend struct {
	ClusterID     string
	ActiveConns   int64
	TotalRequests int64
	Failures      int64
	LastUsed      time.Time
	IsHealthy     bool
}

type Balancer interface {
	Select(backends []*Backend) *Backend
	Name() string
}

type RoundRobinBalancer struct {
	counter atomic.Uint64
}

func NewRoundRobinBalancer() *RoundRobinBalancer {
	return &RoundRobinBalancer{}
}

func (b *RoundRobinBalancer) Name() string { return string(BalancerRoundRobin) }

func (b *RoundRobinBalancer) Select(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}
	idx := b.counter.Add(1) % uint64(len(backends))
	return backends[idx]
}

type LeastConnBalancer struct {
	mu sync.Mutex
}

func NewLeastConnBalancer() *LeastConnBalancer {
	return &LeastConnBalancer{}
}

func (b *LeastConnBalancer) Name() string { return string(BalancerLeastConn) }

func (b *LeastConnBalancer) Select(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}
	best := backends[0]
	for _, be := range backends[1:] {
		if atomic.LoadInt64(&be.ActiveConns) < atomic.LoadInt64(&best.ActiveConns) {
			best = be
		}
	}
	return best
}

type RandomBalancer struct{}

func NewRandomBalancer() *RandomBalancer {
	return &RandomBalancer{}
}

func (b *RandomBalancer) Name() string { return string(BalancerRandom) }

func (b *RandomBalancer) Select(backends []*Backend) *Backend {
	if len(backends) == 0 {
		return nil
	}
	return backends[time.Now().UnixNano()%int64(len(backends))]
}