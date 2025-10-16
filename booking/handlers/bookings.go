package handlers

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	repo       domain.BookingRepository
	validator  domain.BookingValidator
	logger     binternal.ServiceLogger
	cache      cache.CacheManager
	coreSvc    *core.CoreService
	authHelper *binternal.AuthHelper
	publisher  domain.EventPublisher
}

func NewBookingsHandler(
	repo domain.BookingRepository,
	validator domain.BookingValidator,
	logger binternal.ServiceLogger,
	cache cache.CacheManager,
	coreSvc *core.CoreService,
	authHelper *binternal.AuthHelper,
	publisher domain.EventPublisher,
) *BookingsHandler {
	return &BookingsHandler{
		repo:       repo,
		validator:  validator,
		logger:     logger,
		cache:      cache,
		coreSvc:    coreSvc,
		authHelper: authHelper,
		publisher:  publisher,
	}
}

// =============================
// API Methods (Handler Level)
// =============================

func (h *BookingsHandler) CreateBooking(ctx context.Context, req *CreateBookingRequest) (*BookingResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "create_booking")
	if err != nil {
		return nil, err
	}

	booking := &domain.Booking{
		CustomerID:            userCtx.ID,
		ServiceCategoryID:     req.ServiceCategoryID,
		Title:                 req.Title,
		Description:           req.Description,
		CustomerAddressID:     req.CustomerAddressID,
		Status:                domain.BookingPendingPayment,
		Priority:              req.Priority,
		ScheduledAt:           req.ScheduledAt,
		EstimatedDurationMins: req.EstimatedDurationMins,
		Metadata:              req.Metadata,
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
		Version:               1,
	}

	if err := binternal.ValidateCustomerRole(ctx, userCtx.ID, h.logger); err != nil {
		return nil, err
	}

	if err := h.validator.ValidateCreateRequest(&domain.CreateBookingRequest{
		ServiceCategoryID:     req.ServiceCategoryID,
		Title:                 req.Title,
		Description:           req.Description,
		CustomerAddressID:     req.CustomerAddressID,
		Priority:              req.Priority,
		ScheduledAt:           req.ScheduledAt,
		EstimatedDurationMins: req.EstimatedDurationMins,
		Metadata:              req.Metadata,
	}); err != nil {
		h.logger.Error(ctx, "validation failed", err, map[string]interface{}{
			"user_id": userCtx.ID,
		})
		return nil, binternal.ErrInvalidInput
	}

	domainReq := &domain.CreateBookingRequest{
		ServiceCategoryID:     req.ServiceCategoryID,
		Title:                 req.Title,
		Description:           req.Description,
		CustomerAddressID:     req.CustomerAddressID,
		Priority:              req.Priority,
		ScheduledAt:           req.ScheduledAt,
		EstimatedDurationMins: req.EstimatedDurationMins,
		Metadata:              req.Metadata,
	}
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

	h.cache.Set(ctx, binternal.BookingCacheKey(booking.ID), booking, 30*time.Minute)

	go func() {
		h.publisher.PublishCreatedEvent(context.Background(), &domain.BookingEvent{
			BookingID:      booking.ID,
			Status:         booking.Status,
			PreviousStatus: "",
			Timestamp:      time.Now(),
			UserID:         userCtx.ID,
		})
	}()

	return h.toResponse(booking), nil
}

func (h *BookingsHandler) UpdateBookingStatus(ctx context.Context, id string, req *UpdateStatusRequest) (*BookingResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "update_booking_status")
	if err != nil {
		return nil, err
	}

	userRole, err := h.getUserRole(ctx, userCtx.ID)
	if err != nil {
		return nil, err
	}

	var previousStatus domain.BookingStatus

	err = h.repo.WithTransaction(ctx, func(txRepo domain.BookingRepository) error {
		current, err := txRepo.GetByID(ctx, id)
		if err != nil {
			if errors.Is(err, domain.ErrBookingNotFound) {
				return binternal.ErrNotFound
			}
			return binternal.ErrDatabaseError
		}

		if err := binternal.AuthorizeStatusUpdate(ctx, userRole, userCtx.ID, current); err != nil {
			return err
		}

		newStatus := domain.BookingStatus(req.Status)
		if !domain.CanTransition(current.Status, newStatus) {
			return errs.B().Code(errs.InvalidArgument).Msg("invalid status transition").
				Meta("from", string(current.Status)).
				Meta("to", req.Status).Err()
		}

		previousStatus = current.Status
		current.Status = newStatus

		if err := txRepo.Update(ctx, current); err != nil {
			var optimisticLockErr *domain.ErrOptimisticLockFailure
			if errors.As(err, &optimisticLockErr) {
				return errs.B().Code(errs.FailedPrecondition).Msg("booking has been modified by another process").Err()
			}
			return binternal.ErrDatabaseError
		}

		historyEvent := &domain.BookingEvent{
			BookingID:      id,
			Status:         newStatus,
			PreviousStatus: previousStatus,
			Timestamp:      time.Now(),
			UserID:         userCtx.ID,
			ArtisanID:      current.ArtisanID,
			Reason:         req.Reason,
		}
		return txRepo.CreateStatusHistory(ctx, historyEvent)
	})

	if err != nil {
		return nil, err
	}

	// Re-fetch the booking to get the latest data (including db-generated fields)
	updatedBooking, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, binternal.ErrDatabaseError
	}

	h.cache.Delete(ctx, binternal.BookingCacheKey(id))
	h.cache.Set(ctx, binternal.BookingCacheKey(id), updatedBooking, 30*time.Minute)

	// Publish events in a background context to prevent cancellation
	go func() {
		backgroundCtx := context.Background()
		event := &domain.BookingEvent{
			BookingID:      id,
			Status:         updatedBooking.Status,
			PreviousStatus: previousStatus,
			Timestamp:      time.Now(),
			UserID:         userCtx.ID,
			ArtisanID:      updatedBooking.ArtisanID,
			Reason:         req.Reason,
		}

		h.publisher.PublishStatusEvent(backgroundCtx, event)

		if updatedBooking.Status == domain.BookingCancelled {
			h.publisher.PublishCancelledEvent(backgroundCtx, event)
		}
	}()

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

	h.cache.Set(ctx, binternal.BookingCacheKey(id), booking, 30*time.Minute)

	return h.toResponse(booking), nil
}

