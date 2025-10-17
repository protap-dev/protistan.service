package domain

import "errors"

var (
	ErrInvalidStatus = errors.New("invalid booking status for this operation")
	ErrInvalidInput  = errors.New("invalid input provided")
)

// OfferValidator defines validation rules for booking offers
type OfferValidator interface {
	ValidateOfferCreation(booking *Booking, artisanID string) error
	ValidateOfferAcceptance(offer *BookingOffer, booking *Booking) error
	ValidateOfferRejection(offer *BookingOffer) error
}

// offerValidator implements the OfferValidator interface
type offerValidator struct{}

// NewOfferValidator creates a new offer validator
func NewOfferValidator() OfferValidator {
	return &offerValidator{}
}

// ValidateOfferCreation validates that a booking can be offered to an artisan
func (v *offerValidator) ValidateOfferCreation(booking *Booking, artisanID string) error {
	// Booking must be in requested status
	if booking.Status != BookingRequested {
		return ErrInvalidStatus
	}

	// Booking must not have assigned artisan
	if booking.ArtisanID != nil {
		return ErrOfferAlreadyTaken
	}

	// Artisan ID must be provided
	if artisanID == "" {
		return ErrInvalidInput
	}

	// TODO: Validate artisan exists and is qualified (integrate with artisan service)

	return nil
}

// ValidateOfferAcceptance validates that an offer can be accepted
func (v *offerValidator) ValidateOfferAcceptance(offer *BookingOffer, booking *Booking) error {
	// Offer must be pending
	if offer.Status != OfferPending {
		return ErrCannotRespondToOffer
	}

	// Offer must not be expired
	if offer.IsExpired() {
		return ErrOfferExpired
	}

	// Booking must not be assigned (race condition check)
	if booking.ArtisanID != nil {
		return ErrOfferAlreadyTaken
	}

	return nil
}

// ValidateOfferRejection validates that an offer can be rejected
func (v *offerValidator) ValidateOfferRejection(offer *BookingOffer) error {
	// Offer must be pending
	if offer.Status != OfferPending {
		return ErrCannotRespondToOffer
	}

	// Cannot reject expired offers
	if offer.IsExpired() {
		return ErrOfferExpired
	}

	return nil
}

