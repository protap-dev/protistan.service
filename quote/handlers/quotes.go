package handlers

import (
	"context"
	"fmt"
	"slices"
	"time"

	"encore.app/artisans"
	"encore.app/booking"
	"encore.app/core"
	"encore.app/core/cache"
	quotedomain "encore.app/quote/domain"
	qinternal "encore.app/quote/internal"
	"encore.dev/beta/errs"
)

// QuotesHandler handles quote-related business logic
type QuotesHandler struct {
	repo       quotedomain.QuoteRepository
	quoteSvc   *quotedomain.QuoteService // Added
	validator  *quotedomain.Validator
	logger     qinternal.ServiceLogger
	cache      cache.CacheManager
	coreSvc    *core.CoreService
	authHelper *qinternal.AuthHelper
	publisher  quotedomain.EventPublisher
}

// NewQuotesHandler creates a new quotes handler
func NewQuotesHandler(
	repo quotedomain.QuoteRepository,
	quoteSvc *quotedomain.QuoteService, // Added
	validator *quotedomain.Validator,
	logger qinternal.ServiceLogger,
	cache cache.CacheManager,
	coreSvc *core.CoreService,
	authHelper *qinternal.AuthHelper,
	publisher quotedomain.EventPublisher,
) *QuotesHandler {
	return &QuotesHandler{
		repo:       repo,
		quoteSvc:   quoteSvc, // Added
		validator:  validator,
		logger:     logger,
		cache:      cache,
		coreSvc:    coreSvc,
		authHelper: authHelper,
		publisher:  publisher,
	}
}

// ============================================================================
// REQUEST/RESPONSE TYPES
// ============================================================================

// ProposeQuoteRequest represents a quote proposal request
type ProposeQuoteRequest struct {
	BookingID             string  `json:"booking_id"`
	AmountCents           int64   `json:"amount_cents"`
	Currency              string  `json:"currency"`
	EstimatedDurationMins int     `json:"estimated_duration_mins"`
	Notes                 string  `json:"notes,omitempty"`
	ValidUntil            *string `json:"valid_until,omitempty"` // ISO 8601 format
}