func (h *BookingsHandler) ListBookings(ctx context.Context, params *ListBookingsParams) (*ListBookingsResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "list_bookings")
	if err != nil {
		return nil, err
	}

	userRole, err := h.getUserRole(ctx, userCtx.ID)
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
		status := domain.BookingStatus(params.Status)
		validStatus := slices.Contains([]domain.BookingStatus{
			domain.BookingPendingPayment,
			domain.BookingRequested,
			domain.BookingAccepted,
			domain.BookingEnroute,
			domain.BookingInProgress,
			domain.BookingCompleted,
			domain.BookingCancelled,
			domain.BookingClosed,
		}, status)
		if !validStatus {
			return nil, binternal.ErrValidationFailed
		}

		filtered := make([]*domain.Booking, 0)
		for _, b := range bookings {
			if b.Status == status {
				filtered = append(filtered, b)
			}
		}
		bookings = filtered
	}

	// Get the total count before applying pagination
	totalCount := len(bookings)

	limit := 20
	if params.Limit > 0 && params.Limit < 100 {
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

	var previousStatus domain.BookingStatus

	err = h.repo.WithTransaction(ctx, func(txRepo domain.BookingRepository) error {
		current, err := txRepo.GetByID(ctx, id)
		if err != nil {
			if errors.Is(err, domain.ErrBookingNotFound) {
				return binternal.ErrNotFound
			}
			return binternal.ErrDatabaseError
		}

		if err := binternal.AuthorizeCancel(ctx, userCtx.ID, current); err != nil {
			return err
		}

		previousStatus = current.Status
		current.Status = domain.BookingCancelled

		if err := txRepo.Update(ctx, current); err != nil {
			var optimisticLockErr *domain.ErrOptimisticLockFailure
			if errors.As(err, &optimisticLockErr) {
				return errs.B().Code(errs.FailedPrecondition).Msg("booking has been modified by another process").Err()
			}
			return binternal.ErrDatabaseError
		}

		historyEvent := &domain.BookingEvent{
			BookingID:      id,
			Status:         domain.BookingCancelled,
			PreviousStatus: previousStatus,
			Timestamp:      time.Now(),
			UserID:         userCtx.ID,
			ArtisanID:      current.ArtisanID,
			Reason:         req.Reason,
		}
		return txRepo.CreateStatusHistory(ctx, historyEvent)
	})

	if err != nil {
		return nil, err
	}

	// Re-fetch the booking to get the latest data (including db-generated fields)
	updatedBooking, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, binternal.ErrDatabaseError
	}

	h.cache.Delete(ctx, binternal.BookingCacheKey(id))
	h.cache.Set(ctx, binternal.BookingCacheKey(id), updatedBooking, 30*time.Minute)

	// Publish events in a background context to prevent cancellation
	go func() {
		backgroundCtx := context.Background()
		event := &domain.BookingEvent{
			BookingID:      id,
			Status:         domain.BookingCancelled,
			PreviousStatus: previousStatus,
			Timestamp:      time.Now(),
			UserID:         userCtx.ID,
			ArtisanID:      updatedBooking.ArtisanID,
			Reason:         req.Reason,
		}

		h.publisher.PublishStatusEvent(backgroundCtx, event)

		h.publisher.PublishCancelledEvent(backgroundCtx, event)
	}()

	return h.toResponse(updatedBooking), nil
}

func (h *BookingsHandler) getUserRole(ctx context.Context, userID string) (string, error) {
	userUUID, err := uuid.FromString(userID)
	if err != nil {
		return "", fmt.Errorf("invalid user_id format: %w", err)
	}
	u, err := user.GetUserByID(ctx, userUUID)
	if err != nil {
		h.logger.Error(ctx, "failed to get user role", err, map[string]any{
			"user_id": userID,
		})
		return "", fmt.Errorf("failed to get user role: %w", err)
	}
	return u.UserType, nil
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
		ID:                    booking.ID,
		CustomerID:            booking.CustomerID,
		ArtisanID:             artisanID,
		ServiceCategoryID:     booking.ServiceCategoryID,
		Title:                 booking.Title,
		Description:           booking.Description,
		CustomerAddressID:     booking.CustomerAddressID,
		Status:                booking.Status,
		Priority:              booking.Priority,
		ScheduledAt:           scheduledAt,
		EstimatedDurationMins: booking.EstimatedDurationMins,
		Metadata:              booking.Metadata,
		CreatedAt:             booking.CreatedAt,
		UpdatedAt:             booking.UpdatedAt,
	}
}
