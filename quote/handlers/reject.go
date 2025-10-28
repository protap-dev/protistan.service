package handlers

import (
	"context"

	"encore.app/booking"
	quotedomain "encore.app/quote/domain"
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

	// 2. Get the quote to verify booking ID and ownership
	quote, err := h.repo.GetByID(ctx, id)
	if err != nil {
		h.logger.Error(ctx, "failed to get quote", err, map[string]any{
			"quote_id": id,
		})
		return nil, errs.B().Code(errs.NotFound).Msg("quote not found").Err()
	}

	// 3. Verify booking ID matches
	if quote.BookingID != req.BookingID {
		h.logger.Error(ctx, "booking id mismatch", nil, map[string]interface{}{
			"quote_id":           id,
			"quote_booking_id":   quote.BookingID,
			"request_booking_id": req.BookingID,
		})
		return nil, errs.B().Code(errs.InvalidArgument).Msg("booking_id does not match quote").Err()
	}

	// 4. Get booking to verify authorization
	bookingResp, err := booking.GetBooking(ctx, quote.BookingID)
	if err != nil {
		h.logger.Error(ctx, "failed to get booking", err, map[string]interface{}{
			"booking_id": quote.BookingID,
		})
		return nil, errs.B().Code(errs.NotFound).Msg("booking not found").Err()
	}

	// 5. Verify user is authorized to reject (customer who owns booking)
	if bookingResp.CustomerID != userCtx.ID {
		h.logger.Error(ctx, "user not authorized to reject quote", nil, map[string]interface{}{
			"user_id":     userCtx.ID,
			"customer_id": bookingResp.CustomerID,
		})
		return nil, errs.B().Code(errs.PermissionDenied).Msg("only booking owner can reject quote").Err()
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
		// Domain service handles validation, so we can just bubble up the error
		h.logger.Error(ctx, "failed to reject quote", err, map[string]interface{}{
			"quote_id": id,
		})
		return nil, err // Let the framework handle the error type
	}

	// 8. Clear cache
	h.clearQuoteCache(ctx, updatedQuote.ID)

	h.logger.Info(ctx, "quote rejected successfully", map[string]interface{}{
		"quote_id":    updatedQuote.ID,
		"booking_id":  updatedQuote.BookingID,
		"reason_code": req.ReasonCode,
	})

	return toQuoteResponse(updatedQuote), nil
}
