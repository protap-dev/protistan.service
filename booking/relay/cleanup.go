package relay

import (
	"context"
	"log"
	"time"

	"encore.app/booking/repository"
	"gorm.io/gorm"
)

// Cleanup handles maintenance tasks like removing old processed events
type Cleanup interface {
	CleanupOldEvents(ctx context.Context, db *gorm.DB, retentionPeriod time.Duration) error
}

// DefaultCleanup implements the Cleanup interface
type DefaultCleanup struct{}

// CleanupOldEvents removes processed events older than the audit retention period
func (c *DefaultCleanup) CleanupOldEvents(ctx context.Context, db *gorm.DB, retentionPeriod time.Duration) error {
	cutoff := time.Now().Add(-retentionPeriod)
	result := db.WithContext(ctx).
		Where("processed_at IS NOT NULL AND processed_at < ?", cutoff).
		Delete(&repository.OutboxEvent{})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected > 0 {
		log.Printf("Cleaned up %d old processed events", result.RowsAffected)
	}

	return nil
}
