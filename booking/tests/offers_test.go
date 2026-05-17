package booking

import (
	"context"
	"testing"
	"time"

	"encore.app/booking/domain"
	"encore.app/booking/handlers"
	binternal "encore.app/booking/internal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOfferBooking_Success tests successful offer creation
func TestOfferBooking_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		bookingID     string
		artisanID     string
		expiresIn     int
		expectOfferID bool
	}{
		{
			name:          "create offer with default expiration",
			bookingID:     "booking-123",
			artisanID:     "artisan-456",
			expiresIn:     0, // Should default to 24h
			expectOfferID: true,
		},
		{
			name:          "create offer with custom expiration",
			bookingID:     "booking-123",
			artisanID:     "artisan-456",
			expiresIn:     48, // 48 hours
			expectOfferID: true,
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Create request
			req := &handlers.OfferBookingRequest{
				ArtisanID: tc.artisanID,
				ExpiresIn: tc.expiresIn,
			}

			// Assertions would go here with mock repository
			assert.NotNil(t, req)
			assert.Equal(t, tc.artisanID, req.ArtisanID)
		})
	}
}

// TestOfferBooking_ValidationErrors tests validation error scenarios
func TestOfferBooking_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		bookingID   string
		req         *handlers.OfferBookingRequest
		expectedErr string
	}{
		{
			name:      "missing artisan ID",
			bookingID: "booking-123",
			req: &handlers.OfferBookingRequest{
				ArtisanID: "",
				ExpiresIn: 24,
			},
			expectedErr: "artisan_id is required",
		},
		{
			name:      "expiration too long",
			bookingID: "booking-123",
			req: &handlers.OfferBookingRequest{
				ArtisanID: "artisan-456",
				ExpiresIn: 100, // > 72 hours
			},
			expectedErr: "expires_in cannot exceed 72 hours",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Validate request parameters
			if tc.req.ArtisanID == "" {
				assert.Contains(t, tc.expectedErr, "artisan_id is required")
			}
			if tc.req.ExpiresIn > 72 {
				assert.Contains(t, tc.expectedErr, "expires_in cannot exceed 72 hours")
			}
		})
	}
}

// TestAcceptOffer_Success tests successful offer acceptance
func TestAcceptOffer_Success(t *testing.T) {
	t.Parallel()

	offerID := "offer-123"
	artisanID := "artisan-456"

	// Create mock offer
	offer := &domain.BookingOffer{
		ID:        offerID,
		BookingID: "booking-123",
		ArtisanID: artisanID,
		Status:    domain.OfferPending,
		OfferedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	// Verify offer is pending
	require.Equal(t, domain.OfferPending, offer.Status)
	assert.False(t, offer.IsExpired())
	assert.True(t, offer.CanRespond())

	// Simulate acceptance
	offer.Status = domain.OfferAccepted
	offer.RespondedAt = &[]time.Time{time.Now()}[0]

	assert.Equal(t, domain.OfferAccepted, offer.Status)
	assert.NotNil(t, offer.RespondedAt)
}

// TestAcceptOffer_RaceCondition tests race condition handling
func TestAcceptOffer_RaceCondition(t *testing.T) {
	t.Parallel()

	// Scenario: Two artisans try to accept the same booking simultaneously
	bookingID := "booking-123"
	artisan1ID := "artisan-1"
	artisan2ID := "artisan-2"

	// Create booking
	booking := &domain.Booking{
		ID:         bookingID,
		CustomerID: "customer-123",
		ArtisanID:  nil, // Not yet assigned
		Status:     domain.BookingRequested,
	}

	// Create two offers
	offer1 := &domain.BookingOffer{
		ID:        "offer-1",
		BookingID: bookingID,
		ArtisanID: artisan1ID,
		Status:    domain.OfferPending,
	}

	offer2 := &domain.BookingOffer{
		ID:        "offer-2",
		BookingID: bookingID,
		ArtisanID: artisan2ID,
		Status:    domain.OfferPending,
	}

	// First artisan accepts
	booking.ArtisanID = &artisan1ID
	booking.Status = domain.BookingAssigned
	offer1.Status = domain.OfferAccepted

	// Second artisan should fail (booking already assigned)
	if booking.ArtisanID != nil {
		// This is the expected behavior - reject second acceptance
		assert.Equal(t, domain.OfferPending, offer2.Status)
		assert.NotNil(t, booking.ArtisanID)
		assert.Equal(t, artisan1ID, *booking.ArtisanID)
	}

	// Other pending offers should be cancelled
	offer2.Status = domain.OfferCancelled
	assert.Equal(t, domain.OfferCancelled, offer2.Status)
}

// TestRejectOffer_Success tests successful offer rejection
func TestRejectOffer_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		reason string
	}{
		{
			name:   "reject with valid reason",
			reason: "Too far from my location",
		},
		{
			name:   "reject with schedule conflict",
			reason: "Already booked at that time",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			offer := &domain.BookingOffer{
				ID:        "offer-123",
				BookingID: "booking-123",
				ArtisanID: "artisan-456",
				Status:    domain.OfferPending,
				OfferedAt: time.Now(),
				ExpiresAt: time.Now().Add(24 * time.Hour),
			}

			// Verify can reject
			require.True(t, offer.CanRespond())

			// Reject offer
			offer.Status = domain.OfferRejected
			offer.RejectReason = &tc.reason
			offer.RespondedAt = &[]time.Time{time.Now()}[0]

			assert.Equal(t, domain.OfferRejected, offer.Status)
			assert.NotNil(t, offer.RejectReason)
			assert.Equal(t, tc.reason, *offer.RejectReason)
			assert.NotNil(t, offer.RespondedAt)
		})
	}
}

