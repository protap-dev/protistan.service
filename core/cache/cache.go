package cache

import (
	"container/list"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// ============================================================================
// CACHE MANAGER INTERFACE
// ============================================================================

// CacheManager defines the interface for cache operations
type CacheManager interface {
	// Get retrieves a cached item by key
	Get(ctx context.Context, key string) (any, bool)

	// GetWithVersion retrieves a cached item by key with version information for optimistic locking
	GetWithVersion(ctx context.Context, key string) (any, int64, bool)

	// GetOrLoad retrieves from cache or loads using the provided loader function (stampede protection)
	GetOrLoad(ctx context.Context, key string, ttl time.Duration, loader func() (any, error)) (any, error)

	// Set stores an item in cache with TTL
	Set(ctx context.Context, key string, value any, ttl time.Duration) error

	// SetWithVersion stores an item in cache with TTL and version for optimistic locking
	SetWithVersion(ctx context.Context, key string, value any, version int64, ttl time.Duration) error

	// Delete removes an item from cache
	Delete(ctx context.Context, key string) error

	// DeleteByPrefix removes all items with keys starting with the given prefix
	DeleteByPrefix(ctx context.Context, prefix string) error

	// Clear removes all items from cache
	Clear(ctx context.Context) error

	// AddInvalidationListener adds a listener for cache invalidation events
	AddInvalidationListener(key string, listener CacheInvalidationListener)

	// RemoveInvalidationListener removes a listener for cache invalidation events
	RemoveInvalidationListener(key string, listener CacheInvalidationListener)

	// GetMetrics returns current cache metrics
	GetMetrics() *CacheMetrics
}

// CacheInvalidationListener defines the interface for cache invalidation notifications
type CacheInvalidationListener interface {
	OnCacheInvalidated(key string, version int64)
}

// ============================================================================
// CACHE CONFIGURATION
// ============================================================================

// CacheConfig holds configuration for InMemoryCache
type CacheConfig struct {
	// MaxSizeBytes is the maximum memory size in bytes (default: 100MB)
	MaxSizeBytes int64

	// MaxEntries is the maximum number of entries (default: 10,000)
	MaxEntries int

	// CleanupInterval is how often to run cleanup (default: 5 minutes)
	CleanupInterval time.Duration

	// EnableMetrics enables performance metrics tracking (default: true)
	EnableMetrics bool
}

// DefaultCacheConfig returns default cache configuration
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		MaxSizeBytes:    100 * 1024 * 1024, // 100MB
		MaxEntries:      10000,
		CleanupInterval: 5 * time.Minute,
		EnableMetrics:   true,
	}
}

// ============================================================================
// CACHE METRICS
// ============================================================================

// CacheMetrics tracks cache performance statistics
type CacheMetrics struct {
	Hits        atomic.Int64 // Cache hits
	Misses      atomic.Int64 // Cache misses
	Sets        atomic.Int64 // Set operations
	Deletes     atomic.Int64 // Delete operations
	Evictions   atomic.Int64 // LRU evictions
	Expirations atomic.Int64 // TTL expirations
	Size        atomic.Int64 // Current size in bytes
	Entries     atomic.Int64 // Current number of entries
}

// HitRate returns the cache hit rate as a percentage
func (m *CacheMetrics) HitRate() float64 {
	hits := m.Hits.Load()
	misses := m.Misses.Load()
	total := hits + misses
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total) * 100
}

// ============================================================================
// IN-MEMORY CACHE IMPLEMENTATION
// ============================================================================

// InMemoryCache implements CacheManager with in-memory storage, TTL support, and LRU eviction
type InMemoryCache struct {
	mu           sync.RWMutex
	cache        map[string]*CacheEntry
	evictionList *list.List                  // LRU doubly-linked list
	evictionMap  map[string]*list.Element    // Fast lookup for LRU nodes
	inFlight     map[string]*inFlightRequest // Stampede protection
	listeners    map[string][]CacheInvalidationListener
	config       *CacheConfig
	metrics      *CacheMetrics
	stopCleanup  chan struct{}
}

