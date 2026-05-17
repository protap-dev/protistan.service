package handlers

import (
	"context"
	"strings"
	"time"

	"encore.app/booking"
	bookinghandlers "encore.app/booking/handlers"
	eventscommon "encore.app/core/events"
	pdomain "encore.app/payment/domain"
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
	getCustomerEmail                 func(context.Context, string) (string, error)
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
			AmountCents: q.AmountCents,
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
	customerEmail, err := h.fetchCustomerEmail(ctx, txn.CustomerID)
	if err != nil {
		h.releaseBookingReservationAfterFailure(ctx, txn, userID, reservationKey, "customer_email_lookup_failed")
		return nil, errs.B().Code(errs.Internal).Msg("failed to resolve customer email").Err()
	}
	initRes, err := provider.Initialize(ctx, &pdomain.InitializationRequest{
		OrderReference:        attempt.InternalRef,
		AmountCents:           txn.AmountCents,
		Currency:              txn.Currency,
		CustomerEmail:         customerEmail,
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
		AmountCents:   txn.AmountCents,
		Amount:        amountCentsToMajorUnits(txn.AmountCents),
		Currency:      txn.Currency,
		UpdatedAt:     txn.UpdatedAt,
	}, nil
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
