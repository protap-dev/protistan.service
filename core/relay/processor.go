package relay

import (
	"context"
	"time"

	"encore.app/core/repository"
	"gorm.io/gorm"
)

// DefaultProcessor implements the Processor interface
type DefaultProcessor struct{}

// GetUnprocessedEvents retrieves events from outbox that haven't been processed yet
func (p *DefaultProcessor) GetUnprocessedEvents(ctx context.Context, db *gorm.DB, batchSize int) ([]*repository.OutboxEvent, error) {
	var events []*repository.OutboxEvent

	now := time.Now()

	err := db.WithContext(ctx).
		Model(&repository.OutboxEvent{}).
		Where("processed_at IS NULL").
		Where("status IN (?)", []string{"pending", "processing"}).
		Where("(next_retry_at IS NULL OR next_retry_at <= ?)", now).
		Order("inserted_at ASC").
		Limit(batchSize).
		Find(&events).Error

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
