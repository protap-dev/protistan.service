package repository

import (
	"time"
)

// OutboxEvent represents an event stored in the outbox table for guaranteed event publishing
// This is shared across all services to ensure consistent event handling
type OutboxEvent struct {
	ID          string     `gorm:"primaryKey;autoIncrement"`
	Topic       string     `gorm:"column:topic;not null"`
	Data        []byte     `gorm:"column:data;type:jsonb;not null"`
	InsertedAt  time.Time  `gorm:"column:inserted_at;not null;default:now()"`
	ProcessedAt *time.Time `gorm:"column:processed_at"` // track when event was processed
}

// TableName returns the table name for the OutboxEvent model
func (OutboxEvent) TableName() string {
	return "outbox"
}
