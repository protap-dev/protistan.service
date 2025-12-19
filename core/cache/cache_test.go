package cache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock listener for testing invalidation events
type mockListener struct {
	mu     sync.Mutex
	events []invalidationEvent
}

type invalidationEvent struct {
	key     string
	version int64
}

func (m *mockListener) OnCacheInvalidated(key string, version int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, invalidationEvent{key: key, version: version})
}

func (m *mockListener) getEvents() []invalidationEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	events := make([]invalidationEvent, len(m.events))
	copy(events, m.events)
	return events
}

func (m *mockListener) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = nil
}

func TestInMemoryCache_BasicOperations(t *testing.T) {
	cache := NewInMemoryCache()
	ctx := context.Background()

	tests := []struct {
		name     string
		key      string
		value    interface{}
		ttl      time.Duration
		wantGet  bool
		wantData interface{}
	}{
		{
			name:     "set and get string",
			key:      "test:key1",
			value:    "test-value",
			ttl:      time.Hour,
			wantGet:  true,
			wantData: "test-value",
		},
		{
			name:     "set and get struct",
			key:      "test:key2",
			value:    map[string]string{"key": "value"},
			ttl:      time.Hour,
			wantGet:  true,
			wantData: map[string]string{"key": "value"},
		},
		{
			name:     "get non-existent key",
			key:      "test:nonexistent",
			value:    nil,
			ttl:      time.Hour,
			wantGet:  false,
			wantData: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear cache before each test
			cache.Clear(ctx)

			if tt.value != nil {
				err := cache.Set(ctx, tt.key, tt.value, tt.ttl)
				require.NoError(t, err)
			}

			data, exists := cache.Get(ctx, tt.key)
			assert.Equal(t, tt.wantGet, exists)
			assert.Equal(t, tt.wantData, data)
		})
	}
}

func TestInMemoryCache_VersionManagement(t *testing.T) {
	cache := NewInMemoryCache()
	ctx := context.Background()

	key := "test:version"
	value := "test-value"
	ttl := time.Hour

	// Test initial version
	err := cache.Set(ctx, key, value, ttl)
	require.NoError(t, err)

	data, version, exists := cache.GetWithVersion(ctx, key)
	assert.True(t, exists)
	assert.Equal(t, value, data)
	assert.Equal(t, int64(1), version)

	// Test version increment on update
	newValue := "updated-value"
	err = cache.Set(ctx, key, newValue, ttl)
	require.NoError(t, err)

	data, version, exists = cache.GetWithVersion(ctx, key)
	assert.True(t, exists)
	assert.Equal(t, newValue, data)
	assert.Equal(t, int64(2), version)
}

func TestInMemoryCache_OptimisticLocking(t *testing.T) {
	cache := NewInMemoryCache()
	ctx := context.Background()

	key := "test:optimistic"
	value := "initial-value"
	ttl := time.Hour

	// Set initial value
	err := cache.Set(ctx, key, value, ttl)
	require.NoError(t, err)

	// Get current version
	_, currentVersion, exists := cache.GetWithVersion(ctx, key)
	require.True(t, exists)

	// Test successful version-based update
	newValue := "updated-value"
	err = cache.SetWithVersion(ctx, key, newValue, currentVersion, ttl)
	assert.NoError(t, err)

	data, _, exists := cache.GetWithVersion(ctx, key)
	assert.True(t, exists)
	assert.Equal(t, newValue, data)

	// Test version conflict
	// Get the version BEFORE updating, then try to update with wrong version
	_, oldVersion, exists := cache.GetWithVersion(ctx, key)
	require.True(t, exists)

	// Update the value (this increments version to oldVersion + 1)
	conflictValue := "conflict-value"
	err = cache.Set(ctx, key, conflictValue, ttl)
	require.NoError(t, err)

	// Now try to update with the OLD version (should conflict)
	err = cache.SetWithVersion(ctx, key, "conflicting-value", oldVersion, ttl)
	assert.Error(t, err)

	var conflictErr *CacheVersionConflictError
	ok := errors.As(err, &conflictErr)
	assert.True(t, ok, "Error should be CacheVersionConflictError")
	if ok {
		assert.Equal(t, key, conflictErr.Key)
		assert.Equal(t, oldVersion, conflictErr.ExpectedVersion)
	}
}

