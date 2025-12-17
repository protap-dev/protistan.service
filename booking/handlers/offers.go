package handlers

import (
	"context"
	"fmt"
	"time"

	"encore.app/booking/domain"
	binternal "encore.app/booking/internal"
	"encore.dev/beta/errs"
	"encore.dev/types/uuid"
)

// =============================
// Offer Handler Methods
// =============================

// CreateOfferInternal creates an offer for a booking to an artisan
// This is the core offer creation logic that can be reused from different contexts
func (h *BookingsHandler) CreateOfferInternal(ctx context.Context, bookingID string, artisanID string, offeredBy string, expiresIn int) (*domain.BookingOffer, error) {
	// Set default expiration (24 hours)
	if expiresIn == 0 {
		expiresIn = 24
	}
	if expiresIn > 72 {
		return nil, fmt.Errorf("expires_in cannot exceed 72 hours")
	}

	// Check artisan availability
	availabilityStatus, err := h.repo.GetArtisanAvailability(ctx, artisanID)
	if err != nil {
		return nil, fmt.Errorf("failed to check artisan availability: %w", err)
	}

	if availabilityStatus == "unavailable" { // TODO: Use artisans/domain.ArtisanAvailabilityUnavailable constant
		h.logger.Info(ctx, "cannot offer booking: artisan is unavailable", map[string]any{
			"artisan_id": artisanID,
			"booking_id": bookingID,
		})
		return nil, fmt.Errorf("artisan is currently unavailable")
	}

	// Create offer and update booking status in transaction
	var offer *domain.BookingOffer

	err = h.repo.WithTransaction(ctx, func(txRepo domain.BookingRepository) error {
		// Get fresh booking for status update and validation
		currentBooking, err := txRepo.GetByID(ctx, bookingID)
		if err != nil {
			return fmt.Errorf("failed to get booking: %w", err)
		}

		// Validate offer can be created
		if err := h.offerValidator.ValidateOfferCreation(currentBooking, artisanID); err != nil {
			return err
		}

		// Create offer
		offer = &domain.BookingOffer{
			ID:        binternal.GenerateUUID(),
			BookingID: bookingID,
			ArtisanID: artisanID,
			Status:    domain.OfferPending,
			OfferedBy: offeredBy,
			OfferedAt: time.Now(),
			ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Hour),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := txRepo.CreateOffer(ctx, offer); err != nil {
			return fmt.Errorf("failed to create offer: %w", err)
		}

		// Update booking status to OfferPending
		currentBooking.Status = domain.BookingOfferPending
		currentBooking.UpdatedAt = time.Now()
		currentBooking.OffersCount++

		if err := txRepo.Update(ctx, currentBooking); err != nil {
			return fmt.Errorf("failed to update booking status: %w", err)
		}

		// Create event in outbox
		event := &domain.BookingEvent{
			BookingID:      bookingID,
			Status:         domain.BookingOfferPending,
			PreviousStatus: domain.BookingRequested,
			Timestamp:      time.Now(),
			UserID:         offeredBy,
			ArtisanID:      &offer.ArtisanID,
		}

		if err := txRepo.CreateEventInOutbox(ctx, event); err != nil {
			return fmt.Errorf("failed to create event: %w", err)
		}

		return nil
	})

	if err != nil {
		h.logger.Error(ctx, "failed to create offer and update booking", err, map[string]any{
			"booking_id": bookingID,
			"artisan_id": artisanID,
		})
		return nil, err
	}

	// Clear cache
	h.cache.Delete(ctx, binternal.BookingCacheKey(bookingID))

	h.logger.Info(ctx, "booking offered to artisan", map[string]any{
		"booking_id": bookingID,
		"offer_id":   offer.ID,
		"artisan_id": artisanID,
	})

	return offer, nil
}

