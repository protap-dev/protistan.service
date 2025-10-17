package events

import (
	"context"
	"log"

	"encore.app/booking/domain"
)

// eventPublisher implements the domain.EventPublisher interface.
// It publishes events immediately to pub/sub topics.
// For transactional publishing, use repository.CreateEventInOutbox() instead.
type eventPublisher struct{}

// NewEventPublisher creates a new event publisher.
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

// PublishOfferCreatedEvent publishes when a booking is offered to an artisan.
func (e *eventPublisher) PublishOfferCreatedEvent(ctx context.Context, event *domain.BookingEvent) {
	_, err := OfferedTopic.Publish(ctx, event)
	if err != nil {
		log.Printf("ERROR: failed to publish offer created event: %v", err)
	}
}

// PublishAssignedEvent publishes when an artisan is assigned to a booking.
func (e *eventPublisher) PublishAssignedEvent(ctx context.Context, event *domain.BookingEvent) {
	_, err := AssignedTopic.Publish(ctx, event)
	if err != nil {
		log.Printf("ERROR: failed to publish assigned event: %v", err)
	}
}

// PublishOfferRejectedEvent publishes when an artisan rejects a booking offer.
func (e *eventPublisher) PublishOfferRejectedEvent(ctx context.Context, event *domain.BookingEvent) {
	_, err := StatusTopic.Publish(ctx, event)
	if err != nil {
		log.Printf("ERROR: failed to publish offer rejected event: %v", err)
	}
}

func (e *eventPublisher) PublishQuoteAcceptedEvent(ctx context.Context, event *domain.BookingEvent) {
	_, err := QuoteAcceptedTopic.Publish(ctx, event)
	if err != nil {
		log.Printf("ERROR: failed to publish quote accepted event: %v", err)
	}
}

func (e *eventPublisher) PublishQuoteRejectedEvent(ctx context.Context, event *domain.BookingEvent) {
	_, err := QuoteRejectedTopic.Publish(ctx, event)
	if err != nil {
		log.Printf("ERROR: failed to publish quote rejected event: %v", err)
	}
}

func (e *eventPublisher) PublishPaymentConfirmedEvent(ctx context.Context, event *domain.BookingEvent) {
	_, err := PaymentConfirmedTopic.Publish(ctx, event)
	if err != nil {
		log.Printf("ERROR: failed to publish payment confirmed event: %v", err)
	}
}