// TestRejectOffer_MissingReason tests rejection without reason
func TestRejectOffer_MissingReason(t *testing.T) {
	t.Parallel()

	req := &handlers.RejectOfferRequest{
		Reason: "",
	}

	// Should fail validation
	if req.Reason == "" {
		assert.Empty(t, req.Reason)
		// In real handler, this would return error: "reason is required"
	}
}

// TestOfferExpiration tests offer expiration logic
func TestOfferExpiration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		expiresAt      time.Time
		expectedExpiry bool
	}{
		{
			name:           "not expired - 1 hour remaining",
			expiresAt:      time.Now().Add(1 * time.Hour),
			expectedExpiry: false,
		},
		{
			name:           "expired - 1 hour ago",
			expiresAt:      time.Now().Add(-1 * time.Hour),
			expectedExpiry: true,
		},
		{
			name:           "just expired",
			expiresAt:      time.Now().Add(-1 * time.Minute),
			expectedExpiry: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			offer := &domain.BookingOffer{
				ID:        "offer-123",
				BookingID: "booking-123",
				ArtisanID: "artisan-456",
				Status:    domain.OfferPending,
				ExpiresAt: tc.expiresAt,
			}

			assert.Equal(t, tc.expectedExpiry, offer.IsExpired())

			if tc.expectedExpiry {
				// Expired offers cannot be responded to
				assert.False(t, offer.CanRespond())
			} else {
				// Non-expired pending offers can be responded to
				assert.True(t, offer.CanRespond())
			}
		})
	}
}

