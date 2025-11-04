package events

import (
	"context"
	"log"

	"encore.app/booking/domain"
	topics "encore.app/core/events/topics/booking"
)

// eventPublisher implements the domain.EventPublisher interface.
// It publishes events wrapped in envelopes for observability and tracing.
type eventPublisher struct{}

// NewEventPublisher creates a new event publisher.
func NewEventPublisher() domain.EventPublisher {
	return &eventPublisher{}
}

// PublishStatusEvent publishes a status change event wrapped in an envelope.
func (e *eventPublisher) PublishStatusEvent(ctx context.Context, event *domain.BookingEvent) {
	envelope := CreateEventEnvelope(ctx, GetBookingEventType(event.Status), *event)
	_, err := topics.BookingStatus.Publish(ctx, *envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish status event: %v", err)
	}
}

// PublishCancelledEvent publishes a booking cancelled event wrapped in an envelope.
func (e *eventPublisher) PublishCancelledEvent(ctx context.Context, event *domain.BookingEvent) {
	envelope := CreateEventEnvelope(ctx, "booking.cancelled", *event)
	_, err := CancelledTopic.Publish(ctx, *envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish cancelled event: %v", err)
	}
}

// PublishOfferCreatedEvent publishes when a booking is offered to an artisan.
func (e *eventPublisher) PublishOfferCreatedEvent(ctx context.Context, event *domain.BookingEvent) {
	envelope := CreateEventEnvelope(ctx, "booking.offered", *event)
	_, err := OfferedTopic.Publish(ctx, *envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish offer created event: %v", err)
	}
}

// PublishAssignedEvent publishes when an artisan is assigned to a booking.
func (e *eventPublisher) PublishAssignedEvent(ctx context.Context, event *domain.BookingEvent) {
	envelope := CreateEventEnvelope(ctx, "booking.assigned", *event)
	_, err := topics.BookingAssigned.Publish(ctx, *envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish assigned event: %v", err)
	}
}

// PublishRematchRequestedEvent publishes when a customer requests a rematch.
func (e *eventPublisher) PublishRematchRequestedEvent(ctx context.Context, event *domain.RematchEvent) {
	envelope := CreateEventEnvelope(ctx, "booking.v1.rematch.requested", *event)
	_, err := RematchTopic.Publish(ctx, *envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish rematch requested event: %v", err)
	}
}
