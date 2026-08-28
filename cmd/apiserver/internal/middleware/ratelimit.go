package middleware

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type RateLimiterMW struct {
	mu          sync.RWMutex
	limiters    map[string]*rateLimiterEntry
	defaultRate rate.Limit
	burst       int
}

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func NewRateLimiter(defaultRate string) *RateLimiterMW {
	r := &RateLimiterMW{
		limiters: make(map[string]*rateLimiterEntry),
		burst:    20,
	}
	r.defaultRate = parseRate(defaultRate)
	go r.cleanup()
	return r
}

func (rl *RateLimiterMW) Name() string { return "rate_limit" }

func (rl *RateLimiterMW) Handle(ctx *Context, next func()) {
	key := ctx.TenantID
	if key == "" {
		key = ctx.Request.RemoteAddr
	}

	rl.mu.RLock()
	entry, exists := rl.limiters[key]
	rl.mu.RUnlock()

	if !exists {
		limit := rl.defaultRate
		entry = &rateLimiterEntry{
			limiter:  rate.NewLimiter(limit, rl.burst),
			lastSeen: time.Now(),
		}
		rl.mu.Lock()
		rl.limiters[key] = entry
		rl.mu.Unlock()
	}

	entry.lastSeen = time.Now()

	if !entry.limiter.Allow() {
		ctx.Abort(429, []byte(`{"code":42900,"message":"rate limit exceeded"}`))
		return
	}

	next()
}

func (rl *RateLimiterMW) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		for key, entry := range rl.limiters {
			if time.Since(entry.lastSeen) > 10*time.Minute {
				delete(rl.limiters, key)
			}
		}
		rl.mu.Unlock()
	}
}

func parseRate(s string) rate.Limit {
	if s == "" {
		return rate.Inf
	}
	var n float64
	if _, err := fmt.Sscanf(s, "%f/s", &n); err == nil {
		return rate.Limit(n)
	}
	return rate.Inf
}
