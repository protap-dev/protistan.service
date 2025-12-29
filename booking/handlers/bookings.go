package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"encore.app/booking/domain"
	binternal "encore.app/booking/internal"
	"encore.app/core"
	"encore.app/core/cache"
	"encore.app/user"
	"encore.dev/beta/errs"
	"encore.dev/types/uuid"
)

// BookingsHandler handles booking-related API logic
type BookingsHandler struct {
	repo           domain.BookingRepository
	validator      domain.BookingValidator
	offerValidator domain.OfferValidator
	logger         binternal.ServiceLogger
	cache          cache.CacheManager
	coreSvc        *core.CoreService
	authHelper     *binternal.AuthHelper
	publisher      domain.EventPublisher
}

func NewBookingsHandler(
	repo domain.BookingRepository,
	validator domain.BookingValidator,
	offerValidator domain.OfferValidator,
	logger binternal.ServiceLogger,
	cache cache.CacheManager,
	coreSvc *core.CoreService,
	authHelper *binternal.AuthHelper,
	publisher domain.EventPublisher,
) *BookingsHandler {
	return &BookingsHandler{
		repo:           repo,
		validator:      validator,
		offerValidator: offerValidator,
		logger:         logger,
		cache:          cache,
		coreSvc:        coreSvc,
		authHelper:     authHelper,
		publisher:      publisher,
	}
}

// GetRepository returns the repository for external access
func (h *BookingsHandler) GetRepository() domain.BookingRepository {
	return h.repo
}