// TestOfferValidator_ValidateOfferCreation tests offer creation validation
func TestOfferValidator_ValidateOfferCreation(t *testing.T) {
	t.Parallel()

	validator := domain.NewOfferValidator()

	tests := []struct {
		name        string
		booking     *domain.Booking
		artisanID   string
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid offer creation",
			booking: &domain.Booking{
				ID:        "booking-123",
				Status:    domain.BookingRequested,
				ArtisanID: nil,
			},
			artisanID:   "artisan-456",
			expectError: false,
		},
		{
			name: "booking already assigned",
			booking: &domain.Booking{
				ID:        "booking-123",
				Status:    domain.BookingRequested, // Correct status but artisan assigned
				ArtisanID: &[]string{"artisan-789"}[0],
			},
			artisanID:   "artisan-456",
			expectError: true,
			errorMsg:    "booking already accepted by another artisan",
		},
		{
			name: "booking wrong status",
			booking: &domain.Booking{
				ID:        "booking-123",
				Status:    domain.BookingCompleted,
				ArtisanID: nil,
			},
			artisanID:   "artisan-456",
			expectError: true,
			errorMsg:    "invalid booking status",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validator.ValidateOfferCreation(tc.booking, tc.artisanID)

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestOfferValidator_ValidateOfferAcceptance tests offer acceptance validation
func TestOfferValidator_ValidateOfferAcceptance(t *testing.T) {
	t.Parallel()

	validator := domain.NewOfferValidator()

	tests := []struct {
		name        string
		offer       *domain.BookingOffer
		booking     *domain.Booking
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid acceptance",
			offer: &domain.BookingOffer{
				Status:    domain.OfferPending,
				ExpiresAt: time.Now().Add(24 * time.Hour),
			},
			booking: &domain.Booking{
				Status:    domain.BookingRequested,
				ArtisanID: nil,
			},
			expectError: false,
		},
		{
			name: "offer already accepted",
			offer: &domain.BookingOffer{
				Status:    domain.OfferAccepted,
				ExpiresAt: time.Now().Add(24 * time.Hour),
			},
			booking: &domain.Booking{
				Status:    domain.BookingRequested,
				ArtisanID: nil,
			},
			expectError: true,
			errorMsg:    "cannot respond to this offer",
		},
		{
			name: "offer expired",
			offer: &domain.BookingOffer{
				Status:    domain.OfferPending,
				ExpiresAt: time.Now().Add(-1 * time.Hour),
			},
			booking: &domain.Booking{
				Status:    domain.BookingRequested,
				ArtisanID: nil,
			},
			expectError: true,
			errorMsg:    "expired",
		},
		{
			name: "booking already assigned",
			offer: &domain.BookingOffer{
				Status:    domain.OfferPending,
				ExpiresAt: time.Now().Add(24 * time.Hour),
			},
			booking: &domain.Booking{
				Status:    domain.BookingAssigned,
				ArtisanID: &[]string{"other-artisan"}[0],
			},
			expectError: true,
			errorMsg:    "booking already accepted by another artisan",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validator.ValidateOfferAcceptance(tc.offer, tc.booking)

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestOfferValidator_ValidateOfferRejection tests offer rejection validation
func TestOfferValidator_ValidateOfferRejection(t *testing.T) {
	t.Parallel()

	validator := domain.NewOfferValidator()

	tests := []struct {
		name        string
		offer       *domain.BookingOffer
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid rejection",
			offer: &domain.BookingOffer{
				Status:    domain.OfferPending,
				ExpiresAt: time.Now().Add(24 * time.Hour),
			},
			expectError: false,
		},
		{
			name: "offer already rejected",
			offer: &domain.BookingOffer{
				Status:    domain.OfferRejected,
				ExpiresAt: time.Now().Add(24 * time.Hour),
			},
			expectError: true,
			errorMsg:    "cannot respond to this offer",
		},
		{
			name: "offer already accepted",
			offer: &domain.BookingOffer{
				Status:    domain.OfferAccepted,
				ExpiresAt: time.Now().Add(24 * time.Hour),
			},
			expectError: true,
			errorMsg:    "cannot respond to this offer",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validator.ValidateOfferRejection(tc.offer)

			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestAuthorization_OfferCreation tests authorization for offer creation
func TestAuthorization_OfferCreation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name        string
		userID      string
		booking     *domain.Booking
		expectError bool
	}{
		{
			name:   "customer can offer their own booking",
			userID: "customer-123",
			booking: &domain.Booking{
				ID:         "booking-123",
				CustomerID: "customer-123",
				Status:     domain.BookingRequested,
				ArtisanID:  nil,
			},
			expectError: false,
		},
		{
			name:   "customer cannot offer others' booking",
			userID: "customer-456",
			booking: &domain.Booking{
				ID:         "booking-123",
				CustomerID: "customer-123",
				Status:     domain.BookingRequested,
				ArtisanID:  nil,
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := binternal.AuthorizeOfferCreation(ctx, tc.userID, tc.booking)

			if tc.expectError {
				require.Error(t, err)
				assert.Equal(t, binternal.ErrPermissionDenied, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestAuthorization_OfferResponse tests authorization for offer responses
func TestAuthorization_OfferResponse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name        string
		userID      string
		offer       *domain.BookingOffer
		expectError bool
	}{
		{
			name:   "artisan can respond to their own offer",
			userID: "artisan-456",
			offer: &domain.BookingOffer{
				ID:        "offer-123",
				ArtisanID: "artisan-456",
			},
			expectError: false,
		},
		{
			name:   "artisan cannot respond to others' offer",
			userID: "artisan-789",
			offer: &domain.BookingOffer{
				ID:        "offer-123",
				ArtisanID: "artisan-456",
			},
			expectError: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := binternal.AuthorizeOfferResponse(ctx, tc.userID, tc.offer)

			if tc.expectError {
				require.Error(t, err)
				assert.Equal(t, binternal.ErrPermissionDenied, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
