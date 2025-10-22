package relay

import (
	"context"

	"encore.app/booking/domain"
	"gorm.io/gorm"
)

// OutboxRelay provides a simple interface for the modular outbox relay system
type OutboxRelay struct {
	relay *Relay
}

// NewOutboxRelay creates a new outbox relay instance using the modular architecture
func NewOutboxRelay(db *gorm.DB, publisher domain.EventPublisher, config ...Config) *OutboxRelay {
	// Use centralized configuration if none provided
	cfg := DefaultConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	// Create modular components
	publisherImpl := &DefaultPublisher{}
	processorImpl := &DefaultProcessor{}
	cleanupImpl := &DefaultCleanup{}

	// Create the main relay orchestrator
	relayInstance := NewRelay(db, publisherImpl, processorImpl, cleanupImpl, cfg)

	return &OutboxRelay{
		relay: relayInstance,
	}
}

// Start begins the outbox relay processing loop
func (r *OutboxRelay) Start(ctx context.Context) {
	r.relay.Start(ctx)
}

// Stop gracefully shuts down the relay
func (r *OutboxRelay) Stop() {
	r.relay.Stop()
}

// GetMetrics returns current relay metrics (thread-safe)
func (r *OutboxRelay) GetMetrics() MetricsSnapshot {
	return r.relay.GetMetrics()
}

// IsHealthy returns health status for monitoring
func (r *OutboxRelay) IsHealthy() bool {
	return r.relay.IsHealthy()
}