// Helper function to copy metadata safely
func copyMetadata(src map[string]string) map[string]string {
	if src == nil {
		return make(map[string]string)
	}

	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// =============================
// API Methods (Handler Level)
// =============================

func (h *BookingsHandler) CreateBooking(ctx context.Context, req *CreateBookingRequest) (*BookingResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "create_booking")
	if err != nil {
		return nil, err
	}

	// Handle idempotency key if provided
	if req.IdempotencyKey != nil && *req.IdempotencyKey != "" {
		// Generate request hash from request body for validation
		requestHash := generateRequestHash(req)

		// Check if this idempotency key was already processed
		existingRecord, err := h.repo.CheckIdempotencyKey(ctx, *req.IdempotencyKey, userCtx.ID, requestHash)
		if err != nil {
			h.logger.Error(ctx, "idempotency key check failed", err, map[string]interface{}{
				"user_id": userCtx.ID,
			})
			return nil, binternal.ErrDatabaseError
		}

		if existingRecord != nil {
			// Request already processed
			if existingRecord.Status == "completed" {
				// Return cached response
				var cachedResponse BookingResponse
				if err := json.Unmarshal(existingRecord.ResponseBody, &cachedResponse); err == nil {
					return &cachedResponse, nil
				}
			}

			if existingRecord.Status == "processing" {
				// Another request is currently processing - return conflict
				return nil, errs.B().Code(errs.Aborted).
					Msg("request with this idempotency key is already being processed").Err()
			}
		}

		// Store idempotency key atomically (24 hour expiration)
		expiresAt := time.Now().Add(24 * time.Hour)
		err = h.repo.StoreIdempotencyKey(ctx, *req.IdempotencyKey, userCtx.ID, requestHash, expiresAt)
		if err != nil {
			// If duplicate key error, another concurrent request won - return conflict
			if strings.Contains(err.Error(), "duplicate") {
				return nil, errs.B().Code(errs.Aborted).
					Msg("request with this idempotency key is already being processed").Err()
			}
			return nil, binternal.ErrDatabaseError
		}
	}

	var artisanID *string
	isSpecificArtisan := false
	var specificArtisanID string // Store separately for auto-offer creation
	if req.SpecificArtisanID != nil && *req.SpecificArtisanID != "" {
		isSpecificArtisan = true
		specificArtisanID = *req.SpecificArtisanID
		// Don't set artisanID here - create booking without artisan first
	}

	// Construct the domain request for validation
	domainReq := &domain.CreateBookingRequest{
		ServiceCategoryID: req.ServiceCategoryID,
		ServiceID:         req.ServiceID,
		CustomerAddressID: req.CustomerAddressID,
		Description:       req.Description,
		MediaURLs:         req.MediaURLs,
		ScheduledAt:       req.ScheduledAt,
		IsFlexible:        req.IsFlexible,
	}

	// Validate input first
	if err := h.validator.ValidateCreateRequest(domainReq); err != nil {
		h.logger.Error(ctx, "validation failed", err, map[string]interface{}{
			"user_id": userCtx.ID,
		})
		return nil, binternal.ErrInvalidInput
	}

	// Generate a title automatically
	var autoTitle string
	serviceName := req.Metadata["service_name"]
	if serviceName != "" {
		autoTitle = fmt.Sprintf("%s.", serviceName)
	} else {
		autoTitle = fmt.Sprintf("Service Request: %s", time.Now().Format("Jan 02"))
	}

	// Default priority based on ScheduledAt
	defaultPriority := "normal"
	if req.ScheduledAt != nil && time.Until(*req.ScheduledAt) < 24*time.Hour {
		defaultPriority = "high"
	}

	booking := &domain.Booking{
		CustomerID:        userCtx.ID,
		ArtisanID:         artisanID, // Will be nil for auto-offer case
		ServiceCategoryID: req.ServiceCategoryID,
		ServiceID:         req.ServiceID,
		Title:             autoTitle,
		Description:       req.Description,
		CustomerAddressID: req.CustomerAddressID,
		Status:            domain.BookingRequested,
		MediaURLs:         req.MediaURLs,
		IsFlexible:        req.IsFlexible,
		ScheduledAt:       req.ScheduledAt,
		Priority:          defaultPriority,
		IsSpecificArtisan: isSpecificArtisan,
		Metadata:          copyMetadata(req.Metadata), // Copy request metadata
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
		Version:           1,
	}

	// Validate references
	if err := binternal.ValidateBookingReferences(ctx, domainReq, userCtx.ID, h.logger); err != nil {
		h.logger.Error(ctx, "service reference validation failed", err, map[string]interface{}{
			"user_id": userCtx.ID,
		})
		return nil, binternal.ErrInvalidInput
	}

	if err := h.repo.Create(ctx, booking); err != nil {
		h.logger.Error(ctx, "failed to create booking", err, map[string]interface{}{
			"user_id": userCtx.ID,
		})
		return nil, binternal.ErrDatabaseError
	}

	h.cache.Set(ctx, binternal.BookingCacheKey(booking.ID), booking, binternal.DefaultServiceConfig().Cache.BookingTTL)

	// Create and publish created event
	event := &domain.BookingEvent{
		BookingID:      booking.ID,
		Status:         booking.Status,
		PreviousStatus: "",
		Timestamp:      time.Now(),
		UserID:         userCtx.ID,
	}

	if err := h.repo.CreateEventInOutbox(ctx, event); err != nil {
		h.logger.Error(ctx, "failed to publish booking created event", err, map[string]interface{}{
			"booking_id": booking.ID,
		})
	}

	// Auto-create offer if specific artisan is requested
	var offer *domain.BookingOffer
	if isSpecificArtisan && specificArtisanID != "" {
		offer, err = h.CreateOfferInternal(ctx, booking.ID, specificArtisanID, userCtx.ID, 24) // Default 24 hours
		if err != nil {
			h.logger.Error(ctx, "failed to create automatic offer for specific artisan", err, map[string]any{
				"booking_id": booking.ID,
				"artisan_id": specificArtisanID,
			})
			// Don't fail the entire booking creation, just log the error
			// The booking exists and can be offered manually later
		}
	}

	response := h.toResponse(booking)

	// Add offer metadata to response if offer was created
	if offer != nil {
		if response.Metadata == nil {
			response.Metadata = make(map[string]string)
		}
		response.Metadata["offer_id"] = offer.ID
		response.Metadata["offer_status"] = string(offer.Status)
		response.Metadata["offer_expires_at"] = offer.ExpiresAt.Format(time.RFC3339)
		response.Metadata["auto_offered"] = "true"
		response.Metadata["offered_to"] = offer.ArtisanID
	}

	// Complete idempotency key if provided
	if req.IdempotencyKey != nil && *req.IdempotencyKey != "" {
		_ = h.repo.CompleteIdempotencyKey(ctx, *req.IdempotencyKey, booking.ID, response)
	}

	return response, nil
}

