package handlers

import (
	"context"
	"time"

	"encore.app/booking"
	eventscommon "encore.app/core/events"
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

	// 2. Get the quote
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
	// In MVP, only customer can reject. In future, artisan could also withdraw/cancel their quote
	if bookingResp.CustomerID != userCtx.ID {
		h.logger.Error(ctx, "user not authorized to reject quote", nil, map[string]interface{}{
			"user_id":     userCtx.ID,
			"customer_id": bookingResp.CustomerID,
		})
		return nil, errs.B().Code(errs.PermissionDenied).Msg("only booking owner can reject quote").Err()
	}

	// 6. Validate rejection using domain validator
	if err := h.validator.ValidateRejectQuote(ctx, quote, &quotedomain.RejectQuoteInput{
		DecisionBy: userCtx.ID,
		ReasonCode: req.ReasonCode,
		ReasonText: req.ReasonText,
	}); err != nil {
		h.logger.Error(ctx, "quote rejection validation failed", err, map[string]interface{}{
			"quote_id": id,
			"state":    quote.State,
		})
		return nil, errs.B().Code(errs.FailedPrecondition).Msg(err.Error()).Err()
	}

	// 7. Update quote and publish event in transaction
	now := time.Now()
	var updatedQuote *quotedomain.Quote

	err = h.repo.WithTransaction(ctx, func(txRepo quotedomain.QuoteRepository) error {
		// Re-fetch within transaction
		current, err := txRepo.GetByID(ctx, id)
		if err != nil {
			return err
		}

		// Double-check state
		if current.State != quotedomain.QuoteProposed {
			return errs.B().
				Code(errs.FailedPrecondition).
				Msgf("quote is in state '%s', can only reject proposed quotes", current.State).
				Err()
		}

		// Update quote state
		previousState := current.State
		current.State = quotedomain.QuoteRejected
		current.DecisionBy = &userCtx.ID
		current.DecidedAt = &now
		current.RejectionReasonCode = &req.ReasonCode
		current.RejectionReasonText = req.ReasonText
		current.UpdatedAt = now
		current.DBVersion++

		// Save to database
		if err := txRepo.Update(ctx, current); err != nil {
			h.logger.Error(ctx, "failed to update quote", err, map[string]interface{}{
				"quote_id": id,
			})
			return err
		}

		// Create event for outbox
		event := &eventscommon.QuoteEvent{
			QuoteID:             current.ID,
			BookingID:           current.BookingID,
			Version:             current.Version,
			State:               string(quotedomain.QuoteRejected),
			PreviousState:       string(previousState),
			AmountCents:         current.AmountCents,
			Currency:            current.Currency,
			ProposedBy:          current.ProposedBy,
			DecisionBy:          &userCtx.ID,
			Timestamp:           now,
			UserID:              userCtx.ID,
			RejectionReasonCode: &req.ReasonCode,
		}

		// Store event in outbox
		if err := txRepo.CreateEventInOutbox(ctx, event); err != nil {
			h.logger.Error(ctx, "failed to create event in outbox", err, map[string]interface{}{
				"quote_id": id,
			})
			return err
		}

		updatedQuote = current
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Clear cache
	h.clearQuoteCache(ctx, updatedQuote.ID)

	h.logger.Info(ctx, "quote rejected successfully", map[string]interface{}{
		"quote_id":    updatedQuote.ID,
		"booking_id":  updatedQuote.BookingID,
		"reason_code": req.ReasonCode,
	})

	return toQuoteResponse(updatedQuote), nil
}
