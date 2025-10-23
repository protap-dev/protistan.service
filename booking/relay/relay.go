package relay

import (
	"context"

	"encore.app/core/relay"
	"gorm.io/gorm"
)

// Relay handles reading events from the outbox table and publishing them to topics
// This is the booking-specific wrapper that uses the core relay implementation
type Relay struct {
	coreRelay *relay.Relay[BookingEvent]
}

// NewRelay creates a new relay instance using the core relay implementation
func NewRelay(db *gorm.DB, config relay.Config) *Relay {
	// Create booking-specific components
	publisher := &BookingPublisher{}
	processor := &relay.DefaultProcessor{}
	cleanup := &relay.DefaultCleanup{}

	// Create the core relay with booking-specific types
	coreRelay := relay.NewRelay(db, publisher, processor, cleanup, config)

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