func (h *BookingsHandler) UpdateBookingStatus(ctx context.Context, id string, req *UpdateStatusRequest) (*BookingResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "update_booking_status")
	if err != nil {
		return nil, err
	}

	userRole, err := h.getActiveRole(ctx)
	if err != nil {
		return nil, err
	}

	// Get current booking for authorization and transition validation
	current, err := h.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrBookingNotFound) {
			return nil, binternal.ErrNotFound
		}
		return nil, binternal.ErrDatabaseError
	}

	if err := binternal.AuthorizeStatusUpdate(ctx, userRole, userCtx.ID, current); err != nil {
		return nil, err
	}

	newStatus := domain.BookingStatus(req.Status)
	if !domain.CanTransition(current.Status, newStatus) {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid status transition").
			Meta("from", string(current.Status)).
			Meta("to", req.Status).Err()
	}

	// Use the internal helper for the actual status update
	err = h.UpdateBookingStatusInternal(ctx, id, newStatus, userCtx.ID, req.Reason, current, nil, nil)
	if err != nil {
		return nil, err
	}

	// Re-fetch the booking to get the latest data for response
	updatedBooking, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, binternal.ErrDatabaseError
	}

	return h.toResponse(updatedBooking), nil
}

func (h *BookingsHandler) GetBooking(ctx context.Context, id string) (*BookingResponse, error) {
	// First, validate the booking ID format before proceeding.
	if _, err := uuid.FromString(id); err != nil {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid booking ID format").Err()
	}

	userCtx, err := h.authHelper.ExtractUserContext(ctx, "get_booking")
	if err != nil {
		return nil, err
	}

	if val, ok := h.cache.Get(ctx, binternal.BookingCacheKey(id)); ok {
		booking := val.(*domain.Booking)

		if err := binternal.VerifyUserAccess(ctx, userCtx.ID, booking); err != nil {
			return nil, err
		}

		return h.toResponse(booking), nil
	}

	booking, err := h.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrBookingNotFound) {
			return nil, binternal.ErrNotFound
		}
		return nil, binternal.ErrDatabaseError
	}

	if err := binternal.VerifyUserAccess(ctx, userCtx.ID, booking); err != nil {
		return nil, err
	}

	h.cache.Set(ctx, binternal.BookingCacheKey(id), booking, binternal.DefaultServiceConfig().Cache.BookingTTL)

	return h.toResponse(booking), nil
}

func (h *BookingsHandler) ListBookings(ctx context.Context, params *ListBookingsParams) (*ListBookingsResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "list_bookings")
	if err != nil {
		return nil, err
	}

	userRole, err := h.getActiveRole(ctx)
	if err != nil {
		return nil, err
	}

	var bookings []*domain.Booking

	switch userRole {
	case "customer":
		bookings, err = h.repo.GetByCustomerID(ctx, userCtx.ID)
	case "artisan":
		bookings, err = h.repo.GetByArtisanID(ctx, userCtx.ID)
	default:
		return nil, binternal.ErrPermissionDenied
	}

	if err != nil {
		return nil, binternal.ErrDatabaseError
	}

	if params.Status != "" {
		// Split comma-separated statuses
		statusStrings := strings.Split(params.Status, ",")
		validStatuses := []domain.BookingStatus{
			domain.BookingRequested,
			domain.BookingOfferPending,
			domain.BookingOfferRejected,
			domain.BookingAssigned,
			domain.BookingPendingQuote,
			domain.BookingQuoteProposed,
			domain.BookingQuoteAccepted,
			domain.BookingQuoteRejected,
			domain.BookingPaymentPending,
			domain.BookingConfirmed,
			domain.BookingEnroute,
			domain.BookingInProgress,
			domain.BookingCompleted,
			domain.BookingCancelled,
			domain.BookingClosed,
		}

		// Validate each status
		requestedStatuses := make([]domain.BookingStatus, 0, len(statusStrings))
		for _, s := range statusStrings {
			status := domain.BookingStatus(strings.TrimSpace(s))
			if !slices.Contains(validStatuses, status) {
				return nil, binternal.ErrValidationFailed
			}
			requestedStatuses = append(requestedStatuses, status)
		}

		// Filter bookings by any of the requested statuses
		filtered := make([]*domain.Booking, 0)
		for _, b := range bookings {
			if slices.Contains(requestedStatuses, b.Status) {
				filtered = append(filtered, b)
			}
		}
		bookings = filtered
	}

	// Get the total count before applying pagination
	totalCount := len(bookings)

	// Use centralized configuration for pagination
	config := binternal.DefaultServiceConfig()
	limit := config.Pagination.DefaultLimit
	if params.Limit > 0 && params.Limit <= config.Pagination.MaxLimit {
		limit = params.Limit
	}

	offset := binternal.MaxInt(params.Offset, 0)

	if offset >= len(bookings) {
		bookings = []*domain.Booking{}
	} else {
		end := binternal.MinInt(offset+limit, len(bookings))
		bookings = bookings[offset:end]
	}

	responses := make([]*BookingResponse, 0, len(bookings))
	for _, booking := range bookings {
		responses = append(responses, h.toResponse(booking))
	}

	return &ListBookingsResponse{
		Bookings: responses,
		Total:    totalCount, // Use the correct total count here
		Offset:   params.Offset,
		Limit:    limit,
	}, nil
}

