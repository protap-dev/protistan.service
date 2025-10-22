package relay

import (
	"sync"
	"time"
)

// Metrics tracks relay performance and health
type Metrics struct {
	EventsProcessed    int64
	EventsPublished    int64
	EventsFailed       int64
	ProcessingErrors   int64
	LastProcessingTime time.Time
	mu                 sync.RWMutex
}

// MetricsSnapshot is a thread-safe snapshot of relay metrics for external access
type MetricsSnapshot struct {
	EventsProcessed    int64
	EventsPublished    int64
	EventsFailed       int64
	ProcessingErrors   int64
	LastProcessingTime time.Time
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		LastProcessingTime: time.Now(),
	}
}

// RecordProcessingTime records the time taken for a processing cycle
func (m *Metrics) RecordProcessingTime(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastProcessingTime = time.Now()
}

// RecordEventFailure increments the failed events counter
func (m *Metrics) RecordEventFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EventsFailed++
}

// RecordProcessingError increments the processing errors counter
func (m *Metrics) RecordProcessingError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ProcessingErrors++
}

// GetSnapshot returns a thread-safe snapshot of current metrics
func (m *Metrics) GetSnapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return MetricsSnapshot{
		EventsProcessed:    m.EventsProcessed,
		EventsPublished:    m.EventsPublished,
		EventsFailed:       m.EventsFailed,
		ProcessingErrors:   m.ProcessingErrors,
		LastProcessingTime: m.LastProcessingTime,
	}
}

// IsHealthy returns true if the relay appears healthy (recent activity)
func (m *Metrics) IsHealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Consider unhealthy if no events processed in last 5 minutes
	return time.Since(m.LastProcessingTime) < 5*time.Minute
}