// QuoteResponse represents a quote in responses
type QuoteResponse struct {
	ID                    string     `json:"id"`
	BookingID             string     `json:"booking_id"`
	Version               int        `json:"version"`
	State                 string     `json:"state"`
	AmountCents           int64      `json:"amount_cents"`
	Currency              string     `json:"currency"`
	EstimatedDurationMins int        `json:"estimated_duration_mins"`
	Notes                 string     `json:"notes,omitempty"`
	ValidUntil            *time.Time `json:"valid_until,omitempty"`
	ProposedBy            string     `json:"proposed_by"`
	ProposedAt            time.Time  `json:"proposed_at"`
	DecisionBy            *string    `json:"decision_by,omitempty"`
	DecidedAt             *time.Time `json:"decided_at,omitempty"`
	RejectionReasonCode   *string    `json:"rejection_reason_code,omitempty"`
	RejectionReasonText   *string    `json:"rejection_reason_text,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// ============================================================================
// HANDLER IMPLEMENTATIONS
// ============================================================================

// ProposeQuote creates and proposes a quote for a booking
func (h *QuotesHandler) ProposeQuote(ctx context.Context, req *ProposeQuoteRequest) (*QuoteResponse, error) {
	h.logger.Info(ctx, "proposing quote", map[string]any{
		"booking_id":   req.BookingID,
		"amount_cents": req.AmountCents,
	})

	// 1. Extract and validate user context
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "propose_quote")
	if err != nil {
		h.logger.Error(ctx, "failed to extract user context", err, nil)
		return nil, err
	}

	// 2. Verify user is an artisan
	artisanResp, err := artisans.GetArtisanIDByUserID(ctx, userCtx.ID)
	if err != nil {
		h.logger.Error(ctx, "failed to get artisan profile", err, map[string]interface{}{
			"user_id": userCtx.ID,
		})
		return nil, errs.B().Code(errs.PermissionDenied).Msg("artisan profile not found").Err()
	}
	if !artisanResp.Found {
		return nil, errs.B().Code(errs.PermissionDenied).Msg("only artisans can propose quotes").Err()
	}
	artisanID := artisanResp.ArtisanID

	// 3. Validate booking exists and is in correct state
	bookingResp, err := booking.GetBooking(ctx, req.BookingID)
	if err != nil {
		h.logger.Error(ctx, "failed to get booking", err, map[string]any{
			"booking_id": req.BookingID,
		})
		return nil, errs.B().Code(errs.NotFound).Msg("booking not found").Err()
	}

	// 4. Verify artisan is assigned to this booking
	if bookingResp.ArtisanID == nil || *bookingResp.ArtisanID != artisanID {
		h.logger.Error(ctx, "artisan not assigned to booking", nil, map[string]interface{}{
			"booking_id":         req.BookingID,
			"artisan_id":         artisanID,
			"booking_artisan_id": bookingResp.ArtisanID,
		})
		return nil, errs.B().Code(errs.PermissionDenied).Msg("not assigned to this booking").Err()
	}

	// 5. Validate booking is in a state where quotes can be proposed
	validStates := []string{"assigned", "pending_quote", "quote_rejected"}
	if !contains(validStates, string(bookingResp.Status)) {
		h.logger.Error(ctx, "booking not in valid state for quote", nil, map[string]any{
			"booking_id":     req.BookingID,
			"booking_status": bookingResp.Status,
			"valid_states":   validStates,
		})
		return nil, errs.B().
			Code(errs.FailedPrecondition).
			Msgf("cannot propose quote for booking in state '%s'", bookingResp.Status).
			Err()
	}

	// 6. Parse expiration time from request
	var validUntil *time.Time
	if req.ValidUntil != nil {
		parsedTime, err := time.Parse(time.RFC3339, *req.ValidUntil)
		if err != nil {
			return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid valid_until format, use ISO 8601").Err()
		}
		validUntil = &parsedTime
	}

	// 7. Construct input for domain service
	input := &quotedomain.ProposeQuoteInput{
		BookingID:             req.BookingID,
		AmountCents:           req.AmountCents,
		Currency:              req.Currency,
		EstimatedDurationMins: req.EstimatedDurationMins,
		ValidUntil:            validUntil,
		Notes:                 req.Notes,
		ProposedBy:            artisanID,
	}

	// 8. Call domain service to propose the quote
	quote, err := h.quoteSvc.ProposeQuote(ctx, input)
	if err != nil {
		// Domain service handles validation, so we can just bubble up the error
		h.logger.Error(ctx, "failed to propose quote", err, map[string]any{
			"booking_id": req.BookingID,
		})
		return nil, err // Let the framework handle the error type
	}

	// 9. Clear any cached data
	h.clearQuoteCache(ctx, quote.ID)

	h.logger.Info(ctx, "quote proposed successfully", map[string]interface{}{
		"quote_id":   quote.ID,
		"booking_id": quote.BookingID,
		"version":    quote.Version,
	})

	return toQuoteResponse(quote), nil
}

// ============================================================================
// HELPER METHODS
// ============================================================================

// toQuoteResponse converts domain Quote to API response
func toQuoteResponse(quote *quotedomain.Quote) *QuoteResponse {
	return &QuoteResponse{
		ID:                    quote.ID,
		BookingID:             quote.BookingID,
		Version:               quote.Version,
		State:                 string(quote.State),
		AmountCents:           quote.AmountCents,
		Currency:              quote.Currency,
		EstimatedDurationMins: quote.EstimatedDurationMins,
		Notes:                 quote.Notes,
		ValidUntil:            quote.ValidUntil,
		ProposedBy:            quote.ProposedBy,
		ProposedAt:            quote.ProposedAt,
		DecisionBy:            quote.DecisionBy,
		DecidedAt:             quote.DecidedAt,
		RejectionReasonCode:   quote.RejectionReasonCode,
		RejectionReasonText:   quote.RejectionReasonText,
		CreatedAt:             quote.CreatedAt,
		UpdatedAt:             quote.UpdatedAt,
	}
}

// clearQuoteCache clears cached quote data
func (h *QuotesHandler) clearQuoteCache(ctx context.Context, quoteID string) {
	cacheKey := fmt.Sprintf("quote:%s", quoteID)
	h.cache.Delete(ctx, cacheKey)
}

// contains checks if a string slice contains a value
func contains(slice []string, val string) bool {
	return slices.Contains(slice, val)
}
