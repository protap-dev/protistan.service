package booking

import (
	"context"
	"testing"

	"encore.app/booking/domain"
	binternal "encore.app/booking/internal"
	"github.com/stretchr/testify/assert"
)

func TestIsValidBookingStatus(t *testing.T) {
	tests := []struct {
		name   string
		status domain.BookingStatus
		valid  bool
	}{
		{
			name:   "known lifecycle status",
			status: domain.BookingEnroute,
			valid:  true,
		},
		{
			name:   "known terminal status",
			status: domain.BookingClosed,
			valid:  true,
		},
		{
			name:   "unknown status",
			status: domain.BookingStatus("foo"),
			valid:  false,
		},
		{
			name:   "empty status",
			status: domain.BookingStatus(""),
			valid:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, domain.IsValidBookingStatus(tt.status))
		})
	}
}

func TestAuthorizeStatusUpdateLifecycleHandshake(t *testing.T) {
	ctx := context.Background()
	artisanID := "artisan-1"

	tests := []struct {
		name        string
		role        string
		userID      string
		current     domain.BookingStatus
		target      domain.BookingStatus
		expectError bool
	}{
		{
			name:        "artisan can move confirmed booking enroute",
			role:        "artisan",
			userID:      artisanID,
			current:     domain.BookingConfirmed,
			target:      domain.BookingEnroute,
			expectError: false,
		},
		{
			name:        "artisan can start work after arriving",
			role:        "artisan",
			userID:      artisanID,
			current:     domain.BookingEnroute,
			target:      domain.BookingInProgress,
			expectError: false,
		},
		{
			name:        "customer cannot move confirmed booking enroute",
			role:        "customer",
			userID:      "customer-1",
			current:     domain.BookingConfirmed,
			target:      domain.BookingEnroute,
			expectError: true,
		},
		{
			name:        "artisan requests completion instead of completing booking",
			role:        "artisan",
			userID:      artisanID,
			current:     domain.BookingInProgress,
			target:      domain.BookingCompletionPending,
			expectError: false,
		},
		{
			name:        "artisan cannot complete booking directly",
			role:        "artisan",
			userID:      artisanID,
			current:     domain.BookingInProgress,
			target:      domain.BookingCompleted,
			expectError: true,
		},
		{
			name:        "customer confirms requested completion",
			role:        "customer",
			userID:      "customer-1",
			current:     domain.BookingCompletionPending,
			target:      domain.BookingCompleted,
			expectError: false,
		},
		{
			name:        "customer can close completed booking",
			role:        "customer",
			userID:      "customer-1",
			current:     domain.BookingCompleted,
			target:      domain.BookingClosed,
			expectError: false,
		},
		{
			name:        "customer cannot cancel after completion request",
			role:        "customer",
			userID:      "customer-1",
			current:     domain.BookingCompletionPending,
			target:      domain.BookingCancelled,
			expectError: true,
		},
		{
			name:        "artisan cannot propose quote through public status endpoint",
			role:        "artisan",
			userID:      artisanID,
			current:     domain.BookingPendingQuote,
			target:      domain.BookingQuoteProposed,
			expectError: true,
		},
		{
			name:        "customer cannot accept quote through public status endpoint",
			role:        "customer",
			userID:      "customer-1",
			current:     domain.BookingQuoteProposed,
			target:      domain.BookingQuoteAccepted,
			expectError: true,
		},
		{
			name:        "customer cannot move payment pending through public status endpoint",
			role:        "customer",
			userID:      "customer-1",
			current:     domain.BookingQuoteAccepted,
			target:      domain.BookingPaymentPending,
			expectError: true,
		},
		{
			name:        "artisan cannot confirm booking through public status endpoint",
			role:        "artisan",
			userID:      artisanID,
			current:     domain.BookingPaymentPending,
			target:      domain.BookingConfirmed,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			booking := &domain.Booking{
				CustomerID: "customer-1",
				ArtisanID:  &artisanID,
				Status:     tt.current,
			}

			err := binternal.AuthorizeStatusUpdate(ctx, tt.role, tt.userID, booking, tt.target)
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