// CacheEntry holds cached data with expiration time, version, and size
type CacheEntry struct {
	key       string
	data      any
	version   int64
	expiresAt time.Time
	size      int64 // Estimated size in bytes
	lruNode   *list.Element
}

// inFlightRequest tracks an ongoing cache load operation to prevent stampede
type inFlightRequest struct {
	wg  sync.WaitGroup
	val any
	err error
}

// NewInMemoryCache creates a new in-memory cache manager with default configuration
func NewInMemoryCache() CacheManager {
	return NewInMemoryCacheWithConfig(DefaultCacheConfig())
}

// NewInMemoryCacheWithConfig creates a new in-memory cache manager with custom configuration
func NewInMemoryCacheWithConfig(config *CacheConfig) CacheManager {
	if config == nil {
		config = DefaultCacheConfig()
	}

	cache := &InMemoryCache{
		cache:        make(map[string]*CacheEntry),
		evictionList: list.New(),
		evictionMap:  make(map[string]*list.Element),
		inFlight:     make(map[string]*inFlightRequest),
		listeners:    make(map[string][]CacheInvalidationListener),
		config:       config,
		metrics:      &CacheMetrics{},
		stopCleanup:  make(chan struct{}),
	}

	// Start cleanup goroutine
	go cache.cleanup()

	return cache
}

// ============================================================================
// CORE CACHE OPERATIONS
// ============================================================================

// Get retrieves a cached item by key
func (c *InMemoryCache) Get(ctx context.Context, key string) (any, bool) {
	// Check context before expensive operations
	select {
	case <-ctx.Done():
		return nil, false
	default:
	}

	c.mu.RLock()
	entry, exists := c.cache[key]
	c.mu.RUnlock()

	if !exists {
		if c.config.EnableMetrics {
			c.metrics.Misses.Add(1)
		}
		return nil, false
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		if c.config.EnableMetrics {
			c.metrics.Misses.Add(1)
		}
		// Async cleanup of expired entry
		go c.Delete(context.Background(), key)
		return nil, false
	}

	// Update LRU (move to front)
	c.mu.Lock()
	c.moveToFront(entry)
	c.mu.Unlock()

	if c.config.EnableMetrics {
		c.metrics.Hits.Add(1)
	}

	return entry.data, true
}

// GetWithVersion retrieves a cached item by key with version information for optimistic locking
func (c *InMemoryCache) GetWithVersion(ctx context.Context, key string) (any, int64, bool) {
	select {
	case <-ctx.Done():
		return nil, 0, false
	default:
	}

	c.mu.RLock()
	entry, exists := c.cache[key]
	c.mu.RUnlock()

	if !exists {
		if c.config.EnableMetrics {
			c.metrics.Misses.Add(1)
		}
		return nil, 0, false
	}

	if time.Now().After(entry.expiresAt) {
		if c.config.EnableMetrics {
			c.metrics.Misses.Add(1)
		}
		go c.Delete(context.Background(), key)
		return nil, 0, false
	}

	// Update LRU
	c.mu.Lock()
	c.moveToFront(entry)
	c.mu.Unlock()

	if c.config.EnableMetrics {
		c.metrics.Hits.Add(1)
	}

	return entry.data, entry.version, true
}

// GetOrLoad retrieves from cache or loads using the provided loader function (stampede protection)
func (c *InMemoryCache) GetOrLoad(ctx context.Context, key string, ttl time.Duration, loader func() (any, error)) (any, error) {
	// Check context first
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Try cache first
	if val, ok := c.Get(ctx, key); ok {
		return val, nil
	}

	// Acquire write lock to check in-flight requests
	c.mu.Lock()

	// Double-check cache after acquiring lock
	if entry, exists := c.cache[key]; exists && time.Now().Before(entry.expiresAt) {
		c.moveToFront(entry)
		c.mu.Unlock()
		if c.config.EnableMetrics {
			c.metrics.Hits.Add(1)
		}
		return entry.data, nil
	}

	// Check if another goroutine is already loading
	if req, exists := c.inFlight[key]; exists {
		c.mu.Unlock()
		// Wait for in-flight request to complete
		req.wg.Wait()
		return req.val, req.err
	}

	// Start new load operation
	req := &inFlightRequest{}
	req.wg.Add(1)
	c.inFlight[key] = req
	c.mu.Unlock()

	// Load data (outside lock to avoid blocking other operations)
	val, err := loader()

	// Complete load operation
	c.mu.Lock()
	req.val = val
	req.err = err
	delete(c.inFlight, key)
	c.mu.Unlock()

	req.wg.Done()

	// Store in cache if successful
	if err == nil {
		c.Set(ctx, key, val, ttl)
	}

	return val, err
}

