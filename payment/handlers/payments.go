package handlers

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"encore.app/booking"
	bookinghandlers "encore.app/booking/handlers"
	eventscommon "encore.app/core/events"
	pdomain "encore.app/payment/domain"
	pevents "encore.app/payment/events"
	"encore.app/quote"
	"encore.dev/beta/errs"
)

const (
	defaultPaymentProvider      = "nomba"
	bookingStatusPaymentPending = "payment_pending"
	bookingStatusQuoteAccepted  = "quote_accepted"

	paymentReservationKeyMeta       = "payment_reservation_key"
	paymentReservationReservedUntil = "payment_reserved_until"
	paymentStatusEventEnqueuedAt    = "payment_status_event_enqueued_at"
	paymentStatusEventEnqueuedFor   = "payment_status_event_enqueued_for"
	maxWebhookBodyBytes             = 1 << 20
	paymentOutboxRepairBatchSize    = 100
	stuckPendingPaymentAge          = 30 * time.Minute
	webhookDiagnosticsWindow        = 24 * time.Hour
)

var paymentOutboxRepairGracePeriod = time.Minute

// PaymentsHandler orchestrates payment initialization and webhook processing.
type PaymentsHandler struct {
	repo                             pdomain.TransactionRepository
	providers                        map[string]pdomain.PaymentProvider
	getBooking                       func(context.Context, string) (*bookinghandlers.BookingResponse, error)
	getQuoteForPayment               func(context.Context, string) (*quote.PaymentQuoteResponse, error)
	markBookingPaymentPending        func(context.Context, string, *booking.MarkPaymentPendingRequest) (*booking.BookingPaymentStatusResponse, error)
	releaseBookingPaymentReservation func(context.Context, string, *booking.ReleasePaymentReservationRequest) (*booking.BookingPaymentStatusResponse, error)
	currentUserID                    func() (string, bool)
	config                           PaymentConfig
}

