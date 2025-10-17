package domain

import (
	"context"
	"time"
)

// BookingEvent represents a state change event for a booking.
// It is used for pub/sub and for creating audit trail entries.
type BookingEvent struct {
	BookingID      string        `json:"booking_id"`
	Status         BookingStatus `json:"status"`
	PreviousStatus BookingStatus `json:"previous_status"`
	Timestamp      time.Time     `json:"timestamp"`
	UserID         string        `json:"user_id"`
	ArtisanID      *string       `json:"artisan_id,omitempty"`
	Reason         *string       `json:"reason,omitempty"`
}

// EventPublisher defines the interface for publishing booking-related events.
// By defining this in the domain, we decouple the application core from the pub/sub implementation
type EventPublisher interface {
	PublishStatusEvent(ctx context.Context, event *BookingEvent)
	PublishCreatedEvent(ctx context.Context, event *BookingEvent)
	PublishCancelledEvent(ctx context.Context, event *BookingEvent)
	
	// Offer-related events
	PublishOfferCreatedEvent(ctx context.Context, event *BookingEvent)
	PublishAssignedEvent(ctx context.Context, event *BookingEvent)
	PublishOfferRejectedEvent(ctx context.Context, event *BookingEvent)

	// Quote and payment events
	PublishQuoteAcceptedEvent(ctx context.Context, event *BookingEvent)
	PublishQuoteRejectedEvent(ctx context.Context, event *BookingEvent)
	PublishPaymentConfirmedEvent(ctx context.Context, event *BookingEvent)
}
