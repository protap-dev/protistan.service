package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"encore.app/booking/domain"
	"encore.app/booking/repository"
	"gorm.io/gorm"
)

// Relay handles reading events from the outbox table and publishing them to topics
type Relay struct {
	db        *gorm.DB
	config    Config
	publisher Publisher
	processor Processor
	cleanup   Cleanup
	metrics   *Metrics
	mu        sync.RWMutex
	stopping  bool
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// NewRelay creates a new relay instance with modular components
func NewRelay(db *gorm.DB, publisher Publisher, processor Processor, cleanup Cleanup, config Config) *Relay {
	return &Relay{
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
func (r *Relay) Start(ctx context.Context) {
	r.mu.Lock()
	if r.stopping {
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()

	log.Println("Starting modular outbox relay...")
	r.wg.Add(1)
	go r.relayLoop(ctx)
}

// Stop gracefully shuts down the relay
func (r *Relay) Stop() {
	r.mu.Lock()
	r.stopping = true
	r.mu.Unlock()

	close(r.stopCh)
	r.wg.Wait()
	log.Println("Modular outbox relay stopped")
}

// relayLoop is the main processing loop
func (r *Relay) relayLoop(ctx context.Context) {
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
func (r *Relay) processBatch(ctx context.Context) error {
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
func (r *Relay) processEvent(ctx context.Context, outboxEvent *repository.OutboxEvent) error {
	// Parse the event data
	var bookingEvent domain.BookingEvent
	if err := json.Unmarshal(outboxEvent.Data, &bookingEvent); err != nil {
		return fmt.Errorf("failed to unmarshal event data: %w", err)
	}

	// Publish to the appropriate topic based on stored topic name
	if err := r.publisher.PublishToTopic(ctx, outboxEvent, &bookingEvent); err != nil {
		return fmt.Errorf("failed to publish to topic: %w", err)
	}

	// Mark event as processed (audit trail)
	now := time.Now()
	return r.processor.MarkEventProcessed(ctx, r.db, outboxEvent.ID, &now)
}

// GetMetrics returns current relay metrics (thread-safe)
func (r *Relay) GetMetrics() MetricsSnapshot {
	return r.metrics.GetSnapshot()
}

// IsHealthy returns health status for monitoring
func (r *Relay) IsHealthy() bool {
	return r.metrics.IsHealthy()
}
