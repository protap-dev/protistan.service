package repository

import (
	"context"
	"encoding/json"
	"time"

	eventscommon "encore.app/core/events"
	corerepo "encore.app/core/repository"
	"gorm.io/gorm"
)

// insertOutboxEvent writes a JSON-serialized event into the central outbox table.
func insertOutboxEvent[T any](db *gorm.DB, ctx context.Context, topic string, event T) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Create(&corerepo.OutboxEvent{
		Topic:       topic,
		Data:        data,
		InsertedAt:  time.Now(),
		ProcessedAt: nil,
	}).Error
}

func (r *transactionRepository) CreatePaymentEventInOutbox(ctx context.Context, topic string, event *eventscommon.EventEnvelope[eventscommon.PaymentEvent]) error {
	return insertOutboxEvent(r.coreDB, ctx, topic, event)
}
