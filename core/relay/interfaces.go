package relay

import (
	"context"
	"time"

	"encore.app/core/repository"
	"gorm.io/gorm"
)

// EventData represents the interface that event data must implement
// Services should define their own event types that implement this interface
type EventData interface {
	// EventType returns the type of event (used for routing)
	EventType() string
}

// Processor handles the core event processing logic
type Processor interface {
	GetUnprocessedEvents(ctx context.Context, db *gorm.DB, batchSize int) ([]*repository.OutboxEvent, error)
	MarkEventProcessed(ctx context.Context, db *gorm.DB, eventID string, processedAt *time.Time) error
}

// Publisher handles publishing events to appropriate topics
// EventType is a generic type that services will specify
type Publisher[EventType EventData] interface {
	PublishToTopic(ctx context.Context, outboxEvent *repository.OutboxEvent, event EventType) error
}

// Cleanup handles maintenance tasks like removing old processed events
type Cleanup interface {
	CleanupOldEvents(ctx context.Context, db *gorm.DB, retentionPeriod time.Duration) error
}