// OfferBooking offers a booking to a specific artisan
func (h *BookingsHandler) OfferBooking(ctx context.Context, bookingID string, req *OfferBookingRequest) (*OfferResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "offer_booking")
	if err != nil {
		return nil, err
	}

	// Validate booking ID format
	if _, err := uuid.FromString(bookingID); err != nil {
		return nil, fmt.Errorf("invalid booking ID format")
	}

	// Get booking
	booking, err := h.repo.GetByID(ctx, bookingID)
	if err != nil {
		return nil, fmt.Errorf("failed to get booking: %w", err)
	}

	// Authorization: Only customer who owns the booking or admin can offer it
	userRole, err := h.getActiveRole(ctx)
	if err != nil {
		return nil, err
	}
	if booking.CustomerID != userCtx.ID && userRole != "admin" {
		return nil, binternal.ErrPermissionDenied
	}

	// Validate artisan ID
	if req.ArtisanID == "" {
		return nil, fmt.Errorf("artisan_id is required")
	}

	// Create the offer using the internal function
	offer, err := h.CreateOfferInternal(ctx, bookingID, req.ArtisanID, userCtx.ID, req.ExpiresIn)
	if err != nil {
		return nil, err
	}

	return h.toOfferResponse(offer), nil
}

// AcceptOffer allows an artisan to accept a booking offer
func (h *BookingsHandler) AcceptOffer(ctx context.Context, offerID string, req *AcceptOfferRequest) (*BookingResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "accept_offer")
	if err != nil {
		return nil, err
	}

	// Validate offer ID format
	if _, err := uuid.FromString(offerID); err != nil {
		return nil, fmt.Errorf("invalid offer ID format")
	}

	// Get offer
	offer, err := h.repo.GetOfferByID(ctx, offerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get offer: %w", err)
	}

	// Authorization: Get artisan profile for current user
	// The offer.ArtisanID is the artisan PROFILE ID, not the user ID
	// We need to check if this user owns that artisan profile
	userRole, err := h.getActiveRole(ctx)
	if err != nil {
		return nil, err
	}

	if userRole != "artisan" {
		return nil, binternal.ErrPermissionDenied
	}

	// Now check if this artisan profile matches the offer
	if offer.ArtisanID != userCtx.ID {
		return nil, binternal.ErrPermissionDenied
	}

	// Get booking
	booking, err := h.repo.GetByID(ctx, offer.BookingID)
	if err != nil {
		return nil, fmt.Errorf("failed to get booking: %w", err)
	}

	// Validate offer can be accepted
	if err := h.offerValidator.ValidateOfferAcceptance(offer, booking); err != nil {
		return nil, err
	}

	// Store previous status for event
	previousStatus := booking.Status

	// Accept offer within a transaction
	var updatedBooking *domain.Booking
	err = h.repo.WithTransaction(ctx, func(txRepo domain.BookingRepository) error {
		// Re-check booking is not assigned (race condition check)
		currentBooking, err := txRepo.GetByID(ctx, offer.BookingID)
		if err != nil {
			return err
		}
		if currentBooking.ArtisanID != nil {
			return domain.ErrOfferAlreadyTaken
		}

		// Update offer status
		if err := txRepo.UpdateOfferStatus(ctx, offerID, domain.OfferAccepted, nil); err != nil {
			return err
		}

		// Assign artisan to booking and update status
		currentBooking.ArtisanID = &offer.ArtisanID
		currentBooking.Status = domain.BookingAssigned
		currentBooking.UpdatedAt = time.Now()

		if err := txRepo.Update(ctx, currentBooking); err != nil {
			return err
		}

		// Create assigned event in outbox (atomic with transaction)
		assignedEvent := &domain.BookingEvent{
			BookingID:      offer.BookingID,
			Status:         domain.BookingAssigned,
			PreviousStatus: previousStatus,
			Timestamp:      time.Now(),
			UserID:         userCtx.ID,
			ArtisanID:      &offer.ArtisanID,
			Metadata: map[string]string{
				"customer_id": currentBooking.CustomerID,
			},
		}

		if err := h.repo.CreateEventInOutbox(ctx, assignedEvent); err != nil {
			h.logger.Error(ctx, "failed to publish booking assigned event", err, map[string]interface{}{
				"booking_id": booking.ID,
			})
		}

		// Cancel other pending offers for this booking
		if err := txRepo.CancelPendingOffers(ctx, offer.BookingID); err != nil {
			h.logger.Error(ctx, "failed to cancel pending offers", err, nil)
			// Don't fail the transaction for this
		}

		updatedBooking = currentBooking
		return nil
	})

	if err != nil {
		h.logger.Error(ctx, "failed to accept offer", err, map[string]any{
			"offer_id":   offerID,
			"booking_id": offer.BookingID,
		})
		return nil, fmt.Errorf("failed to accept offer: %w", err)
	}

	// Clear cache
	h.cache.Delete(ctx, binternal.BookingCacheKey(offer.BookingID))

	h.logger.Info(ctx, "offer accepted", map[string]any{
		"offer_id":   offerID,
		"booking_id": offer.BookingID,
		"artisan_id": offer.ArtisanID,
	})

	return h.toResponse(updatedBooking), nil
}

