package handlers

import (
	"context"
	"log"
	"time"

	pdomain "encore.app/payment/domain"
	pevents "encore.app/payment/events"
)

func (h *PaymentsHandler) createBookingStatusEvent(ctx context.Context, txRepo pdomain.TransactionRepository, txn *pdomain.Transaction, status pdomain.TransactionStatus) error {
	now := time.Now()
	reservationKey := ""
	if txn.Metadata != nil {
		reservationKey = txn.Metadata[paymentReservationKeyMeta]
	}
	switch status {
	case pdomain.TxSuccessful:
		event := pevents.CreateEventEnvelope(ctx, "payment.v1.confirmed", pdomain.PaymentEvent{
			TransactionID:  txn.ID,
			BookingID:      txn.BookingID,
			QuoteID:        txn.QuoteID,
			CustomerID:     txn.CustomerID,
			ReservationKey: reservationKey,
			Status:         string(pdomain.TxSuccessful),
			PreviousStatus: string(pdomain.TxPending),
			Provider:       txn.Provider,
			AmountCents:    txn.AmountCents,
			Amount:         amountCentsToMajorUnits(txn.AmountCents),
			Currency:       txn.Currency,
			InternalRef:    txn.InternalRef,
			ProviderRef:    txn.ProviderRef,
			Timestamp:      now,
		})
		return txRepo.CreatePaymentEventInOutbox(ctx, "payment-v1-confirmed", event)
	case pdomain.TxFailed:
		reason := "payment_failed"
		event := pevents.CreateEventEnvelope(ctx, "payment.v1.failed", pdomain.PaymentEvent{
			TransactionID:  txn.ID,
			BookingID:      txn.BookingID,
			QuoteID:        txn.QuoteID,
			CustomerID:     txn.CustomerID,
			ReservationKey: reservationKey,
			Status:         string(pdomain.TxFailed),
			PreviousStatus: string(pdomain.TxPending),
			Provider:       txn.Provider,
			AmountCents:    txn.AmountCents,
			Amount:         amountCentsToMajorUnits(txn.AmountCents),
			Currency:       txn.Currency,
			InternalRef:    txn.InternalRef,
			ProviderRef:    txn.ProviderRef,
			Reason:         &reason,
			Timestamp:      now,
		})
		return txRepo.CreatePaymentEventInOutbox(ctx, "payment-v1-failed", event)
	default:
		return nil
	}
}

func (h *PaymentsHandler) RepairPaymentOutboxEvents(ctx context.Context) (int, error) {
	cutoff := time.Now().Add(-paymentOutboxRepairGracePeriod)
	txns, err := h.repo.ListTransactionsMissingPaymentEvent(ctx, cutoff, paymentOutboxRepairBatchSize)
	if err != nil {
		return 0, err
	}

	repaired := 0
	for _, candidate := range txns {
		var finalizedTxn *pdomain.Transaction
		if err := h.repo.WithTransaction(ctx, func(txRepo pdomain.TransactionRepository) error {
			txn, err := txRepo.GetByID(ctx, candidate.ID)
			if err != nil {
				return err
			}
			if !requiresPaymentStatusEvent(txn.Status) || paymentStatusEventMarked(txn, txn.Status) {
				return nil
			}
			if err := h.createBookingStatusEvent(ctx, txRepo, txn, txn.Status); err != nil {
				return err
			}
			finalizedTxn = txn
			return nil
		}); err != nil {
			return repaired, err
		}
		if finalizedTxn == nil {
			continue
		}
		h.markPaymentStatusEventEnqueued(ctx, finalizedTxn, finalizedTxn.Status)
		repaired++
	}
	log.Printf("payment outbox repair completed: candidates=%d repaired=%d", len(txns), repaired)
	return repaired, nil
}

func (h *PaymentsHandler) CleanupWebhookEvents(ctx context.Context, cutoff time.Time) error {
	return h.repo.DeleteWebhookEventsBefore(ctx, cutoff)
}

func (h *PaymentsHandler) Diagnostics(ctx context.Context) (*PaymentDiagnosticsResponse, error) {
	now := time.Now()
	stuckPendingCutoff := now.Add(-stuckPendingPaymentAge)
	missingOutboxCutoff := now.Add(-paymentOutboxRepairGracePeriod)
	webhookSince := now.Add(-webhookDiagnosticsWindow)

	stuckPending, err := h.repo.CountPendingTransactionsOlderThan(ctx, stuckPendingCutoff)
	if err != nil {
		return nil, err
	}
	missingOutbox, err := h.repo.CountTransactionsMissingPaymentEvent(ctx, missingOutboxCutoff)
	if err != nil {
		return nil, err
	}
	recentWebhooks, err := h.repo.CountWebhookEventsSince(ctx, webhookSince)
	if err != nil {
		return nil, err
	}

	log.Printf("payment diagnostics checked: stuck_pending=%d missing_outbox_marker=%d recent_webhooks=%d", stuckPending, missingOutbox, recentWebhooks)
	return &PaymentDiagnosticsResponse{
		StuckPendingTransactions:       stuckPending,
		CompletedMissingOutboxMarker:   missingOutbox,
		RecentWebhookEvents:            recentWebhooks,
		StuckPendingOlderThanMinutes:   int(stuckPendingPaymentAge / time.Minute),
		WebhookEventsWindowMinutes:     int(webhookDiagnosticsWindow / time.Minute),
		MissingOutboxGracePeriodMinute: int(paymentOutboxRepairGracePeriod / time.Minute),
		CheckedAt:                      now,
	}, nil
}

func (h *PaymentsHandler) markPaymentStatusEventEnqueued(ctx context.Context, txn *pdomain.Transaction, status pdomain.TransactionStatus) {
	if txn.Metadata == nil {
		txn.Metadata = map[string]string{}
	}
	txn.Metadata[paymentStatusEventEnqueuedFor] = string(status)
	txn.Metadata[paymentStatusEventEnqueuedAt] = time.Now().UTC().Format(time.RFC3339)
	txn.UpdatedAt = time.Now()
	if err := h.repo.Update(ctx, txn); err != nil {
		log.Printf("payment outbox marker update failed: transaction=%s status=%s error=%v", redactPaymentRef(txn.ID), status, err)
	}
}

func requiresPaymentStatusEvent(status pdomain.TransactionStatus) bool {
	return status == pdomain.TxSuccessful || status == pdomain.TxFailed
}

func paymentStatusEventMarked(txn *pdomain.Transaction, status pdomain.TransactionStatus) bool {
	return txn.Metadata != nil && txn.Metadata[paymentStatusEventEnqueuedFor] == string(status)
}
