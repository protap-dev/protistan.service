package handlers

import (
	"context"
	"time"

	eventscommon "encore.app/core/events"
	quotedomain "encore.app/quote/domain"
	"encore.dev/beta/errs"
)

// ============================================================================
// EXPIRY HANDLER
// ============================================================================

// ExpireQuotes scans for expired pending quotes and transitions them to expired status.
// This follows the same pattern as the booking service's ExpireOffers method.
func (h *QuotesHandler) ExpireQuotes(ctx context.Context) error {
	h.logger.Info(ctx, "starting quote expiry scan", nil)

	// Find all expired pending quotes
	expiredQuotes, err := h.repo.FindExpiredQuotes(ctx)
	if err != nil {
		h.logger.Error(ctx, "failed to find expired quotes", err, nil)
		return err
	}

	if len(expiredQuotes) == 0 {
		h.logger.Info(ctx, "no expired quotes found", map[string]interface{}{
			"scanned_at": time.Now(),
		})
		return nil
	}

	h.logger.Info(ctx, "found expired quotes", map[string]interface{}{
		"count": len(expiredQuotes),
	})

	// Process each expired quote atomically
	successCount := 0
	errorCount := 0

	for _, quote := range expiredQuotes {
		err := h.expireQuote(ctx, quote)
		if err != nil {
			h.logger.Error(ctx, "failed to expire quote", err, map[string]interface{}{
				"quote_id":   quote.ID,
				"booking_id": quote.BookingID,
			})
			errorCount++
			continue
		}
		successCount++
	}

	h.logger.Info(ctx, "quote expiry scan completed", map[string]interface{}{
		"success_count": successCount,
		"error_count":   errorCount,
		"total":         len(expiredQuotes),
	})

	// Return error if any quotes failed to process
	if errorCount > 0 {
		return errs.B().
			Code(errs.Internal).
			Msgf("failed to expire %d out of %d quotes", errorCount, len(expiredQuotes)).
			Err()
	}

	return nil
}

// expireQuote transitions a single quote to expired status and publishes event
func (h *QuotesHandler) expireQuote(ctx context.Context, quote *quotedomain.Quote) error {
	return h.repo.WithTransaction(ctx, func(txRepo quotedomain.QuoteRepository) error {
		// Re-fetch quote within transaction to ensure we have latest state
		current, err := txRepo.GetByID(ctx, quote.ID)
		if err != nil {
			return err
		}

		// Idempotency check: only process if still proposed
		if current.State != quotedomain.QuoteProposed {
			h.logger.Info(ctx, "quote already processed, skipping", map[string]any{
				"quote_id": quote.ID,
				"state":    current.State,
			})
			return nil
		}

		// Double-check expiry within transaction (defensive)
		if current.ValidUntil == nil || time.Now().Before(*current.ValidUntil) {
			h.logger.Info(ctx, "quote no longer expired, skipping", map[string]any{
				"quote_id":    quote.ID,
				"valid_until": current.ValidUntil,
			})
			return nil
		}

		// Update quote status to expired
		now := time.Now()
		current.State = quotedomain.QuoteExpired
		current.UpdatedAt = now

		if err := txRepo.Update(ctx, current); err != nil {
			return err
		}

		// Create quote expired event
		event := &eventscommon.QuoteEvent{
			QuoteID:             current.ID,
			BookingID:           current.BookingID,
			Version:             current.Version,
			State:               string(quotedomain.QuoteExpired),
			PreviousState:       string(quotedomain.QuoteProposed),
			AmountCents:         current.AmountCents,
			Currency:            current.Currency,
			ProposedBy:          current.ProposedBy,
			Timestamp:           now,
			UserID:              "system",
			RejectionReasonCode: nil,
		}

		// Write event to outbox for guaranteed delivery
		if err := txRepo.CreateEventInOutbox(ctx, event); err != nil {
			return err
		}

		h.logger.Info(ctx, "quote expired successfully", map[string]interface{}{
			"quote_id":   current.ID,
			"booking_id": current.BookingID,
		})

		return nil
	})
}