// Set stores an item in cache with TTL
func (c *InMemoryCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return c.SetWithVersion(ctx, key, value, 0, ttl)
}

// SetWithVersion stores an item in cache with TTL and version for optimistic locking
func (c *InMemoryCache) SetWithVersion(ctx context.Context, key string, value any, expectedVersion int64, ttl time.Duration) error {
	// Check context
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.cache[key]
	if exists {
		// Check if expired
		if time.Now().After(entry.expiresAt) {
			exists = false
			c.removeEntry(entry)
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

	// Estimate size
	entrySize := estimateSize(value)

	// Evict entries if necessary
	c.evictIfNeeded(entrySize)

	// Create new entry
	newEntry := &CacheEntry{
		key:       key,
		data:      value,
		version:   newVersion,
		expiresAt: time.Now().Add(ttl),
		size:      entrySize,
	}

	// Add to LRU list (front = most recently used)
	node := c.evictionList.PushFront(newEntry)
	newEntry.lruNode = node
	c.evictionMap[key] = node

	// Store in cache
	if exists {
		// Update existing entry size
		c.metrics.Size.Add(entrySize - entry.size)
		c.removeEntry(entry)
	} else {
		c.metrics.Size.Add(entrySize)
		c.metrics.Entries.Add(1)
	}

	c.cache[key] = newEntry

	if c.config.EnableMetrics {
		c.metrics.Sets.Add(1)
	}

	// Notify listeners of invalidation
	c.notifyInvalidation(key, newVersion)

	return nil
}

// Delete removes an item from cache
func (c *InMemoryCache) Delete(ctx context.Context, key string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.cache[key]
	if exists {
		c.removeEntry(entry)
		delete(c.cache, key)

		if c.config.EnableMetrics {
			c.metrics.Deletes.Add(1)
		}

		// Notify listeners of invalidation
		c.notifyInvalidation(key, entry.version+1)
	}
	return nil
}

// DeleteByPrefix removes all items with keys starting with the given prefix
func (c *InMemoryCache) DeleteByPrefix(ctx context.Context, prefix string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	var keysToDelete []string
	c.mu.RLock()
	for key := range c.cache {
		if strings.HasPrefix(key, prefix) {
			keysToDelete = append(keysToDelete, key)
		}
	}
	c.mu.RUnlock()

	if len(keysToDelete) == 0 {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, key := range keysToDelete {
		if entry, exists := c.cache[key]; exists {
			nextVersion := entry.version + 1

			c.removeEntry(entry)
			delete(c.cache, key)

			if c.config.EnableMetrics {
				c.metrics.Deletes.Add(1)
			}

			c.notifyInvalidation(key, nextVersion)
		}
	}

	return nil
}

// Clear removes all items from cache
func (c *InMemoryCache) Clear(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Collect all keys and versions for notification
	keysToInvalidate := make([]string, 0, len(c.cache))
	versionsToInvalidate := make([]int64, 0, len(c.cache))

	for key, entry := range c.cache {
		keysToInvalidate = append(keysToInvalidate, key)
		versionsToInvalidate = append(versionsToInvalidate, entry.version+1)
	}

	// Clear all structures
	c.cache = make(map[string]*CacheEntry)
	c.evictionList = list.New()
	c.evictionMap = make(map[string]*list.Element)
	c.metrics.Size.Store(0)
	c.metrics.Entries.Store(0)

	// Notify listeners for all cleared keys
	for i, key := range keysToInvalidate {
		c.notifyInvalidation(key, versionsToInvalidate[i])
	}

	return nil
}

// ============================================================================
// LRU EVICTION LOGIC
// ============================================================================

// evictIfNeeded evicts entries if adding newSize would exceed limits
func (c *InMemoryCache) evictIfNeeded(newSize int64) {
	currentSize := c.metrics.Size.Load()
	currentEntries := int(c.metrics.Entries.Load())

	// Evict by size
	for currentSize+newSize > c.config.MaxSizeBytes && c.evictionList.Len() > 0 {
		c.evictOldest()
		currentSize = c.metrics.Size.Load()
	}

	// Evict by count
	for currentEntries >= c.config.MaxEntries && c.evictionList.Len() > 0 {
		c.evictOldest()
		currentEntries = int(c.metrics.Entries.Load())
	}
}

// evictOldest removes the least recently used entry
func (c *InMemoryCache) evictOldest() {
	oldest := c.evictionList.Back()
	if oldest == nil {
		return
	}

	entry := oldest.Value.(*CacheEntry)
	c.removeEntry(entry)
	delete(c.cache, entry.key)

	if c.config.EnableMetrics {
		c.metrics.Evictions.Add(1)
	}

	// Notify listeners
	c.notifyInvalidation(entry.key, entry.version+1)
}

// moveToFront moves an entry to the front of the LRU list (most recently used)
func (c *InMemoryCache) moveToFront(entry *CacheEntry) {
	if entry.lruNode != nil {
		c.evictionList.MoveToFront(entry.lruNode)
	}
}

// removeEntry removes an entry from LRU tracking
func (c *InMemoryCache) removeEntry(entry *CacheEntry) {
	if entry.lruNode != nil {
		c.evictionList.Remove(entry.lruNode)
		delete(c.evictionMap, entry.key)
	}
	c.metrics.Size.Add(-entry.size)
	c.metrics.Entries.Add(-1)
}

// ============================================================================
// INVALIDATION LISTENERS
// ============================================================================

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

// ============================================================================
// CLEANUP & METRICS
// ============================================================================

// cleanup periodically removes expired entries
func (c *InMemoryCache) cleanup() {
	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanupExpired()
		case <-c.stopCleanup:
			return
		}
	}
}

// cleanupExpired removes all expired entries
func (c *InMemoryCache) cleanupExpired() {
	c.mu.Lock()
	now := time.Now()
	var keysToInvalidate []string
	var versionsToInvalidate []int64

	for key, entry := range c.cache {
		if now.After(entry.expiresAt) {
			keysToInvalidate = append(keysToInvalidate, key)
			versionsToInvalidate = append(versionsToInvalidate, entry.version+1)
			c.removeEntry(entry)
			delete(c.cache, key)

			if c.config.EnableMetrics {
				c.metrics.Expirations.Add(1)
			}
		}
	}
	c.mu.Unlock()

	// Notify listeners for expired entries
	for i, key := range keysToInvalidate {
		c.notifyInvalidation(key, versionsToInvalidate[i])
	}
}

// GetMetrics returns current cache metrics
func (c *InMemoryCache) GetMetrics() *CacheMetrics {
	return c.metrics
}

// Stop gracefully stops the cache cleanup goroutine
func (c *InMemoryCache) Stop() {
	close(c.stopCleanup)
}

// ============================================================================
// UTILITY FUNCTIONS
// ============================================================================

// estimateSize estimates the memory size of a value in bytes
func estimateSize(value any) int64 {
	if value == nil {
		return 8 // pointer size
	}

	// Use unsafe.Sizeof for basic estimation
	size := int64(unsafe.Sizeof(value))

	// Add additional size for common types
	switch v := value.(type) {
	case string:
		size += int64(len(v))
	case []byte:
		size += int64(len(v))
	case map[string]any:
		size += int64(len(v)) * 64 // Rough estimate: 64 bytes per entry
	}

	return size
}

// ============================================================================
// ERROR TYPES
// ============================================================================

// CacheVersionConflictError represents a version conflict in optimistic locking
type CacheVersionConflictError struct {
	Key             string
	ExpectedVersion int64
	CurrentVersion  int64
}

func (e *CacheVersionConflictError) Error() string {
	return fmt.Sprintf("cache version conflict for key %s: expected version %d, got %d", e.Key, e.ExpectedVersion, e.CurrentVersion)
}
