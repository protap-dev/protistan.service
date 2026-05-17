package relay

import (
	"context"

	corerelay "encore.app/core/relay"
	"gorm.io/gorm"
)

// OutboxRelay provides a service-facing wrapper for the payment outbox relay.
type OutboxRelay struct {
	relay *Relay
}

func NewOutboxRelay(db *gorm.DB, config ...corerelay.Config) *OutboxRelay {
	cfg := corerelay.DefaultConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	return &OutboxRelay{
		relay: NewRelay(db, cfg),
	}
}

func (r *OutboxRelay) Start(ctx context.Context) {
	r.relay.Start(ctx)
}

func (r *OutboxRelay) Stop() {
	r.relay.Stop()
}
