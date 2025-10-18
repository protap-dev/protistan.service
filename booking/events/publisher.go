package events

import (
	"context"
	"log"

	"encore.app/booking/domain"
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
	_, err := StatusTopic.Publish(ctx, envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish status event: %v", err)
	}
}

// PublishCreatedEvent publishes a booking created event wrapped in an envelope.
func (e *eventPublisher) PublishCreatedEvent(ctx context.Context, event *domain.BookingEvent) {
	envelope := CreateEventEnvelope(ctx, "booking.created", *event)
	_, err := CreatedTopic.Publish(ctx, envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish created event: %v", err)
	}
}

// PublishCancelledEvent publishes a booking cancelled event wrapped in an envelope.
func (e *eventPublisher) PublishCancelledEvent(ctx context.Context, event *domain.BookingEvent) {
	envelope := CreateEventEnvelope(ctx, "booking.cancelled", *event)
	_, err := CancelledTopic.Publish(ctx, envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish cancelled event: %v", err)
	}
}

// PublishOfferCreatedEvent publishes when a booking is offered to an artisan.
func (e *eventPublisher) PublishOfferCreatedEvent(ctx context.Context, event *domain.BookingEvent) {
	envelope := CreateEventEnvelope(ctx, "booking.offered", *event)
	_, err := OfferedTopic.Publish(ctx, envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish offer created event: %v", err)
	}
}

// PublishAssignedEvent publishes when an artisan is assigned to a booking.
func (e *eventPublisher) PublishAssignedEvent(ctx context.Context, event *domain.BookingEvent) {
	envelope := CreateEventEnvelope(ctx, "booking.assigned", *event)
	_, err := AssignedTopic.Publish(ctx, envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish assigned event: %v", err)
	}
}

// PublishOfferRejectedEvent publishes when an artisan rejects a booking offer.
func (e *eventPublisher) PublishOfferRejectedEvent(ctx context.Context, event *domain.BookingEvent) {
	envelope := CreateEventEnvelope(ctx, "booking.offer.rejected", *event)
	_, err := StatusTopic.Publish(ctx, envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish offer rejected event: %v", err)
	}
}

// PublishQuoteAcceptedEvent publishes when a quote is accepted.
func (e *eventPublisher) PublishQuoteAcceptedEvent(ctx context.Context, event *domain.BookingEvent) {
	envelope := CreateEventEnvelope(ctx, "booking.quote.accepted", *event)
	_, err := QuoteAcceptedTopic.Publish(ctx, envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish quote accepted event: %v", err)
	}
}

// PublishQuoteRejectedEvent publishes when a quote is rejected.
func (e *eventPublisher) PublishQuoteRejectedEvent(ctx context.Context, event *domain.BookingEvent) {
	envelope := CreateEventEnvelope(ctx, "booking.quote.rejected", *event)
	_, err := QuoteRejectedTopic.Publish(ctx, envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish quote rejected event: %v", err)
	}
}

// PublishPaymentConfirmedEvent publishes when payment is confirmed.
func (e *eventPublisher) PublishPaymentConfirmedEvent(ctx context.Context, event *domain.BookingEvent) {
	envelope := CreateEventEnvelope(ctx, "booking.payment.confirmed", *event)
	_, err := PaymentConfirmedTopic.Publish(ctx, envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish payment confirmed event: %v", err)
	}
}
