package events

import (
	"context"
	"time"

	"encore.app/booking/domain"
	"encore.app/booking/internal"
)

// EventEnvelope wraps booking events with observability metadata for reliable saga tracing
type EventEnvelope[T any] struct {
	EventID       string    `json:"event_id"`
	EventType     string    `json:"event_type"`
	OccurredAt    time.Time `json:"occurred_at"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	CausationID   string    `json:"causation_id,omitempty"`
	Producer      string    `json:"producer"`
	Data          T         `json:"data"`
}

// EventMetadata contains request tracing information
type EventMetadata = internal.EventMetadata

const (
	producer string = "booking-service"
)

// ExtractEventMetadata extracts event metadata from context
func ExtractEventMetadata(ctx context.Context) (*EventMetadata, bool) {
	return internal.ExtractEventMetadata(ctx)
}

// WithEventMetadata adds event metadata to context
func WithEventMetadata(ctx context.Context, metadata *EventMetadata) context.Context {
	return internal.WithEventMetadata(ctx, metadata)
}

// CreateEventEnvelope creates a new event envelope with the provided data and metadata
func CreateEventEnvelope[T any](ctx context.Context, eventType string, data T) *EventEnvelope[T] {
	metadata, _ := ExtractEventMetadata(ctx)

	// Generate new correlation ID if not present in metadata
	correlationID := ""
	causationID := "" // Causation is the ID of the PREVIOUS event that caused this one

	if metadata != nil {
		correlationID = metadata.CorrelationID
		causationID = metadata.CausationID // Use causation from metadata (previous event's ID)
	}

	// If no correlation ID in metadata, generate new one (this is a root event)
	if correlationID == "" {
		correlationID = internal.GenerateRandomID()
	}

	// If no causation ID in metadata, leave it empty (this is a root event with no prior cause)
	// DO NOT generate a random causation ID - it should only be set if there's an actual previous event

	return &EventEnvelope[T]{
		EventID:       internal.GenerateRandomID(), // New unique ID for THIS event
		EventType:     eventType,
		OccurredAt:    time.Now(),
		CorrelationID: correlationID, // Same for all events in saga
		CausationID:   causationID,   // ID of previous event that caused this (empty for root)
		Producer:      producer,
		Data:          data,
	}
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
