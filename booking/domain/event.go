package domain

import (
	"context"
	"time"
)

// BookingEvent represents a state change event for a booking.
// It is used for pub/sub and for creating audit trail entries.
type BookingEvent struct {
	BookingID      string            `json:"booking_id"`
	Status         BookingStatus     `json:"status"`
	PreviousStatus BookingStatus     `json:"previous_status"`
	Timestamp      time.Time         `json:"timestamp"`
	UserID         string            `json:"user_id"`
	ArtisanID      *string           `json:"artisan_id,omitempty"`
	Reason         *string           `json:"reason,omitempty"`
	OfferID        *string           `json:"offer_id,omitempty"` // For offer-related events
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// RematchEvent represents a rematch request event for a booking.
// It is used when a customer wants to request a different artisan for their booking.
type RematchEvent struct {
	BookingID         string        `json:"booking_id"`
	PreviousArtisanID *string       `json:"previous_artisan_id,omitempty"` // Current artisan being replaced
	CurrentStatus     BookingStatus `json:"current_status"`                // Current booking status
	Reason            *string       `json:"reason,omitempty"`              // Optional reason for rematch
	Timestamp         time.Time     `json:"timestamp"`
	UserID            string        `json:"user_id"` // Customer requesting rematch
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

	// Rematch events
	PublishRematchRequestedEvent(ctx context.Context, event *RematchEvent)
}
