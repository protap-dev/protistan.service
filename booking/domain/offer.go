package domain

import (
	"errors"
	"time"
)

// BookingOfferStatus represents the state of an offer to an artisan
type BookingOfferStatus string

const (
	OfferPending   BookingOfferStatus = "pending"
	OfferAccepted  BookingOfferStatus = "accepted"
	OfferRejected  BookingOfferStatus = "rejected"
	OfferExpired   BookingOfferStatus = "expired"
	OfferCancelled BookingOfferStatus = "cancelled"
)

// BookingOffer represents an offer to a specific artisan
type BookingOffer struct {
	ID           string             `json:"id"`
	BookingID    string             `json:"booking_id"`
	ArtisanID    string             `json:"artisan_id"`
	Status       BookingOfferStatus `json:"status"`
	OfferedBy    string             `json:"offered_by"` // UserID who made the offer
	OfferedAt    time.Time          `json:"offered_at"`
	ExpiresAt    time.Time          `json:"expires_at"`
	RespondedAt  *time.Time         `json:"responded_at,omitempty"`
	RejectReason *string            `json:"reject_reason,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

// IsExpired checks if the offer has expired
func (o *BookingOffer) IsExpired() bool {
	return time.Now().After(o.ExpiresAt) && o.Status == OfferPending
}

// CanRespond checks if the artisan can still respond to the offer
func (o *BookingOffer) CanRespond() bool {
	return o.Status == OfferPending && !o.IsExpired()
}

// Domain errors for offers
var (
	ErrOfferNotFound        = errors.New("offer not found")
	ErrOfferExpired         = errors.New("offer has expired")
	ErrOfferAlreadyTaken    = errors.New("booking already accepted by another artisan")
	ErrCannotRespondToOffer = errors.New("cannot respond to this offer")
)
