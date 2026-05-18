package relay

import (
	"context"

	corerelay "encore.app/core/relay"
	"gorm.io/gorm"
)

// Relay handles payment outbox events using the shared core relay.
type Relay struct {
	coreRelay *corerelay.Relay[PaymentEvent]
}

func NewRelay(db *gorm.DB, config corerelay.Config) *Relay {
	publisher := &PaymentPublisher{}
	processor := &corerelay.DefaultProcessor{}
	cleanup := &corerelay.DefaultCleanup{}

	coreRelay := corerelay.NewRelay(db, publisher, processor, cleanup, config, "payment-")

	return &Relay{
		coreRelay: coreRelay,
	}
}

func (r *Relay) Start(ctx context.Context) {
	r.coreRelay.Start(ctx)
}

func (r *Relay) Stop() {
	r.coreRelay.Stop()
}

func (r *Relay) GetMetrics() corerelay.MetricsSnapshot {
	return r.coreRelay.GetMetrics()
}

func (r *Relay) IsHealthy() bool {
	return r.coreRelay.IsHealthy()
}
