package handlers

import (
	"context"
	"errors"

	"encore.app/booking"
	quotedomain "encore.app/quote/domain"
	qinternal "encore.app/quote/internal"
	"encore.dev/beta/errs"
)

// RejectQuoteRequest represents rejecting a quote
type RejectQuoteRequest struct {
	BookingID  string  `json:"booking_id"`
	ReasonCode string  `json:"reason_code"`
	ReasonText *string `json:"reason_text,omitempty"`
}

// ============================================================================
// REJECT QUOTE HANDLER
// ============================================================================
func (h *QuotesHandler) RejectQuote(ctx context.Context, id string, req *RejectQuoteRequest) (*QuoteResponse, error) {
	h.logger.Info(ctx, "rejecting quote", map[string]any{
		"quote_id":    id,
		"booking_id":  req.BookingID,
		"reason_code": req.ReasonCode,
	})

	// 1. Extract user context
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "reject_quote")
	if err != nil {
		h.logger.Error(ctx, "failed to extract user context", err, nil)
		return nil, err
	}

	// 2. Get the quote to verify booking ID
	quote, err := h.repo.GetByID(ctx, id)
	if err != nil {
		h.logger.Error(ctx, "failed to get quote", err, map[string]any{
			"quote_id": id,
		})
		return nil, errs.B().Code(errs.NotFound).Msg("quote not found").Err()
	}

	// 3. Verify booking ID matches
	if quote.BookingID != req.BookingID {
		h.logger.Error(ctx, "booking id mismatch", nil, map[string]any{
			"quote_id":           id,
			"quote_booking_id":   quote.BookingID,
			"request_booking_id": req.BookingID,
		})
		return nil, errs.B().Code(errs.InvalidArgument).Msg("booking_id does not match quote").Err()
	}

	// 4. Get booking
	bookingResp, err := booking.GetBooking(ctx, quote.BookingID)
	if err != nil {
		h.logger.Error(ctx, "failed to get booking", err, map[string]any{
			"booking_id": quote.BookingID,
		})
		return nil, errs.B().Code(errs.NotFound).Msg("booking not found").Err()
	}

	// 5. Authorize that the user can reject a quote for this booking
	if err := qinternal.AuthorizeRejectQuote(ctx, userCtx.ID, bookingResp.CustomerID); err != nil {
		h.logger.Error(ctx, "user not authorized to reject quote", err, map[string]any{
			"user_id":    userCtx.ID,
			"booking_id": req.BookingID,
		})
		return nil, err
	}

	// 6. Construct input for domain service
	input := &quotedomain.RejectQuoteInput{
		DecisionBy: userCtx.ID,
		ReasonCode: req.ReasonCode,
		ReasonText: req.ReasonText,
	}

	// 7. Call domain service to reject the quote
	updatedQuote, err := h.quoteSvc.RejectQuote(ctx, id, input)
	if err != nil {
		// If quote is expired but state hasn't updated yet, expire it now
		if errors.Is(err, quotedomain.ErrQuoteExpired) {
			h.handleExpiredQuoteSync(ctx, id, "reject")
		}

		// Domain service handles validation, so we can just bubble up the error
		h.logger.Error(ctx, "failed to reject quote", err, map[string]any{
			"quote_id": id,
		})
		return nil, err // Let the framework handle the error type
	}

	// 8. Clear cache
	h.clearQuoteCache(ctx, updatedQuote.ID)
	h.clearBookingQuotesCache(ctx, updatedQuote.BookingID)

	h.logger.Info(ctx, "quote rejected successfully", map[string]any{
		"quote_id":    updatedQuote.ID,
		"booking_id":  updatedQuote.BookingID,
		"reason_code": req.ReasonCode,
	})

	return toQuoteResponse(updatedQuote), nil
}
