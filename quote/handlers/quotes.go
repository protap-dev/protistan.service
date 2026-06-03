package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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
	BookingID             string                      `json:"booking_id"`
	AmountCents           int64                       `json:"amount_cents"`
	Currency              string                      `json:"currency"`
	EstimatedDurationMins int                         `json:"estimated_duration_mins"`
	Notes                 string                      `json:"notes,omitempty"`
	ValidUntil            *string                     `json:"valid_until,omitempty"` // ISO 8601 format
	Breakdown             []quotedomain.BreakdownItem `json:"breakdown,omitempty"`
}

// QuoteResponse represents a quote in responses
type QuoteResponse struct {
	ID                    string                      `json:"id"`
	BookingID             string                      `json:"booking_id"`
	Version               int                         `json:"version"`
	State                 string                      `json:"state"`
	AmountCents           int64                       `json:"amount_cents"`
	Currency              string                      `json:"currency"`
	Breakdown             []quotedomain.BreakdownItem `json:"breakdown,omitempty"`
	EstimatedDurationMins int                         `json:"estimated_duration_mins"`
	Notes                 string                      `json:"notes,omitempty"`
	ValidUntil            *time.Time                  `json:"valid_until,omitempty"`
	ProposedBy            string                      `json:"proposed_by"`
	ProposedAt            time.Time                   `json:"proposed_at"`
	DecisionBy            *string                     `json:"decision_by,omitempty"`
	DecidedAt             *time.Time                  `json:"decided_at,omitempty"`
	RejectionReasonCode   *string                     `json:"rejection_reason_code,omitempty"`
	RejectionReasonText   *string                     `json:"rejection_reason_text,omitempty"`
	CreatedAt             time.Time                   `json:"created_at"`
	UpdatedAt             time.Time                   `json:"updated_at"`
}

// ListQuotesResponse represents multiple quotes
type ListQuotesResponse struct {
	Quotes []*QuoteResponse `json:"quotes"`
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

	// 2. Get booking
	bookingResp, err := booking.GetBooking(ctx, req.BookingID)
	if err != nil {
		h.logger.Error(ctx, "failed to get booking", err, map[string]any{
			"booking_id": req.BookingID,
		})
		return nil, errs.B().Code(errs.NotFound).Msg("booking not found").Err()
	}

	// 3. Authorize that the user can propose a quote for this booking
	if err := qinternal.AuthorizeProposeQuote(ctx, userCtx.ID, bookingResp.ArtisanID, string(bookingResp.Status)); err != nil {
		h.logger.Error(ctx, "user not authorized to propose quote", err, map[string]any{
			"user_id":    userCtx.ID,
			"booking_id": req.BookingID,
		})
		return nil, err
	}

	// 4. Parse expiration time from request
	var validUntil *time.Time
	if req.ValidUntil != nil {
		parsedTime, err := time.Parse(time.RFC3339, *req.ValidUntil)
		if err != nil {
			return nil, errs.B().Code(errs.InvalidArgument).Msg("invalid valid_until format, use ISO 8601").Err()
		}
		validUntil = &parsedTime
	}

	// 5. Construct input for domain service
	input := &quotedomain.ProposeQuoteInput{
		BookingID:             req.BookingID,
		AmountCents:           req.AmountCents,
		Currency:              req.Currency,
		EstimatedDurationMins: req.EstimatedDurationMins,
		ValidUntil:            validUntil,
		Notes:                 req.Notes,
		ProposedBy:            userCtx.ID, // The service will resolve this to an artisan ID
		Breakdown:             req.Breakdown,
	}

	// 6. Call domain service to propose the quote
	quote, err := h.quoteSvc.ProposeQuote(ctx, input)
	if err != nil {
		// Domain service handles validation, so we can just bubble up the error
		h.logger.Error(ctx, "failed to propose quote", err, map[string]any{
			"booking_id": req.BookingID,
		})
		if errors.Is(err, quotedomain.ErrActiveQuoteExists) {
			return nil, errs.B().Code(errs.FailedPrecondition).Msg(quotedomain.ErrActiveQuoteExists.Message).Err()
		}
		return nil, err // Let the framework handle the error type
	}

	// 7. Clear any cached data
	h.clearQuoteCache(ctx, quote.ID)
	h.clearBookingQuotesCache(ctx, quote.BookingID)

	h.logger.Info(ctx, "quote proposed successfully", map[string]interface{}{
		"quote_id":   quote.ID,
		"booking_id": quote.BookingID,
		"version":    quote.Version,
	})

	return toQuoteResponse(quote), nil
}

