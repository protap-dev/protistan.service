package relay

import (
	"context"

	corerelay "encore.app/core/relay"
	"gorm.io/gorm"
)

// OutboxRelay provides a simple interface for the modular outbox relay system
type OutboxRelay struct {
	relay *Relay
}

// NewOutboxRelay creates a new outbox relay instance using the modular architecture
func NewOutboxRelay(db *gorm.DB, config ...corerelay.Config) *OutboxRelay {
	// Use centralized configuration if none provided
	cfg := corerelay.DefaultConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	// Create the main relay orchestrator
	relayInstance := NewRelay(db, cfg)

	return &OutboxRelay{
		relay: relayInstance,
	}
}

// Start begins the outbox relay processing loop
func (r *OutboxRelay) Start(ctx context.Context) {
	r.relay.Start(ctx)
}