// RejectOffer allows an artisan to reject a booking offer
func (h *BookingsHandler) RejectOffer(ctx context.Context, offerID string, req *RejectOfferRequest) (*OfferResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "reject_offer")
	if err != nil {
		return nil, err
	}

	// Validate offer ID format
	if _, err := uuid.FromString(offerID); err != nil {
		return nil, fmt.Errorf("invalid offer ID format")
	}

	// Validate reason is provided
	if req.Reason == "" {
		return nil, fmt.Errorf("reason is required when rejecting an offer")
	}

	// Get offer
	offer, err := h.repo.GetOfferByID(ctx, offerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get offer: %w", err)
	}

	// Authorization: Only the artisan to whom the offer was made can reject it
	if offer.ArtisanID != userCtx.ID {
		return nil, binternal.ErrPermissionDenied
	}

	// Validate offer can be rejected
	if err := h.offerValidator.ValidateOfferRejection(offer); err != nil {
		return nil, err
	}

	// Update offer status and booking status atomically with event
	var updatedOffer *domain.BookingOffer
	err = h.repo.WithTransaction(ctx, func(txRepo domain.BookingRepository) error {
		// Update offer status
		if err := txRepo.UpdateOfferStatus(ctx, offerID, domain.OfferRejected, &req.Reason); err != nil {
			return err
		}

		// Get and update booking
		booking, err := txRepo.GetByID(ctx, offer.BookingID)
		if err != nil {
			return err
		}

		previousStatus := booking.Status
		booking.Status = domain.BookingOfferRejected
		booking.UpdatedAt = time.Now()

		if err := txRepo.Update(ctx, booking); err != nil {
			return err
		}

		// Create event in outbox (atomic with transaction)
		event := &domain.BookingEvent{
			BookingID:      offer.BookingID,
			Status:         domain.BookingOfferRejected,
			PreviousStatus: previousStatus,
			Timestamp:      time.Now(),
			UserID:         userCtx.ID,
			ArtisanID:      &offer.ArtisanID,
			Reason:         &req.Reason,
		}

		if err := txRepo.CreateEventInOutbox(ctx, event); err != nil {
			return err
		}

		// Get updated offer for response
		updatedOffer, err = txRepo.GetOfferByID(ctx, offerID)
		return err
	})

	if err != nil {
		h.logger.Error(ctx, "failed to reject offer", err, map[string]any{
			"offer_id": offerID,
		})
		return nil, fmt.Errorf("failed to reject offer: %w", err)
	}

	// Clear cache
	h.cache.Delete(ctx, binternal.BookingCacheKey(offer.BookingID))

	h.logger.Info(ctx, "offer rejected", map[string]any{
		"offer_id":   offerID,
		"booking_id": offer.BookingID,
		"reason":     req.Reason,
	})

	return h.toOfferResponse(updatedOffer), nil
}

