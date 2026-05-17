package handlers

import (
	"context"
	"strconv"
	"time"

	pdomain "encore.app/payment/domain"
)

func (h *PaymentsHandler) resolveCurrentAttempt(ctx context.Context, txn *pdomain.Transaction, userID string) (*pdomain.PaymentAttempt, error) {
	if txn.CurrentAttemptID != nil && *txn.CurrentAttemptID != "" {
		return h.repo.GetCurrentAttemptByTransactionID(ctx, txn.ID)
	}

	if txn.Metadata["checkout_url"] == "" && txn.ProviderRef == "" {
		return nil, nil
	}

	return h.createLegacyAttemptFromTransaction(ctx, txn, userID)
}

func (h *PaymentsHandler) createLegacyAttemptFromTransaction(ctx context.Context, txn *pdomain.Transaction, userID string) (*pdomain.PaymentAttempt, error) {
	var attempt *pdomain.PaymentAttempt
	if err := h.repo.WithTransaction(ctx, func(txRepo pdomain.TransactionRepository) error {
		count, err := txRepo.CountAttemptsByTransactionID(ctx, txn.ID)
		if err != nil {
			return err
		}
		if count > 0 {
			attempt, err = txRepo.GetCurrentAttemptByTransactionID(ctx, txn.ID)
			return err
		}

		var expiresAt *time.Time
		if raw := txn.Metadata["return_ctx_expires_at"]; raw != "" {
			if unix, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil {
				parsed := time.Unix(unix, 0)
				expiresAt = &parsed
			}
		}

		status := pdomain.AttemptFailed
		if txn.Status == pdomain.TxPending && txn.Metadata["checkout_url"] != "" {
			status = pdomain.AttemptActive
		}

		attempt = &pdomain.PaymentAttempt{
			TransactionID:          txn.ID,
			AttemptNo:              int(count) + 1,
			Provider:               txn.Provider,
			Status:                 status,
			InternalRef:            txn.InternalRef,
			ProviderRef:            txn.ProviderRef,
			CheckoutURL:            txn.Metadata["checkout_url"],
			ReturnURL:              txn.Metadata["return_url"],
			ReturnContext:          txn.Metadata["return_ctx"],
			ReturnContextExpiresAt: expiresAt,
			CreatedByUserID:        userID,
			CreatedAt:              txn.CreatedAt,
			UpdatedAt:              time.Now(),
		}
		if err := txRepo.CreateAttempt(ctx, attempt); err != nil {
			return err
		}

		txn.CurrentAttemptID = &attempt.ID
		txn.UpdatedAt = time.Now()
		return txRepo.Update(ctx, txn)
	}); err != nil {
		return nil, err
	}

	return attempt, nil
}

func (h *PaymentsHandler) attemptRequiresCancellation(attempt *pdomain.PaymentAttempt) bool {
	switch attempt.Status {
	case pdomain.AttemptActive, pdomain.AttemptInitializing, pdomain.AttemptCancelRequested:
		return true
	default:
		return false
	}
}

func (h *PaymentsHandler) createFreshAttempt(ctx context.Context, txn *pdomain.Transaction, previousAttempt *pdomain.PaymentAttempt, userID string) (*pdomain.PaymentAttempt, error) {
	var attempt *pdomain.PaymentAttempt
	if err := h.repo.WithTransaction(ctx, func(txRepo pdomain.TransactionRepository) error {
		count, err := txRepo.CountAttemptsByTransactionID(ctx, txn.ID)
		if err != nil {
			return err
		}

		now := time.Now()
		attempt = &pdomain.PaymentAttempt{
			TransactionID:   txn.ID,
			AttemptNo:       int(count) + 1,
			Provider:        txn.Provider,
			Status:          pdomain.AttemptInitializing,
			InternalRef:     h.generateInternalRef(txn.BookingID),
			CreatedByUserID: userID,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := txRepo.CreateAttempt(ctx, attempt); err != nil {
			return err
		}

		if previousAttempt != nil && (previousAttempt.Status == pdomain.AttemptCancelled || previousAttempt.Status == pdomain.AttemptSuperseded) {
			previousAttempt.SupersededByAttemptID = &attempt.ID
			previousAttempt.UpdatedAt = now
			if err := txRepo.UpdateAttempt(ctx, previousAttempt); err != nil {
				return err
			}
		}

		h.syncTransactionWithAttempt(txn, attempt)
		txn.UpdatedAt = now
		return txRepo.Update(ctx, txn)
	}); err != nil {
		return nil, err
	}

	return attempt, nil
}

func (h *PaymentsHandler) activateAttempt(ctx context.Context, txn *pdomain.Transaction, attempt *pdomain.PaymentAttempt, initRes *pdomain.InitializationResponse, returnURL, ctxToken string, expiresAt time.Time) error {
	now := time.Now()
	attempt.Status = pdomain.AttemptActive
	attempt.ProviderRef = initRes.OrderRef
	attempt.CheckoutURL = initRes.CheckoutLink
	attempt.ReturnURL = returnURL
	attempt.ReturnContext = ctxToken
	attempt.ReturnContextExpiresAt = &expiresAt
	attempt.UpdatedAt = now
	if err := h.repo.UpdateAttempt(ctx, attempt); err != nil {
		return err
	}

	h.syncTransactionWithAttempt(txn, attempt)
	txn.ProviderRef = initRes.OrderRef
	txn.UpdatedAt = now
	return h.repo.Update(ctx, txn)
}

func (h *PaymentsHandler) markAttemptInitializationFailed(ctx context.Context, txn *pdomain.Transaction, attempt *pdomain.PaymentAttempt, initErr error) {
	attempt.Status = pdomain.AttemptFailed
	attempt.UpdatedAt = time.Now()
	_ = h.repo.UpdateAttempt(ctx, attempt)

	if txn.Metadata == nil {
		txn.Metadata = map[string]string{}
	}
	txn.Metadata["last_initialize_attempt_id"] = attempt.ID
	txn.Metadata["last_initialize_error"] = sanitizeProviderError(initErr)
	txn.UpdatedAt = time.Now()
	_ = h.repo.Update(ctx, txn)
}

func (h *PaymentsHandler) syncTransactionWithAttempt(txn *pdomain.Transaction, attempt *pdomain.PaymentAttempt) {
	if txn.Metadata == nil {
		txn.Metadata = map[string]string{}
	}

	txn.CurrentAttemptID = &attempt.ID
	txn.InternalRef = attempt.InternalRef
	txn.ProviderRef = attempt.ProviderRef

	delete(txn.Metadata, "checkout_url")
	delete(txn.Metadata, "return_url")
	delete(txn.Metadata, "return_ctx")
	delete(txn.Metadata, "return_ctx_expires_at")

	if attempt.CheckoutURL != "" {
		txn.Metadata["checkout_url"] = attempt.CheckoutURL
	}
	if attempt.ReturnURL != "" {
		txn.Metadata["return_url"] = attempt.ReturnURL
	}
	if attempt.ReturnContext != "" {
		txn.Metadata["return_ctx"] = attempt.ReturnContext
	}
	if attempt.ReturnContextExpiresAt != nil {
		txn.Metadata["return_ctx_expires_at"] = strconv.FormatInt(attempt.ReturnContextExpiresAt.Unix(), 10)
		txn.Metadata[paymentReservationReservedUntil] = attempt.ReturnContextExpiresAt.UTC().Format(time.RFC3339)
	}
}