func NewPaymentsHandler(repo pdomain.TransactionRepository, providers map[string]pdomain.PaymentProvider, opts ...HandlerOption) *PaymentsHandler {
	h := &PaymentsHandler{
		repo:          repo,
		providers:     providers,
		currentUserID: defaultUserIDProvider,
		config:        normalizePaymentConfig(PaymentConfig{}),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// InitializePayment creates a fresh checkout session with the configured payment provider.
func (h *PaymentsHandler) InitializePayment(ctx context.Context, req *InitializePaymentRequest) (*InitializePaymentResponse, error) {
	req.Provider = strings.TrimSpace(strings.ToLower(req.Provider))
	if req.Provider == "" {
		req.Provider = defaultPaymentProvider
	}

	provider, exists := h.providers[req.Provider]
	if !exists {
		return nil, errs.B().Code(errs.InvalidArgument).Msgf("unsupported payment provider: %s", req.Provider).Err()
	}

	b, err := h.fetchBooking(ctx, req.BookingID)
	if err != nil {
		return nil, errs.B().Code(errs.NotFound).Msg("booking not found or unauthorized").Err()
	}

	userID, ok := h.currentUserID()
	if !ok {
		return nil, errs.B().Code(errs.Unauthenticated).Msg("authentication required").Err()
	}
	if b.CustomerID != userID {
		return nil, errs.B().Code(errs.PermissionDenied).Msg("only booking owner can initialize payment").Err()
	}

	q, err := h.fetchQuoteForPayment(ctx, req.QuoteID)
	if err != nil {
		return nil, errs.B().Code(errs.NotFound).Msg("quote not found").Err()
	}
	if q.BookingID != req.BookingID {
		return nil, errs.B().Code(errs.InvalidArgument).Msg("quote does not belong to booking").Err()
	}
	if q.State != "accepted" {
		return nil, errs.B().Code(errs.FailedPrecondition).Msg("quote must be accepted before payment").Err()
	}

	txn, err := h.repo.GetPendingByBookingAndQuote(ctx, req.BookingID, req.QuoteID)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to lookup pending transactions").Err()
	}
	reservationKey := ""
	if txn == nil {
		if string(b.Status) != bookingStatusQuoteAccepted {
			return nil, errs.B().Code(errs.FailedPrecondition).Msgf("booking must be quote_accepted before initializing payment; got %s", b.Status).Err()
		}

		reservationKey = eventscommon.GenerateUUID()
		txn = &pdomain.Transaction{
			CustomerID:  b.CustomerID,
			BookingID:   req.BookingID,
			QuoteID:     req.QuoteID,
			Amount:      float64(q.AmountCents) / 100.0,
			Currency:    q.Currency,
			Provider:    req.Provider,
			Method:      "checkout",
			Status:      pdomain.TxPending,
			InternalRef: h.generateInternalRef(req.BookingID),
			Metadata: map[string]string{
				paymentReservationKeyMeta: reservationKey,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := h.repo.Create(ctx, txn); err != nil {
			return nil, errs.B().Code(errs.Internal).Msg("failed to persist transaction").Err()
		}
	} else {
		if txn.Provider != req.Provider {
			return nil, errs.B().Code(errs.FailedPrecondition).Msgf("existing pending transaction uses provider %s", txn.Provider).Err()
		}
		if err := h.validateRetryBookingState(b.Status); err != nil {
			return nil, err
		}
	}
	if reservationKey == "" {
		var reservationKeyCreated bool
		reservationKey, reservationKeyCreated = h.ensurePaymentReservationKey(txn)
		if reservationKeyCreated {
			txn.UpdatedAt = time.Now()
			if err := h.repo.Update(ctx, txn); err != nil {
				return nil, errs.B().Code(errs.Internal).Msg("failed to persist payment reservation key").Err()
			}
		}
	}

	currentAttempt, err := h.resolveCurrentAttempt(ctx, txn, userID)
	if err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to resolve current payment attempt").Err()
	}

	if currentAttempt != nil && h.attemptRequiresCancellation(currentAttempt) {
		if err := h.cancelAttemptForRetry(ctx, provider, currentAttempt); err != nil {
			return nil, err
		}
	}

	reservationExpiresAt := time.Now().Add(h.config.ReturnContextTTL)
	if err := h.reserveBookingForPayment(ctx, b.Status, txn, userID, reservationKey, reservationExpiresAt); err != nil {
		return nil, err
	}

	attempt, err := h.createFreshAttempt(ctx, txn, currentAttempt, userID)
	if err != nil {
		h.releaseBookingReservationAfterFailure(ctx, txn, userID, reservationKey, "payment_attempt_creation_failed")
		return nil, errs.B().Code(errs.Internal).Msg("failed to create payment attempt").Err()
	}

	ctxToken, err := encodeReturnContext(returnContext{
		TransactionID: txn.ID,
		AttemptID:     attempt.ID,
		InternalRef:   attempt.InternalRef,
		BookingID:     txn.BookingID,
		QuoteID:       txn.QuoteID,
		CustomerID:    txn.CustomerID,
		Provider:      txn.Provider,
		ReturnType:    "complete",
		ExpiresAt:     reservationExpiresAt.Unix(),
	}, h.config.ReturnContextSecret)
	if err != nil {
		h.releaseBookingReservationAfterFailure(ctx, txn, userID, reservationKey, "return_context_signing_failed")
		return nil, errs.B().Code(errs.Internal).Msg("failed to sign return context").Err()
	}

	returnURL := buildCheckoutReturnURL(h.config.PublicBaseURL, ctxToken)
	initRes, err := provider.Initialize(ctx, &pdomain.InitializationRequest{
		OrderReference:        attempt.InternalRef,
		Amount:                txn.Amount,
		Currency:              txn.Currency,
		CustomerEmail:         fmt.Sprintf("customer-%s@protisan.com", txn.CustomerID),
		CallbackURL:           returnURL,
		AccountID:             "default",
		AllowedPaymentMethods: req.AllowedPaymentMethods,
	})
	if err != nil {
		h.markAttemptInitializationFailed(ctx, txn, attempt, err)
		h.releaseBookingReservationAfterFailure(ctx, txn, userID, reservationKey, "provider_initialization_failed")
		return nil, errs.B().Code(errs.Internal).Msg("provider initialization failed").Err()
	}

	if err := h.activateAttempt(ctx, txn, attempt, initRes, returnURL, ctxToken, reservationExpiresAt); err != nil {
		return nil, errs.B().Code(errs.Internal).Msg("failed to update initialized payment attempt").Err()
	}

	return h.buildInitializeResponse(txn, attempt)
}

// GetPayment returns backend payment state for app-side verification.
func (h *PaymentsHandler) GetPayment(ctx context.Context, transactionID string) (*PaymentStatusResponse, error) {
	txn, err := h.repo.GetByID(ctx, transactionID)
	if err != nil {
		return nil, errs.B().Code(errs.NotFound).Msg("transaction not found").Err()
	}

	userID, ok := h.currentUserID()
	if !ok {
		return nil, errs.B().Code(errs.Unauthenticated).Msg("authentication required").Err()
	}
	if txn.CustomerID != userID {
		return nil, errs.B().Code(errs.PermissionDenied).Msg("not authorized to view this transaction").Err()
	}

	b, err := h.fetchBooking(ctx, txn.BookingID)
	if err != nil {
		return nil, errs.B().Code(errs.NotFound).Msg("booking not found or unauthorized").Err()
	}

	reference := txn.InternalRef
	if attempt, err := h.repo.GetCurrentAttemptByTransactionID(ctx, txn.ID); err == nil && attempt != nil {
		reference = attempt.InternalRef
	}

	return &PaymentStatusResponse{
		TransactionID: txn.ID,
		BookingID:     txn.BookingID,
		QuoteID:       txn.QuoteID,
		Reference:     reference,
		Provider:      txn.Provider,
		Status:        string(txn.Status),
		BookingStatus: string(b.Status),
		Amount:        txn.Amount,
		Currency:      txn.Currency,
		UpdatedAt:     txn.UpdatedAt,
	}, nil
}

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
		switch verified.Status {
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
		default:
			return nil
		}
	}); err != nil {
		return err
	}

	if finalizedTxn != nil {
		h.markPaymentStatusEventEnqueued(ctx, finalizedTxn, finalizedStatus)
	}

	return nil
}

