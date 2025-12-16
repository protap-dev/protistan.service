package internal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"encore.app/booking/domain"
	"encore.app/core/cache"
	"encore.app/customers"
	"encore.app/user"
	"encore.dev/beta/errs"
	"encore.dev/types/uuid"
)

// ValidateBookingReferences validates referenced IDs for create booking
func ValidateBookingReferences(ctx context.Context, req *domain.CreateBookingRequest, userID string, logger ServiceLogger) error {
	if err := ValidateCustomerRole(ctx, logger); err != nil {
		return fmt.Errorf("customer validation failed: %w", err)
	}
	if err := ValidateCustomerAddressExists(ctx, req.CustomerAddressID, userID, logger); err != nil {
		return fmt.Errorf("invalid customer_address_id: %w", err)
	}
	if err := ValidateServiceCategoryExists(ctx, req.ServiceCategoryID); err != nil {
		return fmt.Errorf("invalid service_category_id: %w", err)
	}
	return nil
}

// ValidateCustomerRole ensures the user is a customer with complete profile
func ValidateCustomerRole(ctx context.Context, logger ServiceLogger) error {
	// Use user service API to get profile (same as booking handlers)
	profileResp, err := user.GetProfile(ctx)
	if err != nil {
		userID := ExtractUserIDFromContext()
		logger.Error(ctx, "failed to get user profile for role validation", err, map[string]any{
			"user_id": userID,
		})
		return fmt.Errorf("failed to validate user: %w", err)
	}
	if GetActiveRole(profileResp.User.ActiveRole) != "customer" {
		return errors.New("only customers can create bookings")
	}
	if !profileResp.User.ProfileComplete {
		return errors.New("customer profile must be complete to create bookings")
	}
	userID := ExtractUserIDFromContext()
	logger.Info(ctx, "customer role validation successful", map[string]any{
		"user_id":          userID,
		"user_role":        GetActiveRole(profileResp.User.ActiveRole),
		"profile_complete": profileResp.User.ProfileComplete,
	})
	return nil
}

// ValidateCustomerAddressExists checks if address exists and belongs to customer
func ValidateCustomerAddressExists(ctx context.Context, addressID string, customerID string, logger ServiceLogger) error {
	if addressID == "" {
		return errors.New("address id is required")
	}
	if customerID == "" {
		return errors.New("customer id is required")
	}

	addressUUID, err := uuid.FromString(addressID)
	if err != nil {
		return fmt.Errorf("invalid address id format: %w", err)
	}
	customerUUID, err := uuid.FromString(customerID)
	if err != nil {
		return fmt.Errorf("invalid customer id format: %w", err)
	}
	address, err := customers.ValidateAddressOwnership(ctx, addressUUID, customerUUID)
	if err != nil {
		logger.Error(ctx, "failed to validate address ownership", err, map[string]any{
			"address_id":  addressID,
			"customer_id": customerID,
		})
		return fmt.Errorf("customer address not found or not owned by customer: %w", err)
	}
	logger.Info(ctx, "customer address validation successful", map[string]any{
		"address_id":  addressID,
		"customer_id": customerID,
		"user_id":     address.UserID,
	})
	return nil
}

// ValidateServiceCategoryExists basic placeholder
func ValidateServiceCategoryExists(ctx context.Context, serviceCategoryID string) error {
	if serviceCategoryID == "" {
		return errors.New("service_category_id is required")
	}
	return nil
}

// ValidateIdempotency checks If-Match header against current booking version for idempotency
func ValidateIdempotency(ctx context.Context, bookingID string, expectedVersion int64, cacheManager cache.CacheManager) error {
	idempotencyKey := fmt.Sprintf("rematch:%s:%d", bookingID, expectedVersion)

	_, exists := cacheManager.Get(ctx, idempotencyKey)
	if exists {
		return nil
	}

	processingMarker := map[string]any{
		"status":           "processing",
		"started_at":       time.Now(),
		"booking_id":       bookingID,
		"expected_version": expectedVersion,
	}

	return cacheManager.Set(ctx, idempotencyKey, processingMarker, 2*time.Minute)
}

// validateRematchEligibility checks if a booking is in a state where rematch is allowed
func ValidateRematchEligibility(ctx context.Context, status domain.BookingStatus) error {
	eligibleStates := map[domain.BookingStatus]bool{
		domain.BookingOfferPending:  true,
		domain.BookingOfferRejected: true,
		domain.BookingAssigned:      true,
		domain.BookingPendingQuote:  true,
		domain.BookingQuoteRejected: true,
	}

	if !eligibleStates[status] {
		return errs.B().Code(errs.InvalidArgument).
			Msg("booking is not in a state where rematch is allowed").
			Meta("current_status", string(status)).
			Meta("allowed_states", "offer_pending, offer_rejected, assigned, pending_quote, quote_rejected").
			Err()
	}

	return nil
}
