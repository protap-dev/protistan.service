package relay

import (
	"context"

	"encore.app/core/relay"
	"gorm.io/gorm"
)

// Relay handles reading events from the outbox table and publishing them to topics
// This is the quote-specific wrapper that uses the core relay implementation
type Relay struct {
	coreRelay *relay.Relay[QuoteEvent]
}

// NewRelay creates a new relay instance using the core relay implementation
func NewRelay(db *gorm.DB, config relay.Config) *Relay {
	// Create quote-specific components
	publisher := &QuotePublisher{}
	processor := &relay.DefaultProcessor{}
	cleanup := &relay.DefaultCleanup{}
	topicPrefix := "quote-"

	// Create the core relay with quote-specific types
	coreRelay := relay.NewRelay(db, publisher, processor, cleanup, config, topicPrefix)

	return &Relay{
		coreRelay: coreRelay,
	}
}

// Start begins the relay processing loop
func (r *Relay) Start(ctx context.Context) {
	r.coreRelay.Start(ctx)
}

// Stop gracefully shuts down the relay
func (r *Relay) Stop() {
	r.coreRelay.Stop()
}

// GetMetrics returns current relay metrics (thread-safe)
func (r *Relay) GetMetrics() relay.MetricsSnapshot {
	return r.coreRelay.GetMetrics()
}

// IsHealthy returns health status for monitoring
func (r *Relay) IsHealthy() bool {
	return r.coreRelay.IsHealthy()
}