func (h *BookingsHandler) CancelBooking(ctx context.Context, id string, req *CancelBookingRequest) (*BookingResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "cancel_booking")
	if err != nil {
		return nil, err
	}

	// Get current booking for authorization
	current, err := h.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrBookingNotFound) {
			return nil, binternal.ErrNotFound
		}
		return nil, binternal.ErrDatabaseError
	}

	if err := binternal.AuthorizeCancel(ctx, userCtx.ID, current); err != nil {
		return nil, err
	}

	// Use the internal helper for the actual status update
	err = h.UpdateBookingStatusInternal(ctx, id, domain.BookingCancelled, userCtx.ID, req.Reason, current, nil, nil)
	if err != nil {
		return nil, err
	}

	// Re-fetch the booking to get the latest data for response
	updatedBooking, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, binternal.ErrDatabaseError
	}

	return h.toResponse(updatedBooking), nil
}

// RematchBooking handles customer requests for rematching their booking with a different artisan
func (h *BookingsHandler) RematchBooking(ctx context.Context, id string, req *RematchBookingRequest) (*BookingResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "rematch_booking")
	if err != nil {
		return nil, err
	}

	// Get current booking for validation and authorization
	current, err := h.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrBookingNotFound) {
			return nil, binternal.ErrNotFound
		}
		return nil, binternal.ErrDatabaseError
	}

	// Authorize that the user is the customer who owns this booking
	if current.CustomerID != userCtx.ID {
		return nil, binternal.ErrPermissionDenied
	}

	// Validate booking is in a rematch-eligible state
	if err := binternal.ValidateRematchEligibility(ctx, current.Status); err != nil {
		return nil, err
	}

	// Check idempotency using If-Match header (booking version)
	if err := binternal.ValidateIdempotency(ctx, id, current.Version, h.cache); err != nil {
		return nil, err
	}

	// Create rematch event
	rematchEvent := &domain.RematchEvent{
		BookingID:         id,
		PreviousArtisanID: current.ArtisanID,
		CurrentStatus:     current.Status,
		Reason:            req.Reason,
		Timestamp:         time.Now(),
		UserID:            userCtx.ID,
	}

	// Publish rematch requested event (wrapped in envelope via outbox)
	if err := h.repo.CreateRematchEventInOutbox(ctx, rematchEvent); err != nil {
		h.logger.Error(ctx, "failed to publish rematch requested event", err, map[string]interface{}{
			"booking_id": id,
			"user_id":    userCtx.ID,
		})
		// Don't fail the request if event publishing fails - the rematch was requested successfully
	}

	// Return current booking state (no state change as per Option A)
	return h.toResponse(current), nil
}