// GetQuote retrieves a single quote by ID
func (h *QuotesHandler) GetQuote(ctx context.Context, id string) (*QuoteResponse, error) {
	h.logger.Info(ctx, "getting quote", map[string]any{"quote_id": id})

	// 1. Check cache first
	cacheKey := fmt.Sprintf("quote:%s", id)
	if cached, found := h.cache.Get(ctx, cacheKey); found {
		if quote, ok := cached.(*quotedomain.Quote); ok {
			h.logger.Info(ctx, "quote found in cache", map[string]any{"quote_id": id})
			return toQuoteResponse(quote), nil
		}
	}

	// 2. Get quote from repository
	quote, err := h.repo.GetByID(ctx, id)
	if err != nil {
		h.logger.Error(ctx, "failed to get quote", err, map[string]any{"quote_id": id})
		return nil, errs.B().Code(errs.NotFound).Msg("quote not found").Err()
	}

	// 3. Authorize access
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "get_quote")
	if err != nil {
		return nil, err
	}
	bookingResp, err := booking.GetBooking(ctx, quote.BookingID)
	if err != nil {
		return nil, errs.B().Code(errs.NotFound).Msg("booking not found	").Err()
	}
	if err := qinternal.AuthorizeListQuotes(ctx, userCtx.ID, bookingResp.CustomerID, bookingResp.ArtisanID); err != nil {
		return nil, err
	}

	// 4. Cache the quote
	h.cache.Set(ctx, cacheKey, quote, 1*time.Hour)

	return toQuoteResponse(quote), nil
}

// ListQuotesByBooking retrieves all quotes for a booking
func (h *QuotesHandler) ListQuotesByBooking(ctx context.Context, bookingID string) (*ListQuotesResponse, error) {
	h.logger.Info(ctx, "listing quotes for booking", map[string]any{
		"booking_id": bookingID,
	})

	// 1. Extract user context
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "listquotes")
	if err != nil {
		h.logger.Error(ctx, "failed to extract user context", err, nil)
		return nil, err
	}

	// 2. Get booking to verify access
	bookingResp, err := booking.GetBooking(ctx, bookingID)
	if err != nil {
		h.logger.Error(ctx, "failed to get booking", err, map[string]any{
			"booking_id": bookingID,
		})
		return nil, errs.B().Code(errs.NotFound).Msg("booking not found").Err()
	}

	// 3. Authorize access - only customer or assigned artisan can view quotes
	if err := qinternal.AuthorizeListQuotes(ctx, userCtx.ID, bookingResp.CustomerID, bookingResp.ArtisanID); err != nil {
		h.logger.Error(ctx, "user not authorized to view quotes", err, map[string]any{
			"user_id":    userCtx.ID,
			"booking_id": bookingID,
		})
		return nil, err
	}

	// 4. Get all quotes for the booking
	quotes, err := h.repo.GetByBookingID(ctx, bookingID)
	if err != nil {
		h.logger.Error(ctx, "failed to get quotes", err, map[string]any{
			"booking_id": bookingID,
		})
		return nil, errs.B().Code(errs.Internal).Msg("failed to retrieve quotes").Err()
	}

	// 5. Convert to response format
	quoteResponses := make([]*QuoteResponse, len(quotes))
	for i, quote := range quotes {
		quoteResponses[i] = toQuoteResponse(quote)
	}

	h.logger.Info(ctx, "quotes retrieved successfully", map[string]any{
		"booking_id":  bookingID,
		"quote_count": len(quotes),
	})

	return &ListQuotesResponse{
		Quotes: quoteResponses,
	}, nil
}

// ============================================================================
// HELPER METHODS
// ============================================================================

// toQuoteResponse converts domain Quote to API response
func toQuoteResponse(quote *quotedomain.Quote) *QuoteResponse {
	var breakdown []quotedomain.BreakdownItem
	if quote.Breakdown != nil {
		_ = json.Unmarshal(quote.Breakdown, &breakdown)
	}

	return &QuoteResponse{
		ID:                    quote.ID,
		BookingID:             quote.BookingID,
		Version:               quote.Version,
		State:                 string(quote.State),
		AmountCents:           quote.AmountCents,
		Currency:              quote.Currency,
		Breakdown:             breakdown,
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

// clearBookingQuotesCache clears the cache for a list of quotes for a booking
func (h *QuotesHandler) clearBookingQuotesCache(ctx context.Context, bookingID string) {
	cacheKey := fmt.Sprintf("quotes:booking:%s", bookingID)
	h.cache.Delete(ctx, cacheKey)
}

// handleExpiredQuoteSync triggers immediate expiration when an expired quote is interacted with
func (h *QuotesHandler) handleExpiredQuoteSync(ctx context.Context, quoteID string, action string) {
	h.logger.Info(ctx, "triggering immediate expiration for expired quote during attempt", map[string]any{
		"quote_id": quoteID,
		"action":   action,
	})

	if err := h.quoteSvc.ExpireQuote(ctx, quoteID); err != nil {
		h.logger.Error(ctx, fmt.Sprintf("failed to trigger immediate expiration for quote on %s", action), err, map[string]any{
			"quote_id": quoteID,
		})
	}
}
