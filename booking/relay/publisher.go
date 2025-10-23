package relay

import (
	"context"

	"encore.app/booking/domain"
	"encore.app/booking/events"
	"encore.app/core/repository"
)

// BookingEvent implements the core EventData interface
// This makes BookingEvent compatible with the core relay system
type BookingEvent struct {
	domain.BookingEvent
}

// EventType returns the event type for routing
func (e BookingEvent) EventType() string {
	// This could be enhanced to return different types based on the event
	return "booking"
}

// BookingPublisher implements the core relay Publisher interface for booking events
type BookingPublisher struct{}

// PublishToTopic publishes the booking event to the correct topic based on the stored topic name in outbox
func (p *BookingPublisher) PublishToTopic(ctx context.Context, outboxEvent *repository.OutboxEvent, event BookingEvent) error {
	// Use the stored topic name from outbox table for routing
	switch outboxEvent.Topic {
	case "booking.status":
		_, err := events.StatusTopic.Publish(ctx, events.CreateEventEnvelope(ctx, "booking.status", event.BookingEvent))
		return err
	case "booking.created":
		_, err := events.CreatedTopic.Publish(ctx, events.CreateEventEnvelope(ctx, "booking.created", event.BookingEvent))
		return err
	case "booking.cancelled":
		_, err := events.CancelledTopic.Publish(ctx, events.CreateEventEnvelope(ctx, "booking.cancelled", event.BookingEvent))
		return err
	case "booking.offered":
		_, err := events.OfferedTopic.Publish(ctx, events.CreateEventEnvelope(ctx, "booking.offered", event.BookingEvent))
		return err
	case "booking.assigned":
		_, err := events.AssignedTopic.Publish(ctx, events.CreateEventEnvelope(ctx, "booking.assigned", event.BookingEvent))
		return err
	case "booking.offer.rejected":
		_, err := events.OfferRejectedTopic.Publish(ctx, events.CreateEventEnvelope(ctx, "booking.offer.rejected", event.BookingEvent))
		return err
	case "booking.v1.offer.expired":
		_, err := events.OfferExpiredTopic.Publish(ctx, events.CreateEventEnvelope(ctx, "booking.v1.offer.expired", event.BookingEvent))
		return err
	default:
		// Default to status topic for unknown topic names
		_, err := events.StatusTopic.Publish(ctx, events.CreateEventEnvelope(ctx, "booking.status", event.BookingEvent))
		return err
	}
}
