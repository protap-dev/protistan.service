package events

import (
	"context"

	"encore.app/booking/domain"
	eventscommon "encore.app/core/events"
)

const (
	producer string = "booking-service"
)

type EventEnvelope[T any] = eventscommon.EventEnvelope[T]

// CreateEventEnvelope creates a new event envelope with the provided data and metadata
func CreateEventEnvelope[T any](ctx context.Context, eventType string, data T) *EventEnvelope[T] {
	return eventscommon.CreateEventEnvelope(ctx, eventType, data, producer)
}

// GetBookingEventType returns the event type string for a booking status
func GetBookingEventType(status domain.BookingStatus) string {
	switch status {
	case domain.BookingRequested:
		return "booking.created"
	case domain.BookingOfferPending:
		return "booking.offer.pending"
	case domain.BookingOfferRejected:
		return "booking.offer.rejected"
	case domain.BookingAssigned:
		return "booking.assigned"
	case domain.BookingPendingQuote:
		return "booking.quote.pending"
	case domain.BookingQuoteProposed:
		return "booking.quote.proposed"
	case domain.BookingQuoteAccepted:
		return "booking.quote.accepted"
	case domain.BookingPaymentPending:
		return "booking.payment.pending"
	case domain.BookingConfirmed:
		return "booking.confirmed"
	case domain.BookingEnroute:
		return "booking.enroute"
	case domain.BookingInProgress:
		return "booking.in_progress"
	case domain.BookingCompleted:
		return "booking.completed"
	case domain.BookingCancelled:
		return "booking.cancelled"
	case domain.BookingClosed:
		return "booking.closed"
	default:
		return "booking.status"
	}
}
