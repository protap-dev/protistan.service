package cache

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// CacheManager defines the interface for cache operations
type CacheManager interface {
	// Get retrieves a cached item by key
	Get(ctx context.Context, key string) (any, bool)

	// GetWithVersion retrieves a cached item by key with version information for optimistic locking
	GetWithVersion(ctx context.Context, key string) (any, int64, bool)

	// Set stores an item in cache with TTL
	Set(ctx context.Context, key string, value any, ttl time.Duration) error

	// SetWithVersion stores an item in cache with TTL and version for optimistic locking
	SetWithVersion(ctx context.Context, key string, value any, version int64, ttl time.Duration) error

	// Delete removes an item from cache
	Delete(ctx context.Context, key string) error

	// Clear removes all items from cache
	Clear(ctx context.Context) error

	// AddInvalidationListener adds a listener for cache invalidation events
	AddInvalidationListener(key string, listener CacheInvalidationListener)

	// RemoveInvalidationListener removes a listener for cache invalidation events
	RemoveInvalidationListener(key string, listener CacheInvalidationListener)
}

// CacheInvalidationListener defines the interface for cache invalidation notifications
type CacheInvalidationListener interface {
	OnCacheInvalidated(key string, version int64)
}

// InMemoryCache implements CacheManager with in-memory storage and TTL support
type InMemoryCache struct {
	mu        sync.RWMutex
	cache     map[string]*CacheEntry
	listeners map[string][]CacheInvalidationListener
}

// CacheEntry holds cached data with expiration time and version
type CacheEntry struct {
	data      any
	version   int64
	expiresAt time.Time
}

// NewInMemoryCache creates a new in-memory cache manager
func NewInMemoryCache() CacheManager {
	cache := &InMemoryCache{
		cache:     make(map[string]*CacheEntry),
		listeners: make(map[string][]CacheInvalidationListener),
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

// GetWithVersion retrieves a cached item by key with version information for optimistic locking
func (c *InMemoryCache) GetWithVersion(ctx context.Context, key string) (any, int64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.cache[key]
	if !exists {
		return nil, 0, false
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		return nil, 0, false
	}

	return entry.data, entry.version, true
}

// Set stores an item in cache with TTL
func (c *InMemoryCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return c.SetWithVersion(ctx, key, value, 0, ttl)
}

// SetWithVersion stores an item in cache with TTL and version for optimistic locking
func (c *InMemoryCache) SetWithVersion(ctx context.Context, key string, value any, expectedVersion int64, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.cache[key]
	if exists {
		// Check if expired
		if time.Now().After(entry.expiresAt) {
			exists = false
		} else if expectedVersion != 0 && entry.version != expectedVersion {
			// Version mismatch - optimistic locking conflict
			return &CacheVersionConflictError{
				Key:             key,
				ExpectedVersion: expectedVersion,
				CurrentVersion:  entry.version,
			}
		}
	}

	// Calculate new version
	var newVersion int64
	if exists {
		newVersion = entry.version + 1
	} else {
		newVersion = 1
	}

	// Create new entry
	newEntry := &CacheEntry{
		data:      value,
		version:   newVersion,
		expiresAt: time.Now().Add(ttl),
	}

	// Store in cache
	c.cache[key] = newEntry

	// Notify listeners of invalidation
	c.notifyInvalidation(key, newVersion)

	return nil
}

// CacheVersionConflictError represents a version conflict in optimistic locking
type CacheVersionConflictError struct {
	Key             string
	ExpectedVersion int64
	CurrentVersion  int64
}

func (e *CacheVersionConflictError) Error() string {
	return fmt.Sprintf("cache version conflict for key %s: expected version %d, got %d", e.Key, e.ExpectedVersion, e.CurrentVersion)
}

// Delete removes an item from cache
func (c *InMemoryCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.cache[key]
	if exists {
		delete(c.cache, key)
		// Notify listeners of invalidation
		c.notifyInvalidation(key, entry.version+1)
	}
	return nil
}

// Clear removes all items from cache
func (c *InMemoryCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Collect all keys and versions for notification
	var keysToInvalidate []string
	var versionsToInvalidate []int64
	for key, entry := range c.cache {
		keysToInvalidate = append(keysToInvalidate, key)
		versionsToInvalidate = append(versionsToInvalidate, entry.version+1)
	}

	// Clear cache
	c.cache = make(map[string]*CacheEntry)

	// Notify listeners for all cleared keys
	for i, key := range keysToInvalidate {
		c.notifyInvalidation(key, versionsToInvalidate[i])
	}

	return nil
}

// AddInvalidationListener adds a listener for cache invalidation events
func (c *InMemoryCache) AddInvalidationListener(key string, listener CacheInvalidationListener) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.listeners[key] = append(c.listeners[key], listener)
}

// RemoveInvalidationListener removes a listener for cache invalidation events
func (c *InMemoryCache) RemoveInvalidationListener(key string, listener CacheInvalidationListener) {
	c.mu.Lock()
	defer c.mu.Unlock()

	listeners := c.listeners[key]
	for i, l := range listeners {
		if l == listener {
			c.listeners[key] = append(listeners[:i], listeners[i+1:]...)
			break
		}
	}
}

// notifyInvalidation notifies all listeners for a key about cache invalidation
func (c *InMemoryCache) notifyInvalidation(key string, version int64) {
	listeners := c.listeners[key]
	if len(listeners) == 0 {
		return
	}

	// Create a copy of listeners to avoid holding lock during notification
	listenersCopy := make([]CacheInvalidationListener, len(listeners))
	copy(listenersCopy, listeners)

	// Notify all listeners (outside of lock to prevent deadlocks)
	for _, listener := range listenersCopy {
		go func(l CacheInvalidationListener) {
			defer func() {
				if r := recover(); r != nil {
					// Handle panics in listeners gracefully
				}
			}()
			l.OnCacheInvalidated(key, version)
		}(listener)
	}
}

// cleanup periodically removes expired entries
func (c *InMemoryCache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		var keysToInvalidate []string
		var versionsToInvalidate []int64

		for key, entry := range c.cache {
			if now.After(entry.expiresAt) {
				keysToInvalidate = append(keysToInvalidate, key)
				versionsToInvalidate = append(versionsToInvalidate, entry.version+1)
				delete(c.cache, key)
			}
		}
		c.mu.Unlock()

		// Notify listeners for expired entries
		for i, key := range keysToInvalidate {
			c.notifyInvalidation(key, versionsToInvalidate[i])
		}
	}
}
