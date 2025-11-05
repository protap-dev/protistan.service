package repository

import (
	"time"
)

// OutboxEvent represents an event stored in the outbox table for guaranteed event publishing
// This is shared across all services to ensure consistent event handling
type OutboxEvent struct {
	ID          string     `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	Topic       string     `json:"topic" gorm:"type:text;not null;index"`
	Data        []byte     `json:"data" gorm:"type:jsonb;not null"`
	InsertedAt  time.Time  `json:"inserted_at" gorm:"not null;default:now()"`
	ProcessedAt *time.Time `json:"processed_at" gorm:"index"`
	RetryCount  int        `json:"retry_count" gorm:"default:0"`
	NextRetryAt *time.Time `json:"next_retry_at" gorm:"index"`
	LastError   *string    `json:"last_error" gorm:"type:text"`
	Status      string     `json:"status" gorm:"type:text;default:'pending';index"` // pending, processing, failed, processed
}

// TableName returns the table name for the OutboxEvent model
func (OutboxEvent) TableName() string {
	return "outbox"
}
