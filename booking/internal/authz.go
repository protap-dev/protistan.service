package internal

import (
	"context"

	"encore.app/booking/domain"
	"encore.dev/beta/errs"
)

// VerifyUserAccess ensures the user can access the given booking
func VerifyUserAccess(ctx context.Context, userID string, booking *domain.Booking) error {
	if booking.CustomerID == userID {
		return nil
	}
	if booking.ArtisanID != nil && *booking.ArtisanID == userID {
		return nil
	}
	return ErrPermissionDenied
}

// AuthorizeStatusUpdate validates whether the given role/user can update the booking status
func AuthorizeStatusUpdate(ctx context.Context, role string, userID string, booking *domain.Booking) error {
	switch role {
	case "customer":
		if booking.CustomerID != userID {
			return ErrPermissionDenied
		}
		return nil
	case "artisan":
		if booking.ArtisanID == nil || *booking.ArtisanID != userID {
			return ErrPermissionDenied
		}
		return nil
	case "admin":
		return nil
	default:
		return ErrPermissionDenied
	}
}

// AuthorizeCancel validates whether the user can cancel the booking
func AuthorizeCancel(ctx context.Context, userID string, booking *domain.Booking) error {
	if booking.CustomerID != userID {
		return ErrPermissionDenied
	}
	switch booking.Status {
	case domain.BookingPendingPayment,
		domain.BookingRequested,
		domain.BookingAccepted,
		domain.BookingEnroute:
		return nil
	case domain.BookingInProgress,
		domain.BookingCompleted,
		domain.BookingCancelled,
		domain.BookingClosed:
		return errs.B().Code(errs.FailedPrecondition).Msg("cannot cancel a booking in progress or completed").Err()
	default:
		return ErrValidationFailed
	}
}
