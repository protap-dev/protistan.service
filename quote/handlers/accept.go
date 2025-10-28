package handlers

import (
	"context"
	"time"

	"encore.app/booking"
	eventscommon "encore.app/core/events"
	quotedomain "encore.app/quote/domain"
	"encore.dev/beta/errs"
)

// AcceptQuoteRequest represents accepting a quote
type AcceptQuoteRequest struct {
	BookingID string `json:"booking_id"`
}

// AcceptQuote accepts a proposed quote
func (h *QuotesHandler) AcceptQuote(ctx context.Context, id string, req *AcceptQuoteRequest) (*QuoteResponse, error) {
	h.logger.Info(ctx, "accepting quote", map[string]any{
		"quote_id":   id,
		"booking_id": req.BookingID,
	})

	// 1. Extract user context
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "accept_quote")
	if err != nil {
		h.logger.Error(ctx, "failed to extract user context", err, nil)
		return nil, err
	}

	// 2. Get the quote from database
	quote, err := h.repo.GetByID(ctx, id)
	if err != nil {
		h.logger.Error(ctx, "failed to get quote", err, map[string]interface{}{
			"quote_id": id,
		})
		return nil, errs.B().Code(errs.NotFound).Msg("quote not found").Err()
	}

	// 3. Verify booking ID matches (security check)
	if quote.BookingID != req.BookingID {
		h.logger.Error(ctx, "booking id mismatch", nil, map[string]interface{}{
			"quote_id":           id,
			"quote_booking_id":   quote.BookingID,
			"request_booking_id": req.BookingID,
		})
		return nil, errs.B().Code(errs.InvalidArgument).Msg("booking_id does not match quote").Err()
	}

	// 4. Get booking to verify customer ownership
	bookingResp, err := booking.GetBooking(ctx, quote.BookingID)
	if err != nil {
		h.logger.Error(ctx, "failed to get booking", err, map[string]interface{}{
			"booking_id": quote.BookingID,
		})
		return nil, errs.B().Code(errs.NotFound).Msg("booking not found").Err()
	}

	// 5. Verify user is the customer who owns this booking
	if bookingResp.CustomerID != userCtx.ID {
		h.logger.Error(ctx, "user not authorized to accept quote", nil, map[string]interface{}{
			"user_id":     userCtx.ID,
			"customer_id": bookingResp.CustomerID,
		})
		return nil, errs.B().Code(errs.PermissionDenied).Msg("only booking owner can accept quote").Err()
	}

	// 6. Validate quote can be accepted (domain validation)
	if err := h.validator.ValidateAcceptQuote(ctx, quote, userCtx.ID); err != nil {
		h.logger.Error(ctx, "quote acceptance validation failed", err, map[string]interface{}{
			"quote_id": id,
			"state":    quote.State,
		})
		return nil, errs.B().Code(errs.FailedPrecondition).Msg(err.Error()).Err()
	}

	// 7. Update quote and publish event in transaction
	now := time.Now()
	var updatedQuote *quotedomain.Quote

	err = h.repo.WithTransaction(ctx, func(txRepo quotedomain.QuoteRepository) error {
		// Re-fetch quote within transaction for optimistic locking
		current, err := txRepo.GetByID(ctx, id)
		if err != nil {
			return err
		}

		// Double-check state (defensive programming)
		if current.State != quotedomain.QuoteProposed {
			return errs.B().
				Code(errs.FailedPrecondition).
				Msgf("quote is in state '%s', can only accept proposed quotes", current.State).
				Err()
		}

		// Check if expired
		if current.ValidUntil != nil && time.Now().After(*current.ValidUntil) {
			return errs.B().Code(errs.FailedPrecondition).Msg("quote has expired").Err()
		}

		// Update quote state
		previousState := current.State
		current.State = quotedomain.QuoteAccepted
		current.DecisionBy = &userCtx.ID
		current.DecidedAt = &now
		current.UpdatedAt = now
		current.DBVersion++ // Increment version for optimistic locking

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
			State:               string(quotedomain.QuoteAccepted),
			PreviousState:       string(previousState),
			AmountCents:         current.AmountCents,
			Currency:            current.Currency,
			ProposedBy:          current.ProposedBy,
			DecisionBy:          &userCtx.ID,
			Timestamp:           now,
			UserID:              userCtx.ID,
			RejectionReasonCode: nil,
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

	h.logger.Info(ctx, "quote accepted successfully", map[string]interface{}{
		"quote_id":   updatedQuote.ID,
		"booking_id": updatedQuote.BookingID,
	})

	return toQuoteResponse(updatedQuote), nil
}
