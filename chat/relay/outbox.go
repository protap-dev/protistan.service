package relay

import (
	"context"
	"sync"

	corerelay "encore.app/core/relay"
	"gorm.io/gorm"
)

// OutboxRelay provides a simple interface for the modular outbox relay system
type OutboxRelay struct {
	relay   *Relay
	mu      sync.Mutex
	cancel  context.CancelFunc
	running bool
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

// StartAsync runs the relay loop in a managed goroutine and returns immediately.
func (r *OutboxRelay) StartAsync(ctx context.Context) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	childCtx, cancel := context.WithCancel(ctx)
	r.running = true
	r.cancel = cancel
	r.mu.Unlock()

	go func() {
		r.relay.Start(childCtx)
		r.mu.Lock()
		r.running = false
		r.cancel = nil
		r.mu.Unlock()
	}()
}

// Stop halts the relay processing loop gracefully.
func (r *OutboxRelay) Stop() {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	cancel := r.cancel
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	r.relay.Stop()
}

// IsRunning indicates whether the relay goroutine is currently active.
func (r *OutboxRelay) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}