// ListBookingOffers lists all offers for a specific booking
func (h *BookingsHandler) ListBookingOffers(ctx context.Context, bookingID string) (*ListOffersResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "list_booking_offers")
	if err != nil {
		return nil, err
	}

	// Validate booking ID format
	if _, err := uuid.FromString(bookingID); err != nil {
		return nil, fmt.Errorf("invalid booking ID format")
	}

	// Get booking
	booking, err := h.repo.GetByID(ctx, bookingID)
	if err != nil {
		return nil, fmt.Errorf("failed to get booking: %w", err)
	}

	// Authorization: Only customer who owns booking, assigned artisan, or admin can view offers
	userRole, err := h.getActiveRole(ctx)
	if err != nil {
		return nil, err
	}

	canView := booking.CustomerID == userCtx.ID ||
		(booking.ArtisanID != nil && *booking.ArtisanID == userCtx.ID) ||
		userRole == "admin"

	if !canView {
		return nil, binternal.ErrPermissionDenied
	}

	// Get offers
	offers, err := h.repo.GetOffersByBookingID(ctx, bookingID)
	if err != nil {
		h.logger.Error(ctx, "failed to list booking offers", err, map[string]any{
			"booking_id": bookingID,
		})
		return nil, fmt.Errorf("failed to list offers: %w", err)
	}

	// Convert to response DTOs
	offerResponses := make([]*OfferResponse, len(offers))
	for i, offer := range offers {
		offerResponses[i] = h.toOfferResponse(offer)
	}

	return &ListOffersResponse{
		Offers: offerResponses,
		Total:  len(offerResponses),
	}, nil
}

// ListArtisanOffers lists all offers for the authenticated artisan
func (h *BookingsHandler) ListArtisanOffers(ctx context.Context, params *ListOffersParams) (*ListArtisanOffersResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "list_artisan_offers")
	if err != nil {
		return nil, err
	}

	// Authorization: must currently be in artisan mode (active_role == "artisan")
	activeRole, err := h.getActiveRole(ctx)
	if err != nil {
		return nil, err
	}
	if activeRole != "artisan" {
		return nil, binternal.ErrPermissionDenied
	}

	// Build filter from request params
	filter := domain.OfferFilter{}
	if params.Status != "" {
		filter.Status = domain.BookingOfferStatus(params.Status)
	}

	// Parse ExpiresAfter parameter
	if params.ExpiresAfter != "" {
		duration, err := time.ParseDuration(params.ExpiresAfter)
		if err == nil {
			expiresAfterTime := time.Now().Add(-duration)
			filter.ExpiresAfter = &expiresAfterTime
		} else {
			expiresAfterTime, err := time.Parse(time.RFC3339, params.ExpiresAfter)
			if err != nil {
				return nil, fmt.Errorf("invalid expires_after format: must be a duration string or RFC3339 timestamp")
			}
			filter.ExpiresAfter = &expiresAfterTime
		}
	}

	offerDetails, err := h.repo.GetArtisanOffersWithDetails(ctx, userCtx.ID, filter)
	if err != nil {
		h.logger.Error(ctx, "failed to list artisan offers", err, map[string]any{
			"artisan_id": userCtx.ID,
			"status":     params.Status,
		})
		return nil, fmt.Errorf("failed to list offers: %w", err)
	}

	offerResponses := make([]*ArtisanOfferResponse, len(offerDetails))
	for i, detail := range offerDetails {
		offerResponses[i] = &ArtisanOfferResponse{
			ID:        detail.Offer.ID,
			Status:    string(detail.Offer.Status),
			ExpiresAt: detail.Offer.ExpiresAt,
			Booking: BookingPreviewDTO{
				ID:                detail.Booking.ID,
				ServiceType:       detail.Booking.ServiceCategoryID,
				CustomerFirstName: detail.CustomerFirstName,
				LocationArea:      fmt.Sprintf("%s, %s", detail.CustomerCity, detail.CustomerState),
				Description:       detail.Booking.Description,
				Photos:            detail.Booking.MediaURLs,
				Distance:          0,
				ScheduledAt:       detail.Booking.ScheduledAt,
				IsFlexible:        detail.Booking.IsFlexible,
			},
		}
	}

	return &ListArtisanOffersResponse{
		Offers: offerResponses,
		Total:  len(offerResponses),
	}, nil
}

