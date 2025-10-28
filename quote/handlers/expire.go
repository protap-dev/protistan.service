package handlers

import (
	"context"
	"time"

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
		h.logger.Info(ctx, "no expired quotes found", map[string]any{
			"scanned_at": time.Now(),
		})
		return nil
	}

	h.logger.Info(ctx, "found expired quotes", map[string]any{
		"count": len(expiredQuotes),
	})

	// Process each expired quote atomically
	successCount := 0
	errorCount := 0

	for _, quote := range expiredQuotes {
		err := h.expireQuote(ctx, quote)
		if err != nil {
			h.logger.Error(ctx, "failed to expire quote", err, map[string]any{
				"quote_id":   quote.ID,
				"booking_id": quote.BookingID,
			})
			errorCount++
			continue
		}
		successCount++
	}

	h.logger.Info(ctx, "quote expiry scan completed", map[string]any{
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

// expireQuote transitions a single quote to expired status by calling the domain service.
func (h *QuotesHandler) expireQuote(ctx context.Context, quote *quotedomain.Quote) error {
	// The domain service handles the transaction and all business logic,
	// including idempotency checks.
	err := h.quoteSvc.ExpireQuote(ctx, quote.ID)
	if err != nil {
		// The service layer logs errors, but we log here as well to capture the context of the cron job.
		h.logger.Error(ctx, "failed to expire quote via domain service", err, map[string]any{
			"quote_id": quote.ID,
		})
		return err
	}

	h.logger.Info(ctx, "successfully processed quote for expiration", map[string]any{
		"quote_id": quote.ID,
	})
	return nil
}