func TestInMemoryCache_EventInvalidation(t *testing.T) {
	cache := NewInMemoryCache()
	ctx := context.Background()

	key := "test:invalidation"
	listener := &mockListener{}
	cache.AddInvalidationListener(key, listener)

	// Test invalidation on set
	err := cache.Set(ctx, key, "value1", time.Hour)
	require.NoError(t, err)

	// Wait for async notification
	time.Sleep(10 * time.Millisecond)

	events := listener.getEvents()
	assert.Len(t, events, 1)
	assert.Equal(t, key, events[0].key)
	assert.Equal(t, int64(1), events[0].version)

	// Test invalidation on delete
	listener.reset()
	err = cache.Delete(ctx, key)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	events = listener.getEvents()
	assert.Len(t, events, 1)
	assert.Equal(t, key, events[0].key)
	assert.Equal(t, int64(2), events[0].version) // Version + 1 after delete
}

func TestInMemoryCache_TTLExpiration(t *testing.T) {
	cache := NewInMemoryCache()
	ctx := context.Background()

	key := "test:ttl"
	value := "test-value"

	// Set with short TTL (100ms)
	err := cache.Set(ctx, key, value, 100*time.Millisecond)
	require.NoError(t, err)

	// Should be available immediately
	data, exists := cache.Get(ctx, key)
	assert.True(t, exists)
	assert.Equal(t, value, data)

	// Wait for expiration (longer than TTL)
	time.Sleep(150 * time.Millisecond)

	// Should be expired now (cleanup runs every 5 minutes, but item should still be expired)
	data, exists = cache.Get(ctx, key)
	assert.False(t, exists)
	assert.Nil(t, data)
}

func TestInMemoryCache_ConcurrentAccess(t *testing.T) {
	cache := NewInMemoryCache()
	ctx := context.Background()

	key := "test:concurrent"
	ttl := time.Hour

	// Concurrent writes
	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			value := map[string]int{"id": id}
			err := cache.Set(ctx, key, value, ttl)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Should have final value
	data, exists := cache.Get(ctx, key)
	assert.True(t, exists)
	assert.NotNil(t, data)

	// Concurrent reads
	readResults := make(chan interface{}, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, exists := cache.Get(ctx, key)
			if exists {
				readResults <- data
			}
		}()
	}

	wg.Wait()
	close(readResults)

	// All reads should return the same value
	firstValue := <-readResults
	for value := range readResults {
		assert.Equal(t, firstValue, value)
	}
}

func TestInMemoryCache_ListenerManagement(t *testing.T) {
	cache := NewInMemoryCache()
	ctx := context.Background()

	key := "test:listener"
	listener1 := &mockListener{}
	listener2 := &mockListener{}

	// Add multiple listeners
	cache.AddInvalidationListener(key, listener1)
	cache.AddInvalidationListener(key, listener2)

	// Trigger invalidation
	err := cache.Set(ctx, key, "value", time.Hour)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	// Both listeners should receive the event
	events1 := listener1.getEvents()
	events2 := listener2.getEvents()

	assert.Len(t, events1, 1)
	assert.Len(t, events2, 1)
	assert.Equal(t, events1[0].key, events2[0].key)

	// Remove one listener
	cache.RemoveInvalidationListener(key, listener1)

	listener1.reset()
	listener2.reset()

	// Trigger another invalidation
	err = cache.Set(ctx, key, "value2", time.Hour)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	// Only listener2 should receive the event
	events1 = listener1.getEvents()
	events2 = listener2.getEvents()

	assert.Len(t, events1, 0)
	assert.Len(t, events2, 1)
}

func TestInMemoryCache_ClearOperation(t *testing.T) {
	cache := NewInMemoryCache()
	ctx := context.Background()

	// Add some data
	keys := []string{"test:clear1", "test:clear2", "test:clear3"}
	for _, key := range keys {
		err := cache.Set(ctx, key, "value", time.Hour)
		require.NoError(t, err)
	}

	// Add listeners
	listener := &mockListener{}
	for _, key := range keys {
		cache.AddInvalidationListener(key, listener)
	}

	// Clear cache
	err := cache.Clear(ctx)
	require.NoError(t, err)

	// Verify all data is gone
	for _, key := range keys {
		data, exists := cache.Get(ctx, key)
		assert.False(t, exists)
		assert.Nil(t, data)
	}

	// Wait for invalidation events
	time.Sleep(10 * time.Millisecond)

	// All listeners should have received events
	events := listener.getEvents()
	assert.Len(t, events, len(keys))

	// Verify all keys were invalidated
	invalidatedKeys := make(map[string]bool)
	for _, event := range events {
		invalidatedKeys[event.key] = true
	}
	assert.Len(t, invalidatedKeys, len(keys))
}
