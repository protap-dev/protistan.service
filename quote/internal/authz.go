package internal

import (
	"context"
	"slices"

	"encore.app/artisans"
	"encore.dev/beta/errs"
)

// AuthorizeProposeQuote checks if an artisan is permitted to propose a quote for a booking.
func AuthorizeProposeQuote(ctx context.Context, userID string, bookingArtisanID *string, bookingStatus string) error {
	// 1. Verify user is an artisan
	artisanResp, err := artisans.GetArtisanIDByUserID(ctx, userID)
	if err != nil {
		return errs.B().Code(errs.Internal).Msg("failed to get artisan profile").Err()
	}
	if !artisanResp.Found {
		return errs.B().Code(errs.PermissionDenied).Msg("only artisans can propose quotes").Err()
	}
	artisanID := artisanResp.ArtisanID

	// 2. Verify artisan is assigned to this booking
	if bookingArtisanID == nil || *bookingArtisanID != artisanID {
		return errs.B().Code(errs.PermissionDenied).Msg("not assigned to this booking").Err()
	}

	// 3. Verify booking is in a state where quotes can be proposed
	validStates := []string{"assigned", "pending_quote", "quote_rejected"}
	if !slices.Contains(validStates, bookingStatus) {
		return errs.B().
			Code(errs.FailedPrecondition).
			Msgf("cannot propose quote for booking in state '%s'", bookingStatus).
			Err()
	}

	return nil
}

// AuthorizeAcceptQuote checks if a user is permitted to accept a quote.
func AuthorizeAcceptQuote(ctx context.Context, userID string, bookingCustomerID string) error {
	// Only the customer who owns the booking can accept the quote.
	if bookingCustomerID != userID {
		return errs.B().Code(errs.PermissionDenied).Msg("only booking owner can accept quote").Err()
	}
	return nil
}

// AuthorizeRejectQuote checks if a user is permitted to reject a quote.
func AuthorizeRejectQuote(ctx context.Context, userID string, bookingCustomerID string) error {
	// Only the customer who owns the booking can reject the quote.
	if bookingCustomerID != userID {
		return errs.B().Code(errs.PermissionDenied).Msg("only booking owner can reject quote").Err()
	}
	return nil
}
