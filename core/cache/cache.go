package cache

import (
	"context"
	"sync"
	"time"
)

// CacheManager defines the interface for cache operations
type CacheManager interface {
	// Get retrieves a cached item by key
	Get(ctx context.Context, key string) (any, bool)

	// Set stores an item in cache with TTL
	Set(ctx context.Context, key string, value any, ttl time.Duration) error

	// Delete removes an item from cache
	Delete(ctx context.Context, key string) error

	// Clear removes all items from cache
	Clear(ctx context.Context) error
}

// InMemoryCache implements CacheManager with in-memory storage and TTL support
type InMemoryCache struct {
	mu    sync.RWMutex
	cache map[string]*cacheEntry
}

// cacheEntry holds cached data with expiration time
type cacheEntry struct {
	data      any
	expiresAt time.Time
}

// NewInMemoryCache creates a new in-memory cache manager
func NewInMemoryCache() CacheManager {
	cache := &InMemoryCache{
		cache: make(map[string]*cacheEntry),
	}

	// Start cleanup goroutine to remove expired entries
	go cache.cleanup()

	return cache
}

// Get retrieves a cached item by key
func (c *InMemoryCache) Get(ctx context.Context, key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.cache[key]
	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		// Don't return expired entries
		return nil, false
	}

	return entry.data, true
}

// Set stores an item in cache with TTL
func (c *InMemoryCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[key] = &cacheEntry{
		data:      value,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

// Delete removes an item from cache
func (c *InMemoryCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cache, key)
	return nil
}

// Clear removes all items from cache
func (c *InMemoryCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*cacheEntry)
	return nil
}

// cleanup periodically removes expired entries
func (c *InMemoryCache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.cache {
			if now.After(entry.expiresAt) {
				delete(c.cache, key)
			}
		}
		c.mu.Unlock()
	}
}
