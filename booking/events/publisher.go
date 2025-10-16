package events

import (
	"context"
	"log"

	"encore.app/booking/domain"
)

// eventPublisher implements the domain.EventPublisher interface.
// It is a stateless service that publishes to the global topics defined in booking.go.
type eventPublisher struct{}

// NewEventPublisher creates a new stateless event publisher.
func NewEventPublisher() domain.EventPublisher {
	return &eventPublisher{}
}

// PublishStatusEvent publishes a status change event.
func (e *eventPublisher) PublishStatusEvent(ctx context.Context, event *domain.BookingEvent) {
	_, err := StatusTopic.Publish(ctx, event)
	if err != nil {
		log.Printf("ERROR: failed to publish status event: %v", err)
	}
}

// PublishCreatedEvent publishes a booking created event.
func (e *eventPublisher) PublishCreatedEvent(ctx context.Context, event *domain.BookingEvent) {
	_, err := CreatedTopic.Publish(ctx, event)
	if err != nil {
		log.Printf("ERROR: failed to publish created event: %v", err)
	}
}

// PublishCancelledEvent publishes a booking cancelled event.
func (e *eventPublisher) PublishCancelledEvent(ctx context.Context, event *domain.BookingEvent) {
	_, err := CancelledTopic.Publish(ctx, event)
	if err != nil {
		log.Printf("ERROR: failed to publish cancelled event: %v", err)
	}
}