// ExpireOffers scans for expired pending offers and transitions them to expired status.
// This is called by the cron job every 10 minutes.
// It publishes booking.v1.offer.expired events via the transactional outbox.
func (h *BookingsHandler) ExpireOffers(ctx context.Context) error {
	h.logger.Info(ctx, "starting offer expiry scan", nil)

	// Find all expired pending offers
	expiredOffers, err := h.repo.FindExpiredOffers(ctx)
	if err != nil {
		h.logger.Error(ctx, "failed to find expired offers", err, nil)
		return err
	}

	if len(expiredOffers) == 0 {
		h.logger.Info(ctx, "no expired offers found", map[string]interface{}{
			"scanned_at": time.Now(),
		})
		return nil
	}

	h.logger.Info(ctx, "found expired offers", map[string]interface{}{
		"count": len(expiredOffers),
	})

	// Process each expired offer atomically
	successCount := 0
	errorCount := 0

	for _, offer := range expiredOffers {
		err := h.expireOffer(ctx, offer)
		if err != nil {
			h.logger.Error(ctx, "failed to expire offer", err, map[string]interface{}{
				"offer_id":   offer.ID,
				"booking_id": offer.BookingID,
			})
			errorCount++
			continue
		}
		successCount++
	}

	h.logger.Info(ctx, "offer expiry scan completed", map[string]interface{}{
		"success_count": successCount,
		"error_count":   errorCount,
		"total":         len(expiredOffers),
	})

	// Return error if any offers failed to process
	if errorCount > 0 {
		return errs.B().
			Code(errs.Internal).
			Msgf("failed to expire %d out of %d offers", errorCount, len(expiredOffers)).
			Err()
	}

	return nil
}

// expireOffer transitions a single offer to expired status and publishes event
func (h *BookingsHandler) expireOffer(ctx context.Context, offer *domain.BookingOffer) error {
	return h.repo.WithTransaction(ctx, func(txRepo domain.BookingRepository) error {
		// Re-fetch offer within transaction to ensure we have latest state
		currentOffer, err := txRepo.GetOfferByID(ctx, offer.ID)
		if err != nil {
			return err
		}

		// Idempotency check: only process if still pending
		if currentOffer.Status != domain.OfferPending {
			h.logger.Info(ctx, "offer already processed, skipping", map[string]any{
				"offer_id": offer.ID,
				"status":   currentOffer.Status,
			})
			return nil
		}

		// Double-check expiry within transaction (defensive)
		if !currentOffer.IsExpired() {
			h.logger.Info(ctx, "offer no longer expired, skipping", map[string]any{
				"offer_id":   offer.ID,
				"expires_at": currentOffer.ExpiresAt,
			})
			return nil
		}

		// Get booking to update status
		booking, err := txRepo.GetByID(ctx, currentOffer.BookingID)
		if err != nil {
			return fmt.Errorf("failed to get booking: %w", err)
		}
		previousStatus := booking.Status

		// Update offer status to expired
		now := time.Now()
		currentOffer.Status = domain.OfferExpired
		currentOffer.UpdatedAt = now
		currentOffer.RespondedAt = &now // Mark when it was expired

		// Save updated offer
		if err := txRepo.UpdateOffer(ctx, currentOffer); err != nil {
			return err
		}

		// Update booking status
		booking.Status = domain.BookingOfferRejected
		booking.UpdatedAt = now

		if err := txRepo.Update(ctx, booking); err != nil {
			return fmt.Errorf("failed to update booking status: %w", err)
		}

		// Create offer expired event
		event := &domain.BookingEvent{
			BookingID:      currentOffer.BookingID,
			Status:         domain.BookingOfferRejected,
			PreviousStatus: previousStatus,
			Timestamp:      now,
			UserID:         "system",
			ArtisanID:      &currentOffer.ArtisanID,
			Reason:         stringPtr("offer_expired"),
			Metadata: map[string]string{
				"offer_id":   currentOffer.ID,
				"expired_at": now.Format(time.RFC3339),
				"expires_at": currentOffer.ExpiresAt.Format(time.RFC3339),
			},
		}

		// Write event to outbox for guaranteed delivery
		if err := txRepo.CreateOfferExpiredEventInOutbox(ctx, event); err != nil {
			return err
		}

		h.logger.Info(ctx, "offer expired successfully", map[string]interface{}{
			"offer_id":   currentOffer.ID,
			"booking_id": currentOffer.BookingID,
			"artisan_id": currentOffer.ArtisanID,
		})

		return nil
	})
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
