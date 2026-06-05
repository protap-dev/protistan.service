package internal

import (
	"context"

	"encore.app/booking/domain"
	"encore.dev/beta/errs"
)

// VerifyUserAccess ensures the user can access the given booking
func VerifyUserAccess(ctx context.Context, userID string, booking *domain.Booking) error {
	// Allow if user is the customer
	if booking.CustomerID == userID {
		return nil
	}

	// Allow if user is the assigned artisan
	if booking.ArtisanID != nil && *booking.ArtisanID == userID {
		return nil

	}

	return ErrPermissionDenied
}

// AuthorizeStatusUpdate validates whether the given role/user can perform a specific status transition.
func AuthorizeStatusUpdate(ctx context.Context, role string, userID string, booking *domain.Booking, targetStatus domain.BookingStatus) error {
	if role == "admin" {
		return nil
	}

	switch targetStatus {
	case domain.BookingCancelled:
		return AuthorizeCancel(ctx, userID, booking)
	case domain.BookingEnroute:
		return authorizeArtisanTransition(role, userID, booking, domain.BookingConfirmed)
	case domain.BookingInProgress:
		return authorizeArtisanTransition(role, userID, booking, domain.BookingEnroute)
	case domain.BookingCompletionPending:
		return authorizeArtisanTransition(role, userID, booking, domain.BookingInProgress)
	case domain.BookingCompleted:
		return authorizeCustomerTransition(role, userID, booking, domain.BookingCompletionPending)
	case domain.BookingClosed:
		return authorizeCustomerTransition(role, userID, booking, domain.BookingCompleted)
	default:
		return ErrPermissionDenied
	}
}

func authorizeArtisanTransition(role string, userID string, booking *domain.Booking, requiredStatus domain.BookingStatus) error {
	if role != "artisan" || booking.ArtisanID == nil || *booking.ArtisanID != userID {
		return ErrPermissionDenied
	}
	if booking.Status != requiredStatus {
		return errs.B().Code(errs.FailedPrecondition).Msg("booking is not ready for this status update").Err()
	}
	return nil
}

func authorizeCustomerTransition(role string, userID string, booking *domain.Booking, requiredStatus domain.BookingStatus) error {
	if role != "customer" || booking.CustomerID != userID {
		return ErrPermissionDenied
	}
	if booking.Status != requiredStatus {
		return errs.B().Code(errs.FailedPrecondition).Msg("booking is not ready for this status update").Err()
	}
	return nil
}

// AuthorizeCancel validates whether the user can cancel the booking
func AuthorizeCancel(ctx context.Context, userID string, booking *domain.Booking) error {
	if booking.CustomerID != userID {
		return ErrPermissionDenied
	}
	switch booking.Status {
	case domain.BookingRequested,
		domain.BookingOfferPending,
		domain.BookingOfferRejected,
		domain.BookingAssigned,
		domain.BookingPendingQuote,
		domain.BookingQuoteProposed,
		domain.BookingQuoteAccepted,
		domain.BookingPaymentPending,
		domain.BookingConfirmed,
		domain.BookingEnroute:
		return nil
	case domain.BookingInProgress,
		domain.BookingCompletionPending,
		domain.BookingCompleted,
		domain.BookingCancelled,
		domain.BookingClosed:
		return errs.B().Code(errs.FailedPrecondition).Msg("cannot cancel a booking in progress or completed").Err()
	default:
		return ErrValidationFailed
	}
}

// AuthorizeOfferCreation validates whether the user can offer a booking to an artisan
func AuthorizeOfferCreation(ctx context.Context, userID string, booking *domain.Booking) error {
	// Only the customer who owns the booking can offer it
	// Admin override handled in handler layer
	if booking.CustomerID != userID {
		return ErrPermissionDenied
	}

	// Booking must be in a state where it can be offered
	if booking.Status != domain.BookingRequested {
		return errs.B().
			Code(errs.FailedPrecondition).
			Msg("booking must be in requested status to be offered").
			Err()
	}

	// Booking must not already have an artisan assigned
	if booking.ArtisanID != nil {
		return errs.B().
			Code(errs.FailedPrecondition).
			Msg("booking already has an assigned artisan").
			Err()
	}

	return nil
}

// AuthorizeOfferResponse validates whether the user can respond to (accept/reject) an offer
func AuthorizeOfferResponse(ctx context.Context, userID string, offer *domain.BookingOffer) error {
	// Only the artisan to whom the offer was made can respond to it
	if offer.ArtisanID != userID {
		return ErrPermissionDenied
	}

	return nil
}

// AuthorizeViewOffers validates whether the user can view offers for a booking
func AuthorizeViewOffers(ctx context.Context, userID string, booking *domain.Booking) error {
	// Customer who owns the booking can view offers
	if booking.CustomerID == userID {
		return nil
	}

	// Assigned artisan can view offers (to see competing offers)
	if booking.ArtisanID != nil && *booking.ArtisanID == userID {
		return nil
	}

	// Admin can view (handled in handler layer)
	return ErrPermissionDenied
}
