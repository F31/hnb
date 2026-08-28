package cache

import (
	"sync"
	"time"
)

type Entry struct {
	Data      any
	CreatedAt time.Time
	TTL       time.Duration
	Version   int64
}

type Cache struct {
	mu         sync.RWMutex
	store      map[string]*Entry
	defaultTTL time.Duration
}

func New(defaultTTL time.Duration) *Cache {
	return &Cache{
		store:      make(map[string]*Entry),
		defaultTTL: defaultTTL,
	}
}

func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	entry, ok := c.store[key]
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}

	if time.Since(entry.CreatedAt) >= entry.TTL {
		c.mu.Lock()
		delete(c.store, key)
		c.mu.Unlock()
		return nil, false
	}

	return entry.Data, true
}

func (c *Cache) Set(key string, data any, ttl time.Duration) {
	if ttl == 0 {
		ttl = c.defaultTTL
	}
	c.mu.Lock()
	c.store[key] = &Entry{
		Data:      data,
		CreatedAt: time.Now(),
		TTL:       ttl,
	}
	c.mu.Unlock()
}

func (c *Cache) Delete(key string) {
	c.mu.Lock()
	delete(c.store, key)
	c.mu.Unlock()
}

func (c *Cache) GetOrSet(key string, ttl time.Duration, fn func() (any, error)) (any, error) {
	if data, ok := c.Get(key); ok {
		return data, nil
	}
	data, err := fn()
	if err != nil {
		return nil, err
	}
	c.Set(key, data, ttl)
	return data, nil
}

// Security-sensitive data: never cache, always read from authoritative source
func (c *Cache) GetOrSetAuth(key string, fn func() (any, error)) (any, error) {
	return fn()
}

func (c *Cache) Clear() {
	c.mu.Lock()
	c.store = make(map[string]*Entry)
	c.mu.Unlock()
}

func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.store)
}
