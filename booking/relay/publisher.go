package relay

import (
	"context"

	"encore.app/booking/domain"
	"encore.app/booking/events"
	"encore.app/booking/repository"
)

// Publisher handles publishing events to appropriate topics
type Publisher interface {
	PublishToTopic(ctx context.Context, outboxEvent *repository.OutboxEvent, event *domain.BookingEvent) error
}

// DefaultPublisher implements the Publisher interface
type DefaultPublisher struct{}

// PublishToTopic publishes the event to the correct topic based on the stored topic name in outbox
func (p *DefaultPublisher) PublishToTopic(ctx context.Context, outboxEvent *repository.OutboxEvent, event *domain.BookingEvent) error {
	// Use the stored topic name from outbox table for routing
	switch outboxEvent.Topic {
	case "booking.status":
		_, err := events.StatusTopic.Publish(ctx, event)
		return err
	case "booking.created":
		_, err := events.CreatedTopic.Publish(ctx, event)
		return err
	case "booking.cancelled":
		_, err := events.CancelledTopic.Publish(ctx, event)
		return err
	case "booking.offered":
		_, err := events.OfferedTopic.Publish(ctx, event)
		return err
	case "booking.assigned":
		_, err := events.AssignedTopic.Publish(ctx, event)
		return err
	case "booking.quote.accepted":
		_, err := events.QuoteAcceptedTopic.Publish(ctx, event)
		return err
	case "booking.quote.rejected":
		_, err := events.QuoteRejectedTopic.Publish(ctx, event)
		return err
	case "booking.payment.confirmed":
		_, err := events.PaymentConfirmedTopic.Publish(ctx, event)
		return err
	default:
		// Default to status topic for unknown topic names
		_, err := events.StatusTopic.Publish(ctx, event)
		return err
	}
}