func (h *PaymentsHandler) buildInitializeResponse(txn *pdomain.Transaction, attempt *pdomain.PaymentAttempt) (*InitializePaymentResponse, error) {
	if attempt == nil || attempt.CheckoutURL == "" {
		return nil, errs.B().Code(errs.Internal).Msg("payment attempt is missing checkout url").Err()
	}

	return &InitializePaymentResponse{
		TransactionID: txn.ID,
		BookingID:     txn.BookingID,
		QuoteID:       txn.QuoteID,
		CheckoutURL:   attempt.CheckoutURL,
		ReturnURL:     attempt.ReturnURL,
		Reference:     attempt.InternalRef,
		Status:        string(txn.Status),
	}, nil
}

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
			Amount:         txn.Amount,
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
			Amount:         txn.Amount,
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

func (h *PaymentsHandler) reserveBookingForPayment(ctx context.Context, status any, txn *pdomain.Transaction, userID, reservationKey string, reservedUntil time.Time) error {
	statusValue := fmt.Sprint(status)
	if statusValue != bookingStatusQuoteAccepted && statusValue != bookingStatusPaymentPending {
		err := errs.B().Code(errs.FailedPrecondition).Msgf("booking is in incompatible state %s for pending transaction", statusValue).Err()
		h.markBookingRepairRequired(ctx, txn, err)
		return err
	}

	if _, err := h.moveBookingToPaymentPending(ctx, txn.BookingID, &booking.MarkPaymentPendingRequest{
		UserID:         userID,
		QuoteID:        txn.QuoteID,
		ReservationKey: reservationKey,
		ReservedUntil:  reservedUntil,
	}); err != nil {
		h.markBookingRepairRequired(ctx, txn, err)
		return errs.B().Code(errs.FailedPrecondition).Msg("payment initialized but booking state repair is required; retry initialize to repair").Err()
	}

	h.clearBookingRepairMetadata(ctx, txn)
	return nil
}

