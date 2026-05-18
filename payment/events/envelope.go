package events

import (
	"context"

	eventscommon "encore.app/core/events"
)

const producer string = "payment-service"

type EventEnvelope[T any] = eventscommon.EventEnvelope[T]

// CreateEventEnvelope creates a new event envelope with payment service metadata.
func CreateEventEnvelope[T any](ctx context.Context, eventType string, data T) *EventEnvelope[T] {
	return eventscommon.CreateEventEnvelope(ctx, eventType, data, producer)
}
