package router

import (
	"fmt"
	"sync"
	"time"
)

type CircuitState int

const (
	CircuitClosed   CircuitState = 0
	CircuitHalfOpen CircuitState = 1
	CircuitOpen     CircuitState = 2
)

type CircuitBreaker struct {
	mu              sync.RWMutex
	state           CircuitState
	failures        int
	threshold       int
	resetTimeout    time.Duration
	lastFailureTime time.Time
	halfOpenSuccess int
}

func NewCircuitBreaker(threshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:        CircuitClosed,
		threshold:    threshold,
		resetTimeout: resetTimeout,
	}
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.RLock()
	state := cb.state
	cb.mu.RUnlock()

	switch state {
	case CircuitClosed:
		return true
	case CircuitHalfOpen:
		return true
	case CircuitOpen:
		cb.mu.Lock()
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = CircuitHalfOpen
			cb.halfOpenSuccess = 0
			cb.mu.Unlock()
			return true
		}
		cb.mu.Unlock()
		return false
	}
	return false
}

func (cb *CircuitBreaker) Success() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitHalfOpen {
		cb.halfOpenSuccess++
		if cb.halfOpenSuccess >= 2 {
			cb.state = CircuitClosed
			cb.failures = 0
		}
	}
}

func (cb *CircuitBreaker) Failure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	if cb.failures >= cb.threshold {
		cb.state = CircuitOpen
	}
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CircuitClosed
	cb.failures = 0
}

func (cb *CircuitBreaker) Stats() map[string]any {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return map[string]any{
		"state":    cb.state,
		"failures": cb.failures,
		"threshold": cb.threshold,
	}
}

type ConnectionPool struct {
	mu       sync.RWMutex
	pool     map[string]*PoolEntry
	maxSize  int
	ttl      time.Duration
}

type PoolEntry struct {
	ClusterID  string
	ActiveReqs int64
	CreatedAt  time.Time
	LastUsed   time.Time
}

func NewConnectionPool(maxSize int, ttl time.Duration) *ConnectionPool {
	return &ConnectionPool{
		pool:    make(map[string]*PoolEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

func (p *ConnectionPool) Acquire(clusterID string) (*PoolEntry, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	entry, exists := p.pool[clusterID]
	if !exists {
		if len(p.pool) >= p.maxSize {
			p.evictStale()
		}
		entry = &PoolEntry{
			ClusterID: clusterID,
			CreatedAt: time.Now(),
		}
		p.pool[clusterID] = entry
	}

	entry.ActiveReqs++
	entry.LastUsed = time.Now()
	return entry, nil
}

func (p *ConnectionPool) Release(clusterID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if entry, exists := p.pool[clusterID]; exists {
		entry.ActiveReqs--
	}
}

func (p *ConnectionPool) evictStale() {
	now := time.Now()
	for id, entry := range p.pool {
		if entry.ActiveReqs == 0 && now.After(entry.CreatedAt.Add(p.ttl)) {
			delete(p.pool, id)
		}
	}
}

func (p *ConnectionPool) Stats() map[string]any {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := make(map[string]any)
	for id, entry := range p.pool {
		stats[id] = map[string]any{
			"active_reqs": entry.ActiveReqs,
			"created_at":  entry.CreatedAt,
			"last_used":   entry.LastUsed,
		}
	}
	return stats
}

func (p *ConnectionPool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.pool)
}

type ClusterRoute struct {
	ClusterID      string
	Backend        *Backend
	Breaker        *CircuitBreaker
	LastError      string
	LastSuccessAt  time.Time
	LastFailureAt  time.Time
}

func NewClusterRoute(clusterID string) *ClusterRoute {
	return &ClusterRoute{
		ClusterID: clusterID,
		Backend:   &Backend{ClusterID: clusterID, IsHealthy: true},
		Breaker:   NewCircuitBreaker(3, 30*time.Second),
	}
}

func (r *ClusterRoute) String() string {
	return fmt.Sprintf("ClusterRoute{cluster=%s, breaker=%d, healthy=%v}",
		r.ClusterID, r.Breaker.State(), r.Backend.IsHealthy)
}