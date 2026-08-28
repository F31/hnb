package upstream

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

type Manager struct {
	mu        sync.RWMutex
	upstreams map[string]Upstream
	nc        *nats.Conn
	interval  time.Duration
}

func NewManager(nc *nats.Conn, interval time.Duration) *Manager {
	m := &Manager{
		upstreams: make(map[string]Upstream),
		nc:        nc,
		interval:  interval,
	}
	go m.healthLoop()
	return m
}

func (m *Manager) Register(name string, upstream Upstream) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.upstreams[name] = upstream
	log.Printf("[upstream] registered %s (%s)", name, upstream.Name())
}

func (m *Manager) Get(name string) (Upstream, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.upstreams[name]
	return u, ok
}

func (m *Manager) Call(name string, req *InternalRequest) (*InternalResponse, error) {
	up, ok := m.Get(name)
	if !ok {
		return nil, fmt.Errorf("upstream %s not found", name)
	}
	if !up.Health() {
		return nil, fmt.Errorf("upstream %s is unhealthy", name)
	}
	return up.Call(req)
}

func (m *Manager) healthLoop() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.RLock()
		for name, up := range m.upstreams {
			healthy := up.Health()
			if !healthy {
				log.Printf("[upstream] %s is unhealthy", name)
			}
		}
		m.mu.RUnlock()
	}
}

func (m *Manager) NATS() *nats.Conn {
	return m.nc
}
