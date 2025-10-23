package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"encore.app/core/repository"
	"gorm.io/gorm"
)

// Relay handles reading events from the outbox table and publishing them to topics
// This is the core generic implementation that all services can use
type Relay[EventType EventData] struct {
	db        *gorm.DB
	config    Config
	publisher Publisher[EventType]
	processor Processor
	cleanup   Cleanup
	metrics   *Metrics
	mu        sync.RWMutex
	stopping  bool
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// NewRelay creates a new relay instance with modular components
func NewRelay[EventType EventData](
	db *gorm.DB,
	publisher Publisher[EventType],
	processor Processor,
	cleanup Cleanup,
	config Config,
) *Relay[EventType] {
	return &Relay[EventType]{
		db:        db,
		config:    config,
		publisher: publisher,
		processor: processor,
		cleanup:   cleanup,
		metrics:   NewMetrics(),
		stopCh:    make(chan struct{}),
	}
}

// Start begins the relay processing loop
func (r *Relay[EventType]) Start(ctx context.Context) {
	r.mu.Lock()
	if r.stopping {
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()

	log.Println("Starting outbox relay...")
	r.wg.Add(1)
	go r.relayLoop(ctx)
}

// Stop gracefully shuts down the relay
func (r *Relay[EventType]) Stop() {
	r.mu.Lock()
	r.stopping = true
	r.mu.Unlock()

	close(r.stopCh)
	r.wg.Wait()
	log.Println("Outbox relay stopped")
}

// relayLoop is the main processing loop
func (r *Relay[EventType]) relayLoop(ctx context.Context) {
	defer r.wg.Done()

	ticker := time.NewTicker(r.config.PollingInterval)
	defer ticker.Stop()

	log.Printf("Relay polling every %v", r.config.PollingInterval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case <-ticker.C:
			if err := r.processBatch(ctx); err != nil {
				r.metrics.RecordProcessingError()
				log.Printf("Error processing outbox batch: %v", err)
			}
		}
	}
}

// processBatch processes a batch of pending events
func (r *Relay[EventType]) processBatch(ctx context.Context) error {
	start := time.Now()

	// Get unprocessed events from outbox table
	events, err := r.processor.GetUnprocessedEvents(ctx, r.db, r.config.BatchSize)
	if err != nil {
		return fmt.Errorf("failed to get unprocessed events: %w", err)
	}

	if len(events) == 0 {
		return nil // No events to process
	}

	log.Printf("Processing %d outbox events", len(events))

	// Process events in batch
	processed := 0
	failed := 0

	for _, outboxEvent := range events {
		if err := r.processEvent(ctx, outboxEvent); err != nil {
			r.metrics.RecordEventFailure()
			log.Printf("Failed to process outbox event %s: %v", outboxEvent.ID, err)
			failed++
		} else {
			processed++
		}
	}

	// Clean up old processed events for audit trail management
	if err := r.cleanup.CleanupOldEvents(ctx, r.db, r.config.AuditRetention); err != nil {
		log.Printf("Failed to cleanup old events: %v", err)
	}

	duration := time.Since(start)
	r.metrics.RecordProcessingTime(duration)
	log.Printf("Processed %d/%d events in %v (failed: %d)", processed, len(events), duration, failed)

	return nil
}

// processEvent processes a single outbox event
func (r *Relay[EventType]) processEvent(ctx context.Context, outboxEvent *repository.OutboxEvent) error {
	// Parse the event data
	var event EventType
	if err := json.Unmarshal(outboxEvent.Data, &event); err != nil {
		return fmt.Errorf("failed to unmarshal event data: %w", err)
	}

	// Publish to the appropriate topic based on event type
	if err := r.publisher.PublishToTopic(ctx, outboxEvent, event); err != nil {
		return fmt.Errorf("failed to publish to topic: %w", err)
	}

	// Mark event as processed (audit trail)
	now := time.Now()
	return r.processor.MarkEventProcessed(ctx, r.db, outboxEvent.ID, &now)
}

// GetMetrics returns current relay metrics (thread-safe)
func (r *Relay[EventType]) GetMetrics() MetricsSnapshot {
	return r.metrics.GetSnapshot()
}

// IsHealthy returns health status for monitoring
func (r *Relay[EventType]) IsHealthy() bool {
	return r.metrics.IsHealthy()
}
func (r *Relay[EventType]) ProcessEvent(ctx context.Context, outboxEvent *repository.OutboxEvent, event EventType) error {
	return r.processEvent(ctx, outboxEvent)
}
