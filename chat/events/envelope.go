package events

import (
	"context"

	eventscommon "encore.app/core/events"
)

const producer string = "chat-service"

type EventEnvelope[T any] = eventscommon.EventEnvelope[T]

// CreateEventEnvelope creates a new event envelope with the provided data and metadata
func CreateEventEnvelope[T any](ctx context.Context, eventType string, data T) *EventEnvelope[T] {
	return eventscommon.CreateEventEnvelope(ctx, eventType, data, producer)
}
