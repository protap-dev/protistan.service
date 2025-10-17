package relay

import (
	"context"
	"time"

	"encore.app/booking/repository"
	"gorm.io/gorm"
)

// Processor handles the core event processing logic
type Processor interface {
	GetUnprocessedEvents(ctx context.Context, db *gorm.DB, batchSize int) ([]*repository.OutboxEvent, error)
	MarkEventProcessed(ctx context.Context, db *gorm.DB, eventID string, processedAt *time.Time) error
}

// DefaultProcessor implements the Processor interface
type DefaultProcessor struct{}

// GetUnprocessedEvents retrieves events from outbox that haven't been processed yet
func (p *DefaultProcessor) GetUnprocessedEvents(ctx context.Context, db *gorm.DB, batchSize int) ([]*repository.OutboxEvent, error) {
	var events []*repository.OutboxEvent

	// Use raw SQL to avoid GORM schema dependency issues
	// This works regardless of whether processed_at column exists
	query := `
		SELECT id, topic, data, inserted_at, processed_at
		FROM outbox
		WHERE processed_at IS NULL
		ORDER BY inserted_at ASC
		LIMIT ?
	`

	err := db.WithContext(ctx).Raw(query, batchSize).Scan(&events).Error
	if err != nil {
		return nil, err
	}

	return events, nil
}

// MarkEventProcessed marks an event as processed with timestamp (audit trail)
func (p *DefaultProcessor) MarkEventProcessed(ctx context.Context, db *gorm.DB, eventID string, processedAt *time.Time) error {
	return db.WithContext(ctx).Model(&repository.OutboxEvent{}).
		Where("id = ?", eventID).
		Update("processed_at", processedAt).Error
}
