package relay

import (
	"context"

	"encore.app/booking/domain"
	"encore.app/booking/events"
	topics "encore.app/core/events/topics/booking"
	"encore.app/core/repository"
)

// BookingEvent implements the core EventData interface
type BookingEvent struct {
	events.EventEnvelope[domain.BookingEvent]
}

// EventType returns the event type for routing
func (e BookingEvent) EventType() string {
	return "booking"
}

// BookingPublisher implements the core relay Publisher interface for booking events
type BookingPublisher struct{}

// PublishToTopic publishes the booking event to the correct topic based on the stored topic name in outbox
func (p *BookingPublisher) PublishToTopic(ctx context.Context, outboxEvent *repository.OutboxEvent, event BookingEvent) error {
	// Extract the envelope from the wrapper
	envelope := event.EventEnvelope

	// Use the stored topic name from outbox table for routing
	switch outboxEvent.Topic {
	case "booking.status":
		_, err := topics.BookingStatus.Publish(ctx, envelope)
		return err
	case "booking.cancelled":
		_, err := events.CancelledTopic.Publish(ctx, envelope)
		return err
	case "booking.offered":
		_, err := events.OfferedTopic.Publish(ctx, envelope)
		return err
	case "booking.assigned":
		_, err := topics.BookingAssigned.Publish(ctx, envelope)
		return err
	default:
		// Default to status topic for unknown topic names
		_, err := topics.BookingStatus.Publish(ctx, envelope)
		return err
	}
}
