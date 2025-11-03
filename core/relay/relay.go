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
	db          *gorm.DB
	config      Config
	publisher   Publisher[EventType]
	processor   Processor
	cleanup     Cleanup
	metrics     *Metrics
	mu          sync.RWMutex
	stopping    bool
	stopCh      chan struct{}
	wg          sync.WaitGroup
	topicPrefix string
}

// NewRelay creates a new relay instance with modular components
func NewRelay[EventType EventData](
	db *gorm.DB,
	publisher Publisher[EventType],
	processor Processor,
	cleanup Cleanup,
	config Config,
	topicPrefix string,
) *Relay[EventType] {
	return &Relay[EventType]{
		db:          db,
		config:      config,
		publisher:   publisher,
		processor:   processor,
		cleanup:     cleanup,
		metrics:     NewMetrics(),
		stopCh:      make(chan struct{}),
		topicPrefix: topicPrefix,
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
	events, err := r.processor.GetUnprocessedEvents(ctx, r.db, r.config.BatchSize, r.topicPrefix)
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

			// Check if permanently failed
			if outboxEvent.Status == "failed" {
				log.Printf("⚠️  DEAD LETTER: Event %s permanently failed after %d retries. Topic: %s, Error: %s",
					outboxEvent.ID,
					outboxEvent.RetryCount,
					outboxEvent.Topic,
					*outboxEvent.LastError)
				// TODO: Send alert/notification for manual intervention
			} else {
				log.Printf("Scheduled retry for event %s (attempt %d/%d) at %v",
					outboxEvent.ID,
					outboxEvent.RetryCount+1,
					r.config.MaxRetries,
					outboxEvent.NextRetryAt)
			}
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
	claimed, err := r.claimEvent(ctx, outboxEvent.ID)
	if err != nil {
		return err
	}
	if !claimed {
		// Another worker claimed it, skip
		return nil
	}
	// Check if we've exceeded max retries
	if outboxEvent.RetryCount >= r.config.MaxRetries {
		log.Printf("Event %s exceeded max retries (%d), marking as failed",
			outboxEvent.ID, r.config.MaxRetries)
		return r.markEventFailed(ctx, outboxEvent, "exceeded max retries")
	}

	var event EventType
	if err := json.Unmarshal(outboxEvent.Data, &event); err != nil {
		// Permanent error - no point retrying
		return r.markEventFailed(ctx, outboxEvent, fmt.Sprintf("unmarshal error: %v", err))
	}

	// Attempt to publish
	if err := r.publisher.PublishToTopic(ctx, outboxEvent, event); err != nil {
		// Increment retry count with exponential backoff
		return r.scheduleRetry(ctx, outboxEvent, err)
	}

	// Success - mark as processed
	now := time.Now()
	return r.processor.MarkEventProcessed(ctx, r.db, outboxEvent.ID, &now)
}

func (r *Relay[EventType]) scheduleRetry(ctx context.Context, event *repository.OutboxEvent, err error) error {
	retryCount := event.RetryCount + 1

	// Calculate exponential backoff
	backoff := r.config.RetryBaseDelay * time.Duration(1<<uint(retryCount))
	if backoff > r.config.RetryMaxDelay {
		backoff = r.config.RetryMaxDelay
	}

	nextRetryAt := time.Now().Add(backoff)

	return r.db.WithContext(ctx).Model(&repository.OutboxEvent{}).
		Where("id = ?", event.ID).
		Updates(map[string]interface{}{
			"retry_count":   retryCount,
			"last_error":    err.Error(),
			"next_retry_at": nextRetryAt,
		}).Error
}

func (r *Relay[EventType]) claimEvent(ctx context.Context, eventID string) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&repository.OutboxEvent{}).
		Where("id = ? AND status = ?", eventID, "pending").
		Update("status", "processing")

	if result.Error != nil {
		return false, result.Error
	}

	return result.RowsAffected > 0, nil
}

func (r *Relay[EventType]) markEventFailed(ctx context.Context, event *repository.OutboxEvent, reason string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&repository.OutboxEvent{}).
		Where("id = ?", event.ID).
		Updates(map[string]interface{}{
			"status":       "failed",
			"last_error":   reason,
			"processed_at": now,
		}).Error
}

// GetMetrics returns current relay metrics (thread-safe)
func (r *Relay[EventType]) GetMetrics() MetricsSnapshot {
	return r.metrics.GetSnapshot()
}

// IsHealthy returns health status for monitoring
func (r *Relay[EventType]) IsHealthy() bool {
	return r.metrics.IsHealthy()
}