// UpdateBookingStatusInternal is a helper method that centralizes common status update logic
func (h *BookingsHandler) UpdateBookingStatusInternal(ctx context.Context, bookingID string, newStatus domain.BookingStatus, userID string, reason *string, current *domain.Booking, metadata map[string]string, rawData []byte) error {
	var previousStatus domain.BookingStatus

	err := h.repo.WithTransaction(ctx, func(txRepo domain.BookingRepository) error {
		previousStatus = current.Status
		current.Status = newStatus
		current.UpdatedAt = time.Now()

		if err := txRepo.Update(ctx, current); err != nil {
			var optimisticLockErr *domain.ErrOptimisticLockFailure
			if errors.As(err, &optimisticLockErr) {
				return errs.B().Code(errs.FailedPrecondition).Msg("booking has been modified by another process").Err()
			}
			return binternal.ErrDatabaseError
		}

		// Write event to outbox - ONE event per status change
		statusEvent := &domain.BookingEvent{
			BookingID:      bookingID,
			Status:         newStatus,
			PreviousStatus: previousStatus,
			Timestamp:      time.Now(),
			UserID:         userID,
			ArtisanID:      current.ArtisanID,
			Reason:         reason,
			Metadata:       make(map[string]string),
			RawData:        rawData,
		}

		// Merge existing booking metadata and provided metadata
		if current.Metadata != nil {
			for k, v := range current.Metadata {
				statusEvent.Metadata[k] = v
			}
		}
		for k, v := range metadata {
			statusEvent.Metadata[k] = v
		}

		if err := txRepo.CreateEventInOutbox(ctx, statusEvent); err != nil {
			h.logger.Error(ctx, "failed to create status change event in outbox", err, map[string]interface{}{
				"booking_id": bookingID,
				"status":     newStatus,
			})
			return err
		}

		h.logger.Info(ctx, "booking status change event written to outbox", map[string]interface{}{
			"booking_id": bookingID,
			"status":     newStatus,
			"has_raw":    len(rawData) > 0,
		})

		return nil
	})

	if err != nil {
		return err
	}

	// Re-fetch the booking to get the latest data and update cache
	updatedBooking, err := h.repo.GetByID(ctx, bookingID)
	if err != nil {
		return binternal.ErrDatabaseError
	}

	// Update cache
	h.cache.Set(ctx, binternal.BookingCacheKey(bookingID), updatedBooking, binternal.DefaultServiceConfig().Cache.BookingTTL)

	return nil
}

func (h *BookingsHandler) getActiveRole(ctx context.Context) (string, error) {
	// Use user service API (same as client) for consistent data access
	profileResp, err := user.GetProfile(ctx)
	if err != nil {
		userID := binternal.ExtractUserIDFromContext()
		h.logger.Error(ctx, "failed to get user profile", err, map[string]any{
			"user_id": userID,
		})
		return "", fmt.Errorf("failed to get user profile: %w", err)
	}

	return binternal.GetActiveRole(profileResp.User.ActiveRole), nil
}

func (h *BookingsHandler) toResponse(booking *domain.Booking) *BookingResponse {
	var artisanID *string
	if booking.ArtisanID != nil {
		artisanID = booking.ArtisanID
	}
	var scheduledAt *time.Time
	if booking.ScheduledAt != nil {
		scheduledAt = booking.ScheduledAt
	}
	return &BookingResponse{
		ID:                booking.ID,
		CustomerID:        booking.CustomerID,
		ArtisanID:         artisanID,
		ServiceCategoryID: booking.ServiceCategoryID,
		Title:             booking.Title,
		Description:       booking.Description,
		CustomerAddressID: booking.CustomerAddressID,
		Status:            booking.Status,
		Priority:          booking.Priority,
		IsFlexible:        booking.IsFlexible,
		ScheduledAt:       scheduledAt,
		Metadata:          booking.Metadata,
		CreatedAt:         booking.CreatedAt,
		UpdatedAt:         booking.UpdatedAt,
	}
}

func (h *BookingsHandler) toOfferResponse(offer *domain.BookingOffer) *OfferResponse {
	return &OfferResponse{
		ID:           offer.ID,
		BookingID:    offer.BookingID,
		ArtisanID:    offer.ArtisanID,
		Status:       string(offer.Status),
		OfferedBy:    offer.OfferedBy,
		OfferedAt:    offer.OfferedAt,
		ExpiresAt:    offer.ExpiresAt,
		RespondedAt:  offer.RespondedAt,
		RejectReason: offer.RejectReason,
		CreatedAt:    offer.CreatedAt,
		UpdatedAt:    offer.UpdatedAt,
	}
}

func generateRequestHash(req *CreateBookingRequest) string {
	// Create deterministic hash of request fields (excluding idempotency key)
	data := fmt.Sprintf("%s|%s|%s|%s|%v|%v",
		req.ServiceCategoryID,
		req.ServiceID,
		req.CustomerAddressID,
		req.Description,
		req.ScheduledAt,
		req.IsFlexible,
	)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}