func (h *PaymentsHandler) releaseBookingReservationAfterFailure(ctx context.Context, txn *pdomain.Transaction, userID, reservationKey, reasonText string) {
	reason := reasonText
	if _, err := h.releaseBookingReservation(ctx, txn.BookingID, &booking.ReleasePaymentReservationRequest{
		UserID:         userID,
		QuoteID:        txn.QuoteID,
		ReservationKey: reservationKey,
		Reason:         &reason,
	}); err != nil {
		h.markBookingRepairRequired(ctx, txn, err)
	}
}

func (h *PaymentsHandler) validateRetryBookingState(status any) error {
	statusValue := fmt.Sprint(status)
	switch statusValue {
	case bookingStatusPaymentPending, bookingStatusQuoteAccepted:
		return nil
	default:
		return errs.B().Code(errs.FailedPrecondition).Msgf("existing pending transaction cannot be retried while booking is %s", statusValue).Err()
	}
}

func (h *PaymentsHandler) ensurePaymentReservationKey(txn *pdomain.Transaction) (string, bool) {
	if txn.Metadata == nil {
		txn.Metadata = map[string]string{}
	}
	if key := txn.Metadata[paymentReservationKeyMeta]; key != "" {
		return key, false
	}
	key := eventscommon.GenerateUUID()
	txn.Metadata[paymentReservationKeyMeta] = key
	return key, true
}

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

// cancelAttemptForRetry tries to cleanly cancel the previous provider-side checkout.
// If the provider confirms the old attempt already succeeded, it reconciles and blocks the retry.
// For all other cases (cancel fails, verify is ambiguous, provider unreachable), the old attempt
// is superseded locally and the retry is allowed. This is safe because:
//   - Nomba checkout links expire naturally on their side
//   - A late success for a superseded attempt is still verified and reconciled
//   - A late failure for a superseded attempt won't fail the parent transaction
func (h *PaymentsHandler) cancelAttemptForRetry(ctx context.Context, provider pdomain.PaymentProvider, attempt *pdomain.PaymentAttempt) error {
	now := time.Now()

	// Step 1: Best-effort provider-side cancellation.
	cancelErr := provider.Cancel(ctx, attempt.InternalRef)
	if cancelErr == nil {
		attempt.Status = pdomain.AttemptCancelled
		cancelledAt := time.Now()
		attempt.CancelledAt = &cancelledAt
		attempt.UpdatedAt = cancelledAt
		_ = h.repo.UpdateAttempt(ctx, attempt)
		return nil
	}

	// Step 2: Cancel failed. Check if payment actually completed on Nomba's side.
	verified, verifyErr := provider.Verify(ctx, attempt.InternalRef)
	if verifyErr == nil {
		switch verified.Status {
		case "SUCCESS":
			// The old attempt actually completed — reconcile and block retry.
			_ = h.processAttemptWebhook(ctx, attempt.Provider, attempt, verified, nil)
			return errs.B().Code(errs.FailedPrecondition).Msg("payment already completed; refresh payment status").Err()
		case "FAILED":
			attempt.Status = pdomain.AttemptFailed
			attempt.UpdatedAt = now
			_ = h.repo.UpdateAttempt(ctx, attempt)
			return nil
		}
	}

	// Step 3: Both cancel and verify are inconclusive. Supersede the old attempt
	// locally and allow the retry. The old checkout link is now orphaned — if the
	// user somehow completes it, the webhook verifier will still require the
	// original amount/currency before reconciling the parent transaction.
	attempt.Status = pdomain.AttemptSuperseded
	attempt.UpdatedAt = now
	_ = h.repo.UpdateAttempt(ctx, attempt)
	return nil
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

func (h *PaymentsHandler) generateInternalRef(bookingID string) string {
	return fmt.Sprintf("TXN-%s-%d", bookingID, time.Now().UnixNano())
}

func (h *PaymentsHandler) validateVerifiedPayment(txn *pdomain.Transaction, attempt *pdomain.PaymentAttempt, verified *pdomain.VerificationResponse) error {
	if verified == nil {
		return fmt.Errorf("missing provider verification response")
	}
	if verified.OrderRef != "" && attempt != nil && verified.OrderRef != attempt.InternalRef {
		return fmt.Errorf("provider verification reference mismatch")
	}
	if verified.Amount <= 0 {
		return fmt.Errorf("provider verification amount missing")
	}
	if math.Abs(txn.Amount-verified.Amount) >= 0.005 {
		return fmt.Errorf("provider verification amount mismatch")
	}
	if verified.Currency != "" && !strings.EqualFold(txn.Currency, verified.Currency) {
		return fmt.Errorf("provider verification currency mismatch")
	}
	return nil
}

func sanitizeProviderError(err error) string {
	if err == nil {
		return ""
	}
	return "provider_initialization_failed"
}

func redactPaymentRef(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "***"
	}
	return value[:4] + "..." + value[len(value)-4:]
}

