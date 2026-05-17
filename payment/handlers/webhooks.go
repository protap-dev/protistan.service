package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	pdomain "encore.app/payment/domain"
	"encore.dev/beta/errs"
)

// NombaWebhook handles the existing Nomba webhook route.
func (h *PaymentsHandler) NombaWebhook(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if req.Header.Get("nomba-signature") == "" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ignored","reason":"no signature, test ping"}`))
		return
	}
	h.ProviderWebhook(defaultPaymentProvider, w, req)
}

// ProviderWebhook handles asynchronous payment notifications for a configured provider.
// It delegates provider-specific parsing/signature checks, then re-queries the provider
// before committing any local payment state.
func (h *PaymentsHandler) ProviderWebhook(providerID string, w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	providerID = strings.TrimSpace(strings.ToLower(providerID))
	if providerID == "" {
		http.Error(w, "payment provider is required", http.StatusBadRequest)
		return
	}

	provider, exists := h.providers[providerID]
	if !exists {
		http.Error(w, "payment provider not configured", http.StatusInternalServerError)
		return
	}

	ctx := req.Context()
	req.Body = http.MaxBytesReader(w, req.Body, maxWebhookBodyBytes)

	event, err := provider.ParseWebhook(ctx, req)
	if err != nil {
		log.Printf("payment webhook rejected: stage=parse error=%v", err)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	if event.RequestID == "" {
		log.Printf("payment webhook rejected: stage=parse error=missing_request_id")
		http.Error(w, "invalid webhook", http.StatusUnauthorized)
		return
	}
	processed, err := h.repo.HasWebhookEvent(ctx, providerID, event.RequestID)
	if err != nil {
		log.Printf("payment webhook lookup failed: request_id=%s error=%v", redactPaymentRef(event.RequestID), err)
		http.Error(w, "webhook lookup failed", http.StatusInternalServerError)
		return
	}
	if processed {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ignored","reason":"duplicate webhook"}`))
		return
	}

	attempt, err := h.repo.GetAttemptByInternalRef(ctx, event.OrderRef)
	if err != nil {
		log.Printf("payment webhook attempt lookup failed: ref=%s error=%v", redactPaymentRef(event.OrderRef), err)
		http.Error(w, "payment attempt lookup failed", http.StatusInternalServerError)
		return
	}
	if attempt == nil {
		http.Error(w, "payment attempt not found", http.StatusNotFound)
		return
	}
	if attempt.Provider != providerID {
		log.Printf("payment webhook provider mismatch: route_provider=%s attempt_provider=%s ref=%s", providerID, attempt.Provider, redactPaymentRef(event.OrderRef))
		http.Error(w, "payment attempt not found", http.StatusNotFound)
		return
	}

	verified, err := provider.Verify(ctx, event.OrderRef)
	if err != nil {
		log.Printf("payment webhook provider verification failed: ref=%s error=%v", redactPaymentRef(event.OrderRef), err)
		http.Error(w, "failed to verify transaction with provider", http.StatusInternalServerError)
		return
	}

	if err := h.processAttemptWebhook(ctx, providerID, attempt, verified, event); err != nil {
		log.Printf("payment webhook processing failed: ref=%s attempt=%s error=%v", redactPaymentRef(event.OrderRef), redactPaymentRef(attempt.ID), err)
		http.Error(w, "internal transaction error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"acknowledged"}`))
}

func (h *PaymentsHandler) processAttemptWebhook(ctx context.Context, providerID string, attempt *pdomain.PaymentAttempt, verified *pdomain.VerificationResponse, event *pdomain.WebhookEvent) error {
	var finalizedTxn *pdomain.Transaction
	var finalizedStatus pdomain.TransactionStatus

	if err := h.repo.WithTransaction(ctx, func(txRepo pdomain.TransactionRepository) error {
		if event != nil && event.RequestID != "" {
			receivedAt := event.ReceivedAt
			if receivedAt.IsZero() {
				receivedAt = time.Now()
			}
			if providerID == "" {
				providerID = attempt.Provider
			}
			recorded, err := txRepo.RecordWebhookEvent(ctx, providerID, event.RequestID, receivedAt)
			if err != nil {
				return err
			}
			if !recorded {
				return nil
			}
		}

		txn, err := txRepo.GetByID(ctx, attempt.TransactionID)
		if err != nil {
			return err
		}

		now := time.Now()
		statusKind := classifyProviderVerificationStatus(verified.Status)
		switch statusKind {
		case "SUCCESS":
			if err := h.validateVerifiedPayment(txn, attempt, verified); err != nil {
				return err
			}
			if attempt.Status != pdomain.AttemptSuccessful {
				attempt.Status = pdomain.AttemptSuccessful
				attempt.ProviderRef = verified.TransactionID
				attempt.UpdatedAt = now
				if err := txRepo.UpdateAttempt(ctx, attempt); err != nil {
					return err
				}
			}

			if txn.Status != pdomain.TxPending {
				return nil
			}
			if txn.CurrentAttemptID == nil || *txn.CurrentAttemptID != attempt.ID {
				if txn.Metadata == nil {
					txn.Metadata = map[string]string{}
				}
				txn.Metadata["successful_non_current_attempt_id"] = attempt.ID
				txn.Metadata["previous_current_attempt_id"] = ""
				if txn.CurrentAttemptID != nil {
					txn.Metadata["previous_current_attempt_id"] = *txn.CurrentAttemptID
				}
			}

			txn.Status = pdomain.TxSuccessful
			h.syncTransactionWithAttempt(txn, attempt)
			txn.UpdatedAt = now
			if err := txRepo.Update(ctx, txn); err != nil {
				return err
			}
			if err := h.createBookingStatusEvent(ctx, txRepo, txn, pdomain.TxSuccessful); err != nil {
				return err
			}
			finalizedTxn = txn
			finalizedStatus = pdomain.TxSuccessful
			return nil

		case "FAILED":
			if attempt.Status != pdomain.AttemptFailed {
				attempt.Status = pdomain.AttemptFailed
				attempt.UpdatedAt = now
				if err := txRepo.UpdateAttempt(ctx, attempt); err != nil {
					return err
				}
			}

			if txn.Status != pdomain.TxPending || txn.CurrentAttemptID == nil || *txn.CurrentAttemptID != attempt.ID {
				return nil
			}

			txn.Status = pdomain.TxFailed
			txn.UpdatedAt = now
			if err := txRepo.Update(ctx, txn); err != nil {
				return err
			}
			if err := h.createBookingStatusEvent(ctx, txRepo, txn, pdomain.TxFailed); err != nil {
				return err
			}
			finalizedTxn = txn
			finalizedStatus = pdomain.TxFailed
			return nil
		case "ACK_ONLY":
			log.Printf("payment webhook acknowledged terminal provider status without local state change: attempt=%s transaction=%s status=%s", redactPaymentRef(attempt.ID), redactPaymentRef(txn.ID), verified.Status)
			return nil
		default:
			log.Printf("payment webhook unhandled provider status: attempt=%s transaction=%s status=%s", redactPaymentRef(attempt.ID), redactPaymentRef(txn.ID), verified.Status)
			return fmt.Errorf("unhandled provider verification status: %s", verified.Status)
		}
	}); err != nil {
		return err
	}

	if finalizedTxn != nil {
		h.markPaymentStatusEventEnqueued(ctx, finalizedTxn, finalizedStatus)
	}

	return nil
}

func (h *PaymentsHandler) validateVerifiedPayment(txn *pdomain.Transaction, attempt *pdomain.PaymentAttempt, verified *pdomain.VerificationResponse) error {
	if verified == nil {
		return fmt.Errorf("missing provider verification response")
	}
	if verified.OrderRef != "" && attempt != nil && verified.OrderRef != attempt.InternalRef {
		return fmt.Errorf("provider verification reference mismatch")
	}
	verifiedAmountCents := verified.AmountCents
	if verifiedAmountCents <= 0 {
		return fmt.Errorf("provider verification amount missing")
	}
	if txn.AmountCents != verifiedAmountCents {
		return fmt.Errorf("provider verification amount mismatch")
	}
	if verified.Currency != "" && !strings.EqualFold(txn.Currency, verified.Currency) {
		return fmt.Errorf("provider verification currency mismatch")
	}
	return nil
}

func classifyProviderVerificationStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "SUCCESS", "SUCCESSFUL", "PAID", "COMPLETED", "COMPLETE":
		return "SUCCESS"
	case "FAILED", "FAILURE", "CANCELLED", "CANCELED", "EXPIRED", "DECLINED", "REJECTED", "ABANDONED", "TIMEOUT", "TIMED_OUT":
		return "FAILED"
	case "REVERSED", "REVERSAL", "REFUNDED", "CHARGEBACK":
		return "ACK_ONLY"
	default:
		return "UNKNOWN"
	}
}

func (h *PaymentsHandler) cancelAttemptForRetry(ctx context.Context, provider pdomain.PaymentProvider, attempt *pdomain.PaymentAttempt) error {
	now := time.Now()

	cancelErr := provider.Cancel(ctx, attempt.InternalRef)
	if cancelErr == nil {
		attempt.Status = pdomain.AttemptCancelled
		cancelledAt := time.Now()
		attempt.CancelledAt = &cancelledAt
		attempt.UpdatedAt = cancelledAt
		_ = h.repo.UpdateAttempt(ctx, attempt)
		return nil
	}

	verified, verifyErr := provider.Verify(ctx, attempt.InternalRef)
	if verifyErr == nil {
		switch verified.Status {
		case "SUCCESS":
			_ = h.processAttemptWebhook(ctx, attempt.Provider, attempt, verified, nil)
			return errs.B().Code(errs.FailedPrecondition).Msg("payment already completed; refresh payment status").Err()
		case "FAILED":
			attempt.Status = pdomain.AttemptFailed
			attempt.UpdatedAt = now
			_ = h.repo.UpdateAttempt(ctx, attempt)
			return nil
		}
	}

	attempt.Status = pdomain.AttemptSuperseded
	attempt.UpdatedAt = now
	_ = h.repo.UpdateAttempt(ctx, attempt)
	return nil
}
