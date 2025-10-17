package handlers

import (
	"context"
	"fmt"
	"time"

	"encore.app/booking/domain"
	binternal "encore.app/booking/internal"
	"encore.dev/types/uuid"
)

// =============================
// Offer Handler Methods
// =============================

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
	userRole, err := h.getUserRole(ctx, userCtx.ID)
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

	// Validate offer can be created
	if err := h.offerValidator.ValidateOfferCreation(booking, req.ArtisanID); err != nil {
		return nil, err
	}

	// Set default expiration (24 hours)
	expiresIn := req.ExpiresIn
	if expiresIn == 0 {
		expiresIn = 24
	}
	if expiresIn > 72 {
		return nil, fmt.Errorf("expires_in cannot exceed 72 hours")
	}

	// Create offer
	offer := &domain.BookingOffer{
		BookingID: bookingID,
		ArtisanID: req.ArtisanID,
		Status:    domain.OfferPending,
		OfferedBy: userCtx.ID,
		OfferedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Duration(expiresIn) * time.Hour), // Default 24h expiration
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := h.repo.CreateOffer(ctx, offer); err != nil {
		h.logger.Error(ctx, "failed to create offer", err, map[string]any{
			"booking_id": bookingID,
			"artisan_id": req.ArtisanID,
		})
		return nil, fmt.Errorf("failed to create offer: %w", err)
	}

	// Update booking status to OfferPending
	booking.Status = domain.BookingOfferPending
	if err := h.repo.Update(ctx, booking); err != nil {
		h.logger.Error(ctx, "failed to update booking status to OfferPending", err, map[string]any{
			"booking_id": bookingID,
			"status":     domain.BookingOfferPending,
		})
		// Log error but don't fail the offer creation
	}

	// Publish event
	go func() {
		backgroundCtx := context.Background()
		h.publisher.PublishOfferCreatedEvent(backgroundCtx, &domain.BookingEvent{
			BookingID: bookingID,
			Status:    domain.BookingOfferPending,
			Timestamp: time.Now(),
			UserID:    userCtx.ID,
			ArtisanID: &offer.ArtisanID,
		})
	}()

	h.logger.Info(ctx, "booking offered to artisan", map[string]any{
		"booking_id": bookingID,
		"offer_id":   offer.ID,
		"artisan_id": req.ArtisanID,
	})

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

	// Authorization: Only the artisan to whom the offer was made can accept it
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

	// Accept offer within a transaction
	var updatedBooking *domain.Booking
	err = h.repo.WithTransaction(ctx, func(txRepo domain.BookingRepository) error {
		// Re-check booking is not assigned (race condition check)
		booking, err := txRepo.GetByID(ctx, offer.BookingID)
		if err != nil {
			return err
		}
		if booking.ArtisanID != nil {
			return domain.ErrOfferAlreadyTaken
		}

		// Update offer status
		if err := txRepo.UpdateOfferStatus(ctx, offerID, domain.OfferAccepted, nil); err != nil {
			return err
		}

		// Assign artisan to booking and update booking status to Assigned
		booking.ArtisanID = &offer.ArtisanID
		booking.Status = domain.BookingAssigned
		booking.UpdatedAt = time.Now()

		if err := txRepo.Update(ctx, booking); err != nil {
			return err
		}

		// Cancel other pending offers for this booking
		if err := txRepo.CancelPendingOffers(ctx, offer.BookingID); err != nil {
			h.logger.Error(ctx, "failed to cancel pending offers", err, nil)
			// Don't fail the transaction for this
		}

		updatedBooking = booking
		return nil
	})

	if err != nil {
		h.logger.Error(ctx, "failed to accept offer", err, map[string]any{
			"offer_id":   offerID,
			"booking_id": offer.BookingID,
		})
		return nil, fmt.Errorf("failed to accept offer: %w", err)
	}

	// Publish events
	go func() {
		backgroundCtx := context.Background()

		// Publish assigned event
		assignedEvent := &domain.BookingEvent{
			BookingID: offer.BookingID,
			Status:    domain.BookingAssigned,
			Timestamp: time.Now(),
			UserID:    userCtx.ID,
			ArtisanID: &offer.ArtisanID,
		}
		h.publisher.PublishAssignedEvent(backgroundCtx, assignedEvent)

		// Publish status event
		statusEvent := &domain.BookingEvent{
			BookingID:      offer.BookingID,
			Status:         domain.BookingAssigned,
			PreviousStatus: domain.BookingOfferPending,
			Timestamp:      time.Now(),
			UserID:         userCtx.ID,
			ArtisanID:      &offer.ArtisanID,
		}
		h.publisher.PublishStatusEvent(backgroundCtx, statusEvent)
	}()

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

	// Update offer status and booking status to OfferRejected
	err = h.repo.WithTransaction(ctx, func(txRepo domain.BookingRepository) error {
		if err := txRepo.UpdateOfferStatus(ctx, offerID, domain.OfferRejected, &req.Reason); err != nil {
			return err
		}
		booking, err := txRepo.GetByID(ctx, offer.BookingID)
		if err != nil {
			return err
		}
		booking.Status = domain.BookingOfferRejected
		booking.UpdatedAt = time.Now()
		if err := txRepo.Update(ctx, booking); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		h.logger.Error(ctx, "failed to reject offer", err, map[string]any{
			"offer_id": offerID,
		})
		return nil, fmt.Errorf("failed to reject offer: %w", err)
	}

	// Re-fetch updated offer
	offer, err = h.repo.GetOfferByID(ctx, offerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get updated offer: %w", err)
	}

	// Publish event
	go func() {
		backgroundCtx := context.Background()
		event := &domain.BookingEvent{
			BookingID:      offer.BookingID,
			Status:         domain.BookingOfferRejected,
			PreviousStatus: domain.BookingOfferPending,
			Timestamp:      time.Now(),
			UserID:         userCtx.ID,
			ArtisanID:      &offer.ArtisanID,
			Reason:         &req.Reason,
		}
		h.publisher.PublishStatusEvent(backgroundCtx, event)
	}()

	h.logger.Info(ctx, "offer rejected", map[string]any{
		"offer_id":   offerID,
		"booking_id": offer.BookingID,
		"reason":     req.Reason,
	})

	return h.toOfferResponse(offer), nil
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
	userRole, err := h.getUserRole(ctx, userCtx.ID)
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
func (h *BookingsHandler) ListArtisanOffers(ctx context.Context, params *ListOffersParams) (*ListOffersResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "list_artisan_offers")
	if err != nil {
		return nil, err
	}

	// Authorization: Only artisans can view their offers
	userRole, err := h.getUserRole(ctx, userCtx.ID)
	if err != nil {
		return nil, err
	}
	if userRole != "artisan" {
		return nil, binternal.ErrPermissionDenied
	}

	// Parse status filter
	var status domain.BookingOfferStatus
	if params.Status != "" {
		status = domain.BookingOfferStatus(params.Status)
	}

	// Get offers
	offers, err := h.repo.GetOffersByArtisanID(ctx, userCtx.ID, status)
	if err != nil {
		h.logger.Error(ctx, "failed to list artisan offers", err, map[string]any{
			"artisan_id": userCtx.ID,
			"status":     params.Status,
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