func (h *PaymentsHandler) markBookingRepairRequired(ctx context.Context, txn *pdomain.Transaction, repairErr error) {
	if txn.Metadata == nil {
		txn.Metadata = map[string]string{}
	}
	txn.Metadata["booking_state_repair_required"] = "true"
	txn.Metadata["booking_state_repair_error"] = repairErr.Error()
	txn.Metadata["booking_state_repair_policy"] = "cancel_existing_checkout_and_retry_initialize"
	txn.UpdatedAt = time.Now()
	_ = h.repo.Update(ctx, txn)
}

func (h *PaymentsHandler) clearBookingRepairMetadata(ctx context.Context, txn *pdomain.Transaction) {
	if txn.Metadata == nil {
		return
	}

	if _, exists := txn.Metadata["booking_state_repair_required"]; !exists {
		return
	}

	delete(txn.Metadata, "booking_state_repair_required")
	delete(txn.Metadata, "booking_state_repair_error")
	delete(txn.Metadata, "booking_state_repair_policy")
	txn.UpdatedAt = time.Now()
	_ = h.repo.Update(ctx, txn)
}

func (h *PaymentsHandler) fetchBooking(ctx context.Context, bookingID string) (*bookinghandlers.BookingResponse, error) {
	if h.getBooking != nil {
		return h.getBooking(ctx, bookingID)
	}
	return booking.GetBooking(ctx, bookingID)
}

func (h *PaymentsHandler) fetchQuoteForPayment(ctx context.Context, quoteID string) (*quote.PaymentQuoteResponse, error) {
	if h.getQuoteForPayment != nil {
		return h.getQuoteForPayment(ctx, quoteID)
	}
	return quote.GetQuoteForPayment(ctx, quoteID)
}

func (h *PaymentsHandler) moveBookingToPaymentPending(ctx context.Context, bookingID string, req *booking.MarkPaymentPendingRequest) (*booking.BookingPaymentStatusResponse, error) {
	if h.markBookingPaymentPending != nil {
		return h.markBookingPaymentPending(ctx, bookingID, req)
	}
	return booking.MarkPaymentPending(ctx, bookingID, req)
}

func (h *PaymentsHandler) releaseBookingReservation(ctx context.Context, bookingID string, req *booking.ReleasePaymentReservationRequest) (*booking.BookingPaymentStatusResponse, error) {
	if h.releaseBookingPaymentReservation != nil {
		return h.releaseBookingPaymentReservation(ctx, bookingID, req)
	}
	return booking.ReleasePaymentReservation(ctx, bookingID, req)
}
