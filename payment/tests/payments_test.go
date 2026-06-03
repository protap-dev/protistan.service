package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"encore.app/booking"
	bookinghandlers "encore.app/booking/handlers"
	"encore.app/payment/domain"
	"encore.app/payment/handlers"
	"encore.app/quote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	bdomain "encore.app/booking/domain"
	eventscommon "encore.app/core/events"
)

func strPtr(s string) *string { return &s }

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

type mockRepository struct {
	mock.Mock
}

func (m *mockRepository) hasExpectation(method string) bool {
	for _, call := range m.ExpectedCalls {
		if call.Method == method {
			return true
		}
	}
	return false
}

func (m *mockRepository) Create(ctx context.Context, txn *domain.Transaction) error {
	txn.ID = "test-txn-id"
	args := m.Called(ctx, txn)
	return args.Error(0)
}

func (m *mockRepository) Update(ctx context.Context, txn *domain.Transaction) error {
	args := m.Called(ctx, txn)
	return args.Error(0)
}

func (m *mockRepository) CreateAttempt(ctx context.Context, attempt *domain.PaymentAttempt) error {
	attempt.ID = "test-attempt-id"
	args := m.Called(ctx, attempt)
	return args.Error(0)
}

func (m *mockRepository) UpdateAttempt(ctx context.Context, attempt *domain.PaymentAttempt) error {
	args := m.Called(ctx, attempt)
	return args.Error(0)
}

func (m *mockRepository) GetByID(ctx context.Context, id string) (*domain.Transaction, error) {
	args := m.Called(ctx, id)
	if txn := args.Get(0); txn != nil {
		return txn.(*domain.Transaction), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockRepository) GetByInternalRef(ctx context.Context, ref string) (*domain.Transaction, error) {
	args := m.Called(ctx, ref)
	if txn := args.Get(0); txn != nil {
		return txn.(*domain.Transaction), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockRepository) GetPendingByBookingAndQuote(ctx context.Context, bookingID, quoteID string) (*domain.Transaction, error) {
	args := m.Called(ctx, bookingID, quoteID)
	if txn := args.Get(0); txn != nil {
		return txn.(*domain.Transaction), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockRepository) GetAttemptByInternalRef(ctx context.Context, ref string) (*domain.PaymentAttempt, error) {
	args := m.Called(ctx, ref)
	if attempt := args.Get(0); attempt != nil {
		return attempt.(*domain.PaymentAttempt), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockRepository) GetCurrentAttemptByTransactionID(ctx context.Context, transactionID string) (*domain.PaymentAttempt, error) {
	args := m.Called(ctx, transactionID)
	if attempt := args.Get(0); attempt != nil {
		return attempt.(*domain.PaymentAttempt), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockRepository) CountAttemptsByTransactionID(ctx context.Context, transactionID string) (int64, error) {
	args := m.Called(ctx, transactionID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockRepository) HasWebhookEvent(ctx context.Context, provider, requestID string) (bool, error) {
	if !m.hasExpectation("HasWebhookEvent") {
		return false, nil
	}
	args := m.Called(ctx, provider, requestID)
	return args.Bool(0), args.Error(1)
}

func (m *mockRepository) RecordWebhookEvent(ctx context.Context, provider, requestID string, receivedAt time.Time) (bool, error) {
	if !m.hasExpectation("RecordWebhookEvent") {
		return true, nil
	}
	args := m.Called(ctx, provider, requestID, receivedAt)
	return args.Bool(0), args.Error(1)
}

func (m *mockRepository) DeleteWebhookEventsBefore(ctx context.Context, cutoff time.Time) error {
	args := m.Called(ctx, cutoff)
	return args.Error(0)
}

func (m *mockRepository) ListTransactionsMissingPaymentEvent(ctx context.Context, cutoff time.Time, limit int) ([]*domain.Transaction, error) {
	args := m.Called(ctx, cutoff, limit)
	if txns := args.Get(0); txns != nil {
		return txns.([]*domain.Transaction), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockRepository) CountPendingTransactionsOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	args := m.Called(ctx, cutoff)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockRepository) CountTransactionsMissingPaymentEvent(ctx context.Context, cutoff time.Time) (int64, error) {
	args := m.Called(ctx, cutoff)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockRepository) CountWebhookEventsSince(ctx context.Context, since time.Time) (int64, error) {
	args := m.Called(ctx, since)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockRepository) WithTransaction(ctx context.Context, fn func(domain.TransactionRepository) error) error {
	if m.hasExpectation("WithTransaction") {
		if args := m.Called(ctx); args.Error(0) != nil {
			return args.Error(0)
		}
	}
	return fn(m)
}

func (m *mockRepository) CreatePaymentEventInOutbox(ctx context.Context, topic string, event *eventscommon.EventEnvelope[eventscommon.PaymentEvent]) error {
	args := m.Called(ctx, topic, event)
	return args.Error(0)
}

type mockProvider struct {
	mock.Mock
}

func (m *mockProvider) Identifier() string { return "mock_nomba" }

func (m *mockProvider) Initialize(ctx context.Context, req *domain.InitializationRequest) (*domain.InitializationResponse, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(*domain.InitializationResponse), args.Error(1)
}

func (m *mockProvider) Verify(ctx context.Context, reference string) (*domain.VerificationResponse, error) {
	args := m.Called(ctx, reference)
	return args.Get(0).(*domain.VerificationResponse), args.Error(1)
}

func (m *mockProvider) Cancel(ctx context.Context, reference string) error {
	args := m.Called(ctx, reference)
	return args.Error(0)
}

func (m *mockProvider) ParseWebhook(ctx context.Context, req *http.Request) (*domain.WebhookEvent, error) {
	args := m.Called(ctx, req)
	return args.Get(0).(*domain.WebhookEvent), args.Error(1)
}

func (m *mockProvider) Reconcile(ctx context.Context, accountID, cursor string) (string, error) {
	args := m.Called(ctx, accountID, cursor)
	return args.String(0), args.Error(1)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestWebhookSignatureValidation(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)
	h := handlers.NewPaymentsHandler(repo, map[string]domain.PaymentProvider{"nomba": provider})

	payload := []byte(`{"event_type": "SUCCESS"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/payments/webhook/nomba", bytes.NewReader(payload))
	req.Header.Set("nomba-signature", "invalid-sig")

	w := httptest.NewRecorder()

	provider.On("ParseWebhook", mock.Anything, req).
		Return(&domain.WebhookEvent{}, assert.AnError)

	h.NombaWebhook(w, req)

	res := w.Result()
	assert.Equal(t, http.StatusUnauthorized, res.StatusCode)
	repo.AssertNotCalled(t, "GetByInternalRef")
}

func TestWebhookSuccessFlow(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)
	h := handlers.NewPaymentsHandler(repo, map[string]domain.PaymentProvider{"nomba": provider})

	payload := []byte(`{"event_type": "SUCCESS"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/payments/webhook/nomba", bytes.NewReader(payload))
	req.Header.Set("nomba-signature", "valid-fake-sig")

	w := httptest.NewRecorder()

	provider.On("ParseWebhook", mock.Anything, req).Return(&domain.WebhookEvent{
		EventType: "SUCCESS",
		OrderRef:  "TXN-1234",
		RequestID: "req-1234",
	}, nil)

	repo.On("GetAttemptByInternalRef", mock.Anything, "TXN-1234").Return(&domain.PaymentAttempt{
		ID:            "attempt-123",
		TransactionID: "db-id-123",
		Provider:      "nomba",
		Status:        domain.AttemptActive,
		InternalRef:   "TXN-1234",
	}, nil)

	provider.On("Verify", mock.Anything, "TXN-1234").Return(&domain.VerificationResponse{
		Status:        "SUCCESS",
		TransactionID: "PAY-123",
		AmountCents:   100000,
	}, nil)

	repo.On("WithTransaction", mock.Anything).Return(nil)
	repo.On("GetByID", mock.Anything, "db-id-123").Return(&domain.Transaction{
		ID:               "db-id-123",
		BookingID:        "book-123",
		CustomerID:       "customer-123",
		AmountCents:      100000,
		Currency:         "NGN",
		Status:           domain.TxPending,
		InternalRef:      "TXN-1234",
		CurrentAttemptID: strPtr("attempt-123"),
	}, nil)
	repo.On("UpdateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.Status == domain.AttemptSuccessful && attempt.ProviderRef == "PAY-123"
	})).Return(nil)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.Status == domain.TxSuccessful &&
			txn.CurrentAttemptID != nil && *txn.CurrentAttemptID == "attempt-123" &&
			txn.ProviderRef == "PAY-123"
	})).Return(nil)
	repo.On("CreatePaymentEventInOutbox", mock.Anything, "payment-v1-confirmed", mock.Anything).Return(nil)

	h.NombaWebhook(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	provider.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestProviderWebhookUsesRouteProviderForLookupAndDedupe(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)
	h := handlers.NewPaymentsHandler(repo, map[string]domain.PaymentProvider{"stripe": provider})
	receivedAt := time.Now().UTC().Truncate(time.Second)

	req := httptest.NewRequest(http.MethodPost, "/v1/payments/webhook/stripe", bytes.NewReader([]byte(`{"type":"checkout.session.completed"}`)))
	w := httptest.NewRecorder()

	provider.On("ParseWebhook", mock.Anything, req).Return(&domain.WebhookEvent{
		EventType:  "checkout.session.completed",
		OrderRef:   "pi-stripe-1",
		RequestID:  "evt-stripe-1",
		ReceivedAt: receivedAt,
	}, nil)
	repo.On("HasWebhookEvent", mock.Anything, "stripe", "evt-stripe-1").Return(false, nil)
	repo.On("GetAttemptByInternalRef", mock.Anything, "pi-stripe-1").Return(&domain.PaymentAttempt{
		ID:            "attempt-stripe",
		TransactionID: "txn-stripe",
		Provider:      "stripe",
		Status:        domain.AttemptActive,
		InternalRef:   "pi-stripe-1",
	}, nil)
	provider.On("Verify", mock.Anything, "pi-stripe-1").Return(&domain.VerificationResponse{
		Status:        "SUCCESS",
		TransactionID: "charge-stripe-1",
		AmountCents:   100000,
		Currency:      "NGN",
	}, nil)
	repo.On("WithTransaction", mock.Anything).Return(nil)
	repo.On("RecordWebhookEvent", mock.Anything, "stripe", "evt-stripe-1", receivedAt).Return(true, nil)
	repo.On("GetByID", mock.Anything, "txn-stripe").Return(&domain.Transaction{
		ID:               "txn-stripe",
		BookingID:        "booking-stripe",
		QuoteID:          "quote-stripe",
		CustomerID:       "customer-stripe",
		Provider:         "stripe",
		AmountCents:      100000,
		Currency:         "NGN",
		Status:           domain.TxPending,
		CurrentAttemptID: strPtr("attempt-stripe"),
	}, nil)
	repo.On("UpdateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.ID == "attempt-stripe" && attempt.ProviderRef == "charge-stripe-1"
	})).Return(nil)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.ID == "txn-stripe" && txn.Provider == "stripe" && txn.Status == domain.TxSuccessful
	})).Return(nil)
	repo.On("CreatePaymentEventInOutbox", mock.Anything, "payment-v1-confirmed", mock.Anything).Return(nil)

	h.ProviderWebhook("stripe", w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	provider.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestWebhookSuccessForNonCurrentAttemptStillConfirmsTransaction(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)
	h := handlers.NewPaymentsHandler(repo, map[string]domain.PaymentProvider{"nomba": provider})

	req := httptest.NewRequest(http.MethodPost, "/v1/payments/webhook/nomba", bytes.NewReader([]byte(`{"event_type":"SUCCESS"}`)))
	req.Header.Set("nomba-signature", "valid-fake-sig")
	w := httptest.NewRecorder()

	provider.On("ParseWebhook", mock.Anything, req).Return(&domain.WebhookEvent{
		EventType: "SUCCESS",
		OrderRef:  "TXN-old",
		RequestID: "req-old",
	}, nil)
	provider.On("Verify", mock.Anything, "TXN-old").Return(&domain.VerificationResponse{
		Status:        "SUCCESS",
		TransactionID: "PAY-old",
		AmountCents:   100000,
	}, nil)
	repo.On("GetAttemptByInternalRef", mock.Anything, "TXN-old").Return(&domain.PaymentAttempt{
		ID:            "attempt-old",
		TransactionID: "txn-1",
		Provider:      "nomba",
		Status:        domain.AttemptSuperseded,
		InternalRef:   "TXN-old",
	}, nil)
	repo.On("WithTransaction", mock.Anything).Return(nil)
	repo.On("GetByID", mock.Anything, "txn-1").Return(&domain.Transaction{
		ID:               "txn-1",
		BookingID:        "booking-1",
		QuoteID:          "quote-1",
		CustomerID:       "customer-1",
		AmountCents:      100000,
		Currency:         "NGN",
		Status:           domain.TxPending,
		InternalRef:      "TXN-new",
		CurrentAttemptID: strPtr("attempt-new"),
		Metadata: map[string]string{
			"payment_reservation_key": "reservation-1",
		},
	}, nil)
	repo.On("UpdateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.ID == "attempt-old" && attempt.Status == domain.AttemptSuccessful && attempt.ProviderRef == "PAY-old"
	})).Return(nil)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.Status == domain.TxSuccessful &&
			txn.CurrentAttemptID != nil && *txn.CurrentAttemptID == "attempt-old" &&
			txn.Metadata["successful_non_current_attempt_id"] == "attempt-old" &&
			txn.Metadata["previous_current_attempt_id"] == "attempt-new"
	})).Return(nil)
	repo.On("CreatePaymentEventInOutbox", mock.Anything, "payment-v1-confirmed", mock.Anything).Return(nil)

	h.NombaWebhook(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	provider.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestWebhookSuccessRejectsProviderAmountMismatch(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)
	h := handlers.NewPaymentsHandler(repo, map[string]domain.PaymentProvider{"nomba": provider})

	req := httptest.NewRequest(http.MethodPost, "/v1/payments/webhook/nomba", bytes.NewReader([]byte(`{"event_type":"SUCCESS"}`)))
	req.Header.Set("nomba-signature", "valid-fake-sig")
	w := httptest.NewRecorder()

	provider.On("ParseWebhook", mock.Anything, req).Return(&domain.WebhookEvent{
		EventType: "SUCCESS",
		OrderRef:  "TXN-amount",
		RequestID: "req-amount",
	}, nil)
	provider.On("Verify", mock.Anything, "TXN-amount").Return(&domain.VerificationResponse{
		Status:        "SUCCESS",
		TransactionID: "PAY-amount",
		AmountCents:   99900,
	}, nil)
	repo.On("GetAttemptByInternalRef", mock.Anything, "TXN-amount").Return(&domain.PaymentAttempt{
		ID:            "attempt-amount",
		TransactionID: "txn-amount",
		Provider:      "nomba",
		Status:        domain.AttemptActive,
		InternalRef:   "TXN-amount",
	}, nil)
	repo.On("WithTransaction", mock.Anything).Return(nil)
	repo.On("GetByID", mock.Anything, "txn-amount").Return(&domain.Transaction{
		ID:               "txn-amount",
		AmountCents:      100000,
		Currency:         "NGN",
		Status:           domain.TxPending,
		CurrentAttemptID: strPtr("attempt-amount"),
	}, nil)

	h.NombaWebhook(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	repo.AssertNotCalled(t, "UpdateAttempt")
	repo.AssertNotCalled(t, "Update")
	repo.AssertNotCalled(t, "CreatePaymentEventInOutbox")
	provider.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestWebhookWithoutAttemptDoesNotFallbackToTransactionUpdate(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)
	h := handlers.NewPaymentsHandler(repo, map[string]domain.PaymentProvider{"nomba": provider})

	req := httptest.NewRequest(http.MethodPost, "/v1/payments/webhook/nomba", bytes.NewReader([]byte(`{"event_type":"SUCCESS"}`)))
	req.Header.Set("nomba-signature", "valid-fake-sig")
	w := httptest.NewRecorder()

	provider.On("ParseWebhook", mock.Anything, req).Return(&domain.WebhookEvent{
		EventType: "SUCCESS",
		OrderRef:  "TXN-no-attempt",
		RequestID: "req-no-attempt",
	}, nil)
	repo.On("GetAttemptByInternalRef", mock.Anything, "TXN-no-attempt").Return((*domain.PaymentAttempt)(nil), nil)

	h.NombaWebhook(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	provider.AssertNotCalled(t, "Verify")
	repo.AssertNotCalled(t, "GetByInternalRef")
	repo.AssertNotCalled(t, "Update")
	repo.AssertNotCalled(t, "CreatePaymentEventInOutbox")
	provider.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestWebhookAttemptLookupErrorDoesNotCallProviderVerification(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)
	h := handlers.NewPaymentsHandler(repo, map[string]domain.PaymentProvider{"nomba": provider})

	req := httptest.NewRequest(http.MethodPost, "/v1/payments/webhook/nomba", bytes.NewReader([]byte(`{"event_type":"SUCCESS"}`)))
	req.Header.Set("nomba-signature", "valid-fake-sig")
	w := httptest.NewRecorder()

	provider.On("ParseWebhook", mock.Anything, req).Return(&domain.WebhookEvent{
		EventType: "SUCCESS",
		OrderRef:  "TXN-lookup-error",
		RequestID: "req-lookup-error",
	}, nil)
	repo.On("GetAttemptByInternalRef", mock.Anything, "TXN-lookup-error").Return((*domain.PaymentAttempt)(nil), assert.AnError)

	h.NombaWebhook(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	provider.AssertNotCalled(t, "Verify")
	repo.AssertNotCalled(t, "Update")
	repo.AssertNotCalled(t, "CreatePaymentEventInOutbox")
	provider.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestWebhookDuplicateRequestDoesNotVerifyAgain(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)
	h := handlers.NewPaymentsHandler(repo, map[string]domain.PaymentProvider{"nomba": provider})

	req := httptest.NewRequest(http.MethodPost, "/v1/payments/webhook/nomba", bytes.NewReader([]byte(`{"event_type":"SUCCESS"}`)))
	req.Header.Set("nomba-signature", "valid-fake-sig")
	w := httptest.NewRecorder()

	provider.On("ParseWebhook", mock.Anything, req).Return(&domain.WebhookEvent{
		EventType: "SUCCESS",
		OrderRef:  "TXN-duplicate",
		RequestID: "req-duplicate",
	}, nil)
	repo.On("HasWebhookEvent", mock.Anything, "nomba", "req-duplicate").Return(true, nil)

	h.NombaWebhook(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	provider.AssertNotCalled(t, "Verify")
	repo.AssertNotCalled(t, "GetAttemptByInternalRef")
	repo.AssertNotCalled(t, "Update")
	repo.AssertNotCalled(t, "CreatePaymentEventInOutbox")
	provider.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestWebhookFailureFlowPublishesFailureEvent(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)
	h := handlers.NewPaymentsHandler(repo, map[string]domain.PaymentProvider{"nomba": provider})

	payload := []byte(`{"event_type": "FAILED"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/payments/webhook/nomba", bytes.NewReader(payload))
	req.Header.Set("nomba-signature", "valid-fake-sig")

	w := httptest.NewRecorder()

	provider.On("ParseWebhook", mock.Anything, req).Return(&domain.WebhookEvent{
		EventType: "FAILED",
		OrderRef:  "TXN-9999",
		RequestID: "req-9999",
	}, nil)

	repo.On("GetAttemptByInternalRef", mock.Anything, "TXN-9999").Return(&domain.PaymentAttempt{
		ID:            "attempt-999",
		TransactionID: "db-id-999",
		Provider:      "nomba",
		Status:        domain.AttemptActive,
		InternalRef:   "TXN-9999",
	}, nil)

	provider.On("Verify", mock.Anything, "TXN-9999").Return(&domain.VerificationResponse{
		Status:        "FAILED",
		TransactionID: "PAY-999",
		AmountCents:   100000,
	}, nil)

	repo.On("WithTransaction", mock.Anything).Return(nil)
	repo.On("GetByID", mock.Anything, "db-id-999").Return(&domain.Transaction{
		ID:               "db-id-999",
		BookingID:        "book-999",
		CustomerID:       "customer-999",
		Status:           domain.TxPending,
		InternalRef:      "TXN-9999",
		CurrentAttemptID: strPtr("attempt-999"),
	}, nil)
	repo.On("UpdateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.Status == domain.AttemptFailed
	})).Return(nil)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.Status == domain.TxFailed
	})).Return(nil)
	repo.On("CreatePaymentEventInOutbox", mock.Anything, "payment-v1-failed", mock.MatchedBy(func(event *eventscommon.EventEnvelope[eventscommon.PaymentEvent]) bool {
		return event != nil && event.Data.Reason != nil && *event.Data.Reason == "payment_failed"
	})).Return(nil)

	h.NombaWebhook(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	provider.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestWebhookCancelledStatusUsesFailureFlow(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)
	h := handlers.NewPaymentsHandler(repo, map[string]domain.PaymentProvider{"nomba": provider})

	req := httptest.NewRequest(http.MethodPost, "/v1/payments/webhook/nomba", bytes.NewReader([]byte(`{"event_type": "CANCELLED"}`)))
	req.Header.Set("nomba-signature", "valid-fake-sig")
	w := httptest.NewRecorder()

	provider.On("ParseWebhook", mock.Anything, req).Return(&domain.WebhookEvent{
		EventType: "CANCELLED",
		OrderRef:  "TXN-cancelled",
		RequestID: "req-cancelled",
	}, nil)
	repo.On("GetAttemptByInternalRef", mock.Anything, "TXN-cancelled").Return(&domain.PaymentAttempt{
		ID:            "attempt-cancelled",
		TransactionID: "txn-cancelled",
		Provider:      "nomba",
		Status:        domain.AttemptActive,
		InternalRef:   "TXN-cancelled",
	}, nil)
	provider.On("Verify", mock.Anything, "TXN-cancelled").Return(&domain.VerificationResponse{
		Status:        "CANCELLED",
		TransactionID: "PAY-cancelled",
		AmountCents:   100000,
	}, nil)
	repo.On("WithTransaction", mock.Anything).Return(nil)
	repo.On("GetByID", mock.Anything, "txn-cancelled").Return(&domain.Transaction{
		ID:               "txn-cancelled",
		BookingID:        "booking-cancelled",
		QuoteID:          "quote-cancelled",
		CustomerID:       "customer-cancelled",
		Status:           domain.TxPending,
		InternalRef:      "TXN-cancelled",
		CurrentAttemptID: strPtr("attempt-cancelled"),
	}, nil)
	repo.On("UpdateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.ID == "attempt-cancelled" && attempt.Status == domain.AttemptFailed
	})).Return(nil)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.ID == "txn-cancelled" && txn.Status == domain.TxFailed
	})).Return(nil)
	repo.On("CreatePaymentEventInOutbox", mock.Anything, "payment-v1-failed", mock.Anything).Return(nil)

	h.NombaWebhook(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	provider.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestWebhookReversedStatusIsAcknowledgedWithoutStateChange(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)
	h := handlers.NewPaymentsHandler(repo, map[string]domain.PaymentProvider{"nomba": provider})

	req := httptest.NewRequest(http.MethodPost, "/v1/payments/webhook/nomba", bytes.NewReader([]byte(`{"event_type": "REVERSED"}`)))
	req.Header.Set("nomba-signature", "valid-fake-sig")
	w := httptest.NewRecorder()

	provider.On("ParseWebhook", mock.Anything, req).Return(&domain.WebhookEvent{
		EventType: "REVERSED",
		OrderRef:  "TXN-reversed",
		RequestID: "req-reversed",
	}, nil)
	repo.On("GetAttemptByInternalRef", mock.Anything, "TXN-reversed").Return(&domain.PaymentAttempt{
		ID:            "attempt-reversed",
		TransactionID: "txn-reversed",
		Provider:      "nomba",
		Status:        domain.AttemptSuccessful,
		InternalRef:   "TXN-reversed",
	}, nil)
	provider.On("Verify", mock.Anything, "TXN-reversed").Return(&domain.VerificationResponse{
		Status:        "REVERSED",
		TransactionID: "PAY-reversed",
		AmountCents:   100000,
	}, nil)
	repo.On("WithTransaction", mock.Anything).Return(nil)
	repo.On("GetByID", mock.Anything, "txn-reversed").Return(&domain.Transaction{
		ID:               "txn-reversed",
		BookingID:        "booking-reversed",
		QuoteID:          "quote-reversed",
		CustomerID:       "customer-reversed",
		Status:           domain.TxSuccessful,
		InternalRef:      "TXN-reversed",
		CurrentAttemptID: strPtr("attempt-reversed"),
	}, nil)

	h.NombaWebhook(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	repo.AssertNotCalled(t, "UpdateAttempt")
	repo.AssertNotCalled(t, "Update")
	repo.AssertNotCalled(t, "CreatePaymentEventInOutbox")
	provider.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestPaymentSuccessPublishesReservationEventAndMarksOutbox(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)
	h := handlers.NewPaymentsHandler(repo, map[string]domain.PaymentProvider{"nomba": provider})

	req := httptest.NewRequest(http.MethodPost, "/v1/payments/webhook/nomba", bytes.NewReader([]byte(`{"event_type":"SUCCESS"}`)))
	req.Header.Set("nomba-signature", "valid-fake-sig")
	w := httptest.NewRecorder()

	provider.On("ParseWebhook", mock.Anything, req).Return(&domain.WebhookEvent{
		EventType: "SUCCESS",
		OrderRef:  "TXN-success",
		RequestID: "req-success",
	}, nil)
	repo.On("HasWebhookEvent", mock.Anything, "nomba", "req-success").Return(false, nil)
	repo.On("GetAttemptByInternalRef", mock.Anything, "TXN-success").Return(&domain.PaymentAttempt{
		ID:            "attempt-success",
		TransactionID: "txn-success",
		Provider:      "nomba",
		Status:        domain.AttemptActive,
		InternalRef:   "TXN-success",
	}, nil)
	provider.On("Verify", mock.Anything, "TXN-success").Return(&domain.VerificationResponse{
		Status:        "SUCCESS",
		TransactionID: "PAY-success",
		AmountCents:   1050000,
		Currency:      "NGN",
	}, nil)
	repo.On("WithTransaction", mock.Anything).Return(nil)
	repo.On("RecordWebhookEvent", mock.Anything, "nomba", "req-success", mock.AnythingOfType("time.Time")).Return(true, nil)
	repo.On("GetByID", mock.Anything, "txn-success").Return(&domain.Transaction{
		ID:               "txn-success",
		BookingID:        "booking-1",
		QuoteID:          "quote-1",
		CustomerID:       "customer-1",
		Provider:         "nomba",
		AmountCents:      1050000,
		Currency:         "NGN",
		Status:           domain.TxPending,
		InternalRef:      "TXN-success",
		CurrentAttemptID: strPtr("attempt-success"),
		Metadata: map[string]string{
			"payment_reservation_key": "reservation-1",
		},
	}, nil)
	repo.On("UpdateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.ID == "attempt-success" && attempt.Status == domain.AttemptSuccessful && attempt.ProviderRef == "PAY-success"
	})).Return(nil)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.ID == "txn-success" && txn.Status == domain.TxSuccessful && txn.Metadata["payment_status_event_enqueued_for"] == ""
	})).Return(nil).Once()
	repo.On("CreatePaymentEventInOutbox", mock.Anything, "payment-v1-confirmed", mock.MatchedBy(func(event *eventscommon.EventEnvelope[eventscommon.PaymentEvent]) bool {
		return event != nil &&
			event.Data.TransactionID == "txn-success" &&
			event.Data.BookingID == "booking-1" &&
			event.Data.QuoteID == "quote-1" &&
			event.Data.CustomerID == "customer-1" &&
			event.Data.ReservationKey == "reservation-1" &&
			event.Data.ProviderRef == "PAY-success" &&
			event.Data.Status == string(domain.TxSuccessful)
	})).Return(nil)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.ID == "txn-success" &&
			txn.Status == domain.TxSuccessful &&
			txn.Metadata["payment_status_event_enqueued_for"] == string(domain.TxSuccessful) &&
			txn.Metadata["payment_status_event_enqueued_at"] != ""
	})).Return(nil).Once()

	h.NombaWebhook(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	provider.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestNombaPayloadExtraction(t *testing.T) {
	rawJSON := []byte(`{
		"event_type": "SUCCESS",
		"timestamp": "12345678",
		"data": {
			"requestId": 999,
			"orderReference": "internal-ref-123"
		}
	}`)

	var payload struct {
		EventType string `json:"event_type"`
		Data      struct {
			OrderReference string `json:"orderReference"`
		} `json:"data"`
	}

	err := json.Unmarshal(rawJSON, &payload)
	assert.NoError(t, err)
	assert.Equal(t, "SUCCESS", payload.EventType)
	assert.Equal(t, "internal-ref-123", payload.Data.OrderReference)
}

func TestInitializePayment_BookingNotFound(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)
	h := handlers.NewPaymentsHandler(repo, map[string]domain.PaymentProvider{"nomba": provider})

	req := &handlers.InitializePaymentRequest{
		BookingID: "invalid-mock",
		QuoteID:   "quote-1",
		Provider:  "nomba",
	}

	// booking.GetBooking will fail because we are in a unit test without service bridges.
	res, err := h.InitializePayment(context.Background(), req)

	require.Error(t, err)
	require.Nil(t, res)
	assert.Contains(t, err.Error(), "booking not found")
}

func TestInitializePaymentDerivesAmountAndMarksBookingPending(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)

	h := handlers.NewPaymentsHandler(
		repo,
		map[string]domain.PaymentProvider{"nomba": provider},
		handlers.WithBookingGetter(func(ctx context.Context, id string) (*bookinghandlers.BookingResponse, error) {
			return &bookinghandlers.BookingResponse{
				ID:         id,
				CustomerID: "customer-1",
				Status:     bdomain.BookingQuoteAccepted,
			}, nil
		}),
		handlers.WithQuoteGetter(func(ctx context.Context, id string) (*quote.PaymentQuoteResponse, error) {
			return &quote.PaymentQuoteResponse{
				ID:          id,
				BookingID:   "booking-1",
				State:       "accepted",
				AmountCents: 1050000,
				Currency:    "NGN",
			}, nil
		}),
		handlers.WithBookingPaymentPendingUpdater(func(ctx context.Context, id string, req *booking.MarkPaymentPendingRequest) (*booking.BookingPaymentStatusResponse, error) {
			assert.Equal(t, "customer-1", req.UserID)
			assert.Equal(t, "quote-1", req.QuoteID)
			assert.NotEmpty(t, req.ReservationKey)
			assert.True(t, req.ReservedUntil.After(time.Now()))
			return &booking.BookingPaymentStatusResponse{
				BookingID:      id,
				Status:         bdomain.BookingPaymentPending,
				QuoteID:        req.QuoteID,
				ReservationKey: req.ReservationKey,
				ReservedUntil:  &req.ReservedUntil,
				UpdatedAt:      time.Now(),
			}, nil
		}),
		handlers.WithCustomerEmailGetter(func(ctx context.Context, customerID string) (string, error) {
			assert.Equal(t, "customer-1", customerID)
			return "customer@example.com", nil
		}),
		handlers.WithUserIDProvider(func() (string, bool) {
			return "customer-1", true
		}),
		handlers.WithPaymentConfig(handlers.PaymentConfig{
			PublicBaseURL:       "https://pay.example.com",
			AppReturnURL:        "protisan://payment-return",
			ReturnContextSecret: "test-secret",
			ReturnContextTTL:    10 * time.Minute,
		}),
	)

	repo.On("GetPendingByBookingAndQuote", mock.Anything, "booking-1", "quote-1").Return(nil, nil)
	repo.On("Create", mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.BookingID == "booking-1" &&
			txn.QuoteID == "quote-1" &&
			txn.AmountCents == 1050000 &&
			txn.Currency == "NGN" &&
			txn.Status == domain.TxPending
	})).Return(nil)
	repo.On("CountAttemptsByTransactionID", mock.Anything, "test-txn-id").Return(int64(0), nil)
	repo.On("CreateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.TransactionID == "test-txn-id" &&
			attempt.AttemptNo == 1 &&
			attempt.Status == domain.AttemptInitializing
	})).Return(nil)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.CurrentAttemptID != nil && *txn.CurrentAttemptID == "test-attempt-id" &&
			txn.InternalRef != "" &&
			txn.Metadata["checkout_url"] == ""
	})).Return(nil)
	provider.On("Initialize", mock.Anything, mock.MatchedBy(func(req *domain.InitializationRequest) bool {
		return req.AmountCents == 1050000 &&
			req.Currency == "NGN" &&
			req.CustomerEmail == "customer@example.com" &&
			req.CallbackURL != "" &&
			strings.HasPrefix(req.CallbackURL, "https://pay.example.com/checkout/complete?ctx=")
	})).Return(&domain.InitializationResponse{
		CheckoutLink: "https://sandbox.nomba.com/checkout/order/123",
		OrderRef:     "TXN-booking-1-123",
	}, nil)
	repo.On("UpdateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.Status == domain.AttemptActive &&
			attempt.CheckoutURL == "https://sandbox.nomba.com/checkout/order/123" &&
			attempt.ReturnURL != "" &&
			attempt.ReturnContext != ""
	})).Return(nil)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.Metadata["checkout_url"] == "https://sandbox.nomba.com/checkout/order/123" &&
			txn.Metadata["return_url"] != "" &&
			txn.Metadata["return_ctx"] != "" &&
			txn.CurrentAttemptID != nil && *txn.CurrentAttemptID == "test-attempt-id"
	})).Return(nil)

	res, err := h.InitializePayment(context.Background(), &handlers.InitializePaymentRequest{
		BookingID: "booking-1",
		QuoteID:   "quote-1",
		Provider:  "nomba",
	})

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "booking-1", res.BookingID)
	assert.Equal(t, "quote-1", res.QuoteID)
	assert.Equal(t, "pending", res.Status)
	assert.Equal(t, "https://sandbox.nomba.com/checkout/order/123", res.CheckoutURL)
	assert.NotEmpty(t, res.ReturnURL)
	repo.AssertExpectations(t)
	provider.AssertExpectations(t)
}

func TestInitializePaymentCancelsAndRotatesAttemptOnRetry(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)

	h := handlers.NewPaymentsHandler(
		repo,
		map[string]domain.PaymentProvider{"nomba": provider},
		handlers.WithBookingGetter(func(ctx context.Context, id string) (*bookinghandlers.BookingResponse, error) {
			return &bookinghandlers.BookingResponse{
				ID:         id,
				CustomerID: "customer-1",
				Status:     bdomain.BookingQuoteAccepted,
			}, nil
		}),
		handlers.WithQuoteGetter(func(ctx context.Context, id string) (*quote.PaymentQuoteResponse, error) {
			return &quote.PaymentQuoteResponse{
				ID:          id,
				BookingID:   "booking-1",
				State:       "accepted",
				AmountCents: 1050000,
				Currency:    "NGN",
			}, nil
		}),
		handlers.WithBookingPaymentPendingUpdater(func(ctx context.Context, id string, req *booking.MarkPaymentPendingRequest) (*booking.BookingPaymentStatusResponse, error) {
			return &booking.BookingPaymentStatusResponse{
				BookingID: id,
				Status:    bdomain.BookingPaymentPending,
				UpdatedAt: time.Now(),
			}, nil
		}),
		handlers.WithCustomerEmailGetter(func(ctx context.Context, customerID string) (string, error) {
			assert.Equal(t, "customer-1", customerID)
			return "customer@example.com", nil
		}),
		handlers.WithUserIDProvider(func() (string, bool) {
			return "customer-1", true
		}),
		handlers.WithPaymentConfig(handlers.PaymentConfig{
			PublicBaseURL:       "https://pay.example.com",
			AppReturnURL:        "protisan://payment-return",
			ReturnContextSecret: "test-secret",
		}),
	)

	repo.On("GetPendingByBookingAndQuote", mock.Anything, "booking-1", "quote-1").Return(&domain.Transaction{
		ID:               "txn-pending",
		CustomerID:       "customer-1",
		BookingID:        "booking-1",
		QuoteID:          "quote-1",
		Provider:         "nomba",
		Status:           domain.TxPending,
		InternalRef:      "TXN-pending",
		CurrentAttemptID: strPtr("attempt-old"),
		Metadata: map[string]string{
			"checkout_url":            "https://sandbox.nomba.com/checkout/order/existing",
			"return_url":              "https://pay.example.com/checkout/complete?ctx=existing",
			"payment_reservation_key": "reservation-existing",
		},
	}, nil)
	repo.On("GetCurrentAttemptByTransactionID", mock.Anything, "txn-pending").Return(&domain.PaymentAttempt{
		ID:            "attempt-old",
		TransactionID: "txn-pending",
		AttemptNo:     1,
		Status:        domain.AttemptActive,
		InternalRef:   "TXN-pending",
		CheckoutURL:   "https://sandbox.nomba.com/checkout/order/existing",
		ReturnURL:     "https://pay.example.com/checkout/complete?ctx=existing",
	}, nil)
	provider.On("Cancel", mock.Anything, "TXN-pending").Return(nil)
	repo.On("UpdateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.ID == "attempt-old" && attempt.Status == domain.AttemptCancelled
	})).Return(nil).Once()
	repo.On("CountAttemptsByTransactionID", mock.Anything, "txn-pending").Return(int64(1), nil)
	repo.On("CreateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.TransactionID == "txn-pending" &&
			attempt.AttemptNo == 2 &&
			attempt.Status == domain.AttemptInitializing
	})).Return(nil)
	provider.On("Initialize", mock.Anything, mock.MatchedBy(func(req *domain.InitializationRequest) bool {
		return req.OrderReference != "TXN-pending"
	})).Return(&domain.InitializationResponse{
		CheckoutLink: "https://sandbox.nomba.com/checkout/order/new",
		OrderRef:     "TXN-pending-new",
	}, nil)
	repo.On("UpdateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.ID == "attempt-old" && attempt.SupersededByAttemptID != nil && *attempt.SupersededByAttemptID == "test-attempt-id"
	})).Return(nil).Once()
	repo.On("Update", mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.CurrentAttemptID != nil && *txn.CurrentAttemptID == "test-attempt-id" && txn.InternalRef != "TXN-pending"
	})).Return(nil)
	repo.On("UpdateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.ID == "test-attempt-id" &&
			attempt.Status == domain.AttemptActive &&
			attempt.CheckoutURL == "https://sandbox.nomba.com/checkout/order/new"
	})).Return(nil)

	res, err := h.InitializePayment(context.Background(), &handlers.InitializePaymentRequest{
		BookingID: "booking-1",
		QuoteID:   "quote-1",
		Provider:  "nomba",
	})

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "txn-pending", res.TransactionID)
	assert.Equal(t, "https://sandbox.nomba.com/checkout/order/new", res.CheckoutURL)
	repo.AssertExpectations(t)
	provider.AssertExpectations(t)
}

func TestInitializePaymentBlocksRetryWhenVerifyReturnsSuccessVariant(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)

	h := handlers.NewPaymentsHandler(
		repo,
		map[string]domain.PaymentProvider{"nomba": provider},
		handlers.WithBookingGetter(func(ctx context.Context, id string) (*bookinghandlers.BookingResponse, error) {
			return &bookinghandlers.BookingResponse{
				ID:         id,
				CustomerID: "customer-1",
				Status:     bdomain.BookingQuoteAccepted,
			}, nil
		}),
		handlers.WithQuoteGetter(func(ctx context.Context, id string) (*quote.PaymentQuoteResponse, error) {
			return &quote.PaymentQuoteResponse{
				ID:          id,
				BookingID:   "booking-1",
				State:       "accepted",
				AmountCents: 1050000,
				Currency:    "NGN",
			}, nil
		}),
		handlers.WithUserIDProvider(func() (string, bool) {
			return "customer-1", true
		}),
	)

	repo.On("GetPendingByBookingAndQuote", mock.Anything, "booking-1", "quote-1").Return(&domain.Transaction{
		ID:               "txn-pending",
		CustomerID:       "customer-1",
		BookingID:        "booking-1",
		QuoteID:          "quote-1",
		AmountCents:      1050000,
		Provider:         "nomba",
		Currency:         "NGN",
		Status:           domain.TxPending,
		InternalRef:      "TXN-pending",
		CurrentAttemptID: strPtr("attempt-old"),
		Metadata: map[string]string{
			"payment_reservation_key": "reservation-existing",
		},
	}, nil)
	repo.On("GetCurrentAttemptByTransactionID", mock.Anything, "txn-pending").Return(&domain.PaymentAttempt{
		ID:            "attempt-old",
		TransactionID: "txn-pending",
		AttemptNo:     1,
		Provider:      "nomba",
		Status:        domain.AttemptActive,
		InternalRef:   "TXN-pending",
	}, nil)
	provider.On("Cancel", mock.Anything, "TXN-pending").Return(assert.AnError)
	provider.On("Verify", mock.Anything, "TXN-pending").Return(&domain.VerificationResponse{
		Status:        "SUCCESSFUL",
		TransactionID: "PAY-success",
		AmountCents:   1050000,
		Currency:      "NGN",
	}, nil)
	repo.On("WithTransaction", mock.Anything).Return(nil)
	repo.On("GetByID", mock.Anything, "txn-pending").Return(&domain.Transaction{
		ID:               "txn-pending",
		CustomerID:       "customer-1",
		BookingID:        "booking-1",
		QuoteID:          "quote-1",
		AmountCents:      1050000,
		Provider:         "nomba",
		Currency:         "NGN",
		Status:           domain.TxPending,
		CurrentAttemptID: strPtr("attempt-old"),
		Metadata: map[string]string{
			"payment_reservation_key": "reservation-existing",
		},
	}, nil)
	repo.On("UpdateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.ID == "attempt-old" && attempt.Status == domain.AttemptSuccessful && attempt.ProviderRef == "PAY-success"
	})).Return(nil)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.ID == "txn-pending" && txn.Status == domain.TxSuccessful
	})).Return(nil)
	repo.On("CreatePaymentEventInOutbox", mock.Anything, "payment-v1-confirmed", mock.Anything).Return(nil)

	res, err := h.InitializePayment(context.Background(), &handlers.InitializePaymentRequest{
		BookingID: "booking-1",
		QuoteID:   "quote-1",
		Provider:  "nomba",
	})

	require.Error(t, err)
	require.Nil(t, res)
	assert.Contains(t, err.Error(), "payment already completed")
	repo.AssertNotCalled(t, "CreateAttempt")
	repo.AssertExpectations(t)
	provider.AssertExpectations(t)
}

func TestInitializePaymentRetriesWhenSuccessVerificationFailsValidation(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)

	h := handlers.NewPaymentsHandler(
		repo,
		map[string]domain.PaymentProvider{"nomba": provider},
		handlers.WithBookingGetter(func(ctx context.Context, id string) (*bookinghandlers.BookingResponse, error) {
			return &bookinghandlers.BookingResponse{
				ID:         id,
				CustomerID: "customer-1",
				Status:     bdomain.BookingPaymentPending,
			}, nil
		}),
		handlers.WithQuoteGetter(func(ctx context.Context, id string) (*quote.PaymentQuoteResponse, error) {
			return &quote.PaymentQuoteResponse{
				ID:          id,
				BookingID:   "booking-1",
				State:       "accepted",
				AmountCents: 1050000,
				Currency:    "NGN",
			}, nil
		}),
		handlers.WithBookingPaymentPendingUpdater(func(ctx context.Context, id string, req *booking.MarkPaymentPendingRequest) (*booking.BookingPaymentStatusResponse, error) {
			return &booking.BookingPaymentStatusResponse{
				BookingID: id,
				Status:    bdomain.BookingPaymentPending,
				UpdatedAt: time.Now(),
			}, nil
		}),
		handlers.WithCustomerEmailGetter(func(ctx context.Context, customerID string) (string, error) {
			return "customer@example.com", nil
		}),
		handlers.WithUserIDProvider(func() (string, bool) {
			return "customer-1", true
		}),
		handlers.WithPaymentConfig(handlers.PaymentConfig{
			PublicBaseURL:       "https://pay.example.com",
			AppReturnURL:        "protisan://payment-return",
			ReturnContextSecret: "test-secret",
		}),
	)

	repo.On("GetPendingByBookingAndQuote", mock.Anything, "booking-1", "quote-1").Return(&domain.Transaction{
		ID:               "txn-pending",
		CustomerID:       "customer-1",
		BookingID:        "booking-1",
		QuoteID:          "quote-1",
		AmountCents:      1050000,
		Provider:         "nomba",
		Currency:         "NGN",
		Status:           domain.TxPending,
		InternalRef:      "TXN-pending",
		CurrentAttemptID: strPtr("attempt-old"),
		Metadata: map[string]string{
			"payment_reservation_key": "reservation-existing",
		},
	}, nil)
	repo.On("GetCurrentAttemptByTransactionID", mock.Anything, "txn-pending").Return(&domain.PaymentAttempt{
		ID:            "attempt-old",
		TransactionID: "txn-pending",
		AttemptNo:     1,
		Provider:      "nomba",
		Status:        domain.AttemptActive,
		InternalRef:   "TXN-pending",
	}, nil)
	provider.On("Cancel", mock.Anything, "TXN-pending").Return(assert.AnError)
	provider.On("Verify", mock.Anything, "TXN-pending").Return(&domain.VerificationResponse{
		Status:        "SUCCESS",
		TransactionID: "PAY-success",
		AmountCents:   999,
		Currency:      "NGN",
	}, nil)
	repo.On("GetByID", mock.Anything, "txn-pending").Return(&domain.Transaction{
		ID:               "txn-pending",
		CustomerID:       "customer-1",
		BookingID:        "booking-1",
		QuoteID:          "quote-1",
		AmountCents:      1050000,
		Provider:         "nomba",
		Currency:         "NGN",
		Status:           domain.TxPending,
		InternalRef:      "TXN-pending",
		CurrentAttemptID: strPtr("attempt-old"),
		Metadata: map[string]string{
			"payment_reservation_key": "reservation-existing",
		},
	}, nil).Once()
	repo.On("UpdateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.ID == "attempt-old" && attempt.Status == domain.AttemptSuperseded
	})).Return(nil).Once()
	repo.On("CountAttemptsByTransactionID", mock.Anything, "txn-pending").Return(int64(1), nil)
	repo.On("CreateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.TransactionID == "txn-pending" &&
			attempt.AttemptNo == 2 &&
			attempt.Status == domain.AttemptInitializing
	})).Return(nil)
	provider.On("Initialize", mock.Anything, mock.MatchedBy(func(req *domain.InitializationRequest) bool {
		return req.OrderReference != "TXN-pending"
	})).Return(&domain.InitializationResponse{
		CheckoutLink: "https://sandbox.nomba.com/checkout/order/new",
		OrderRef:     "TXN-pending-new",
	}, nil)
	repo.On("UpdateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.ID == "attempt-old" && attempt.SupersededByAttemptID != nil && *attempt.SupersededByAttemptID == "test-attempt-id"
	})).Return(nil).Once()
	repo.On("Update", mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.CurrentAttemptID != nil && *txn.CurrentAttemptID == "test-attempt-id" && txn.InternalRef != "TXN-pending"
	})).Return(nil)
	repo.On("UpdateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.ID == "test-attempt-id" &&
			attempt.Status == domain.AttemptActive &&
			attempt.CheckoutURL == "https://sandbox.nomba.com/checkout/order/new"
	})).Return(nil)

	res, err := h.InitializePayment(context.Background(), &handlers.InitializePaymentRequest{
		BookingID: "booking-1",
		QuoteID:   "quote-1",
		Provider:  "nomba",
	})

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "txn-pending", res.TransactionID)
	assert.Equal(t, "https://sandbox.nomba.com/checkout/order/new", res.CheckoutURL)
	repo.AssertExpectations(t)
	provider.AssertExpectations(t)
}

func TestInitializePaymentBlocksRetryWhenSuccessReconciliationFailsPersistence(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)

	h := handlers.NewPaymentsHandler(
		repo,
		map[string]domain.PaymentProvider{"nomba": provider},
		handlers.WithBookingGetter(func(ctx context.Context, id string) (*bookinghandlers.BookingResponse, error) {
			return &bookinghandlers.BookingResponse{
				ID:         id,
				CustomerID: "customer-1",
				Status:     bdomain.BookingPaymentPending,
			}, nil
		}),
		handlers.WithQuoteGetter(func(ctx context.Context, id string) (*quote.PaymentQuoteResponse, error) {
			return &quote.PaymentQuoteResponse{
				ID:          id,
				BookingID:   "booking-1",
				State:       "accepted",
				AmountCents: 1050000,
				Currency:    "NGN",
			}, nil
		}),
		handlers.WithCustomerEmailGetter(func(ctx context.Context, customerID string) (string, error) {
			return "customer@example.com", nil
		}),
		handlers.WithUserIDProvider(func() (string, bool) {
			return "customer-1", true
		}),
		handlers.WithPaymentConfig(handlers.PaymentConfig{
			PublicBaseURL:       "https://pay.example.com",
			AppReturnURL:        "protisan://payment-return",
			ReturnContextSecret: "test-secret",
		}),
	)

	repo.On("GetPendingByBookingAndQuote", mock.Anything, "booking-1", "quote-1").Return(&domain.Transaction{
		ID:               "txn-pending",
		CustomerID:       "customer-1",
		BookingID:        "booking-1",
		QuoteID:          "quote-1",
		AmountCents:      1050000,
		Provider:         "nomba",
		Currency:         "NGN",
		Status:           domain.TxPending,
		InternalRef:      "TXN-pending",
		CurrentAttemptID: strPtr("attempt-old"),
		Metadata: map[string]string{
			"payment_reservation_key": "reservation-existing",
		},
	}, nil)
	repo.On("GetCurrentAttemptByTransactionID", mock.Anything, "txn-pending").Return(&domain.PaymentAttempt{
		ID:            "attempt-old",
		TransactionID: "txn-pending",
		AttemptNo:     1,
		Provider:      "nomba",
		Status:        domain.AttemptActive,
		InternalRef:   "TXN-pending",
	}, nil)
	provider.On("Cancel", mock.Anything, "TXN-pending").Return(assert.AnError)
	provider.On("Verify", mock.Anything, "TXN-pending").Return(&domain.VerificationResponse{
		Status:        "SUCCESS",
		TransactionID: "PAY-success",
		AmountCents:   1050000,
		Currency:      "NGN",
	}, nil)
	repo.On("GetByID", mock.Anything, "txn-pending").Return(&domain.Transaction{
		ID:               "txn-pending",
		CustomerID:       "customer-1",
		BookingID:        "booking-1",
		QuoteID:          "quote-1",
		AmountCents:      1050000,
		Provider:         "nomba",
		Currency:         "NGN",
		Status:           domain.TxPending,
		InternalRef:      "TXN-pending",
		CurrentAttemptID: strPtr("attempt-old"),
		Metadata: map[string]string{
			"payment_reservation_key": "reservation-existing",
		},
	}, nil).Once()
	repo.On("UpdateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.ID == "attempt-old" && attempt.Status == domain.AttemptSuccessful
	})).Return(assert.AnError).Once()

	res, err := h.InitializePayment(context.Background(), &handlers.InitializePaymentRequest{
		BookingID: "booking-1",
		QuoteID:   "quote-1",
		Provider:  "nomba",
	})

	require.Error(t, err)
	require.Nil(t, res)
	repo.AssertExpectations(t)
	provider.AssertExpectations(t)
}

func TestInitializePaymentRejectsProviderSwitchForPendingTransaction(t *testing.T) {
	repo := new(mockRepository)
	nombaProvider := new(mockProvider)
	stripeProvider := new(mockProvider)

	h := handlers.NewPaymentsHandler(
		repo,
		map[string]domain.PaymentProvider{
			"nomba":  nombaProvider,
			"stripe": stripeProvider,
		},
		handlers.WithBookingGetter(func(ctx context.Context, id string) (*bookinghandlers.BookingResponse, error) {
			return &bookinghandlers.BookingResponse{
				ID:         id,
				CustomerID: "customer-1",
				Status:     bdomain.BookingPaymentPending,
			}, nil
		}),
		handlers.WithQuoteGetter(func(ctx context.Context, id string) (*quote.PaymentQuoteResponse, error) {
			return &quote.PaymentQuoteResponse{
				ID:          id,
				BookingID:   "booking-1",
				State:       "accepted",
				AmountCents: 1050000,
				Currency:    "NGN",
			}, nil
		}),
		handlers.WithUserIDProvider(func() (string, bool) {
			return "customer-1", true
		}),
	)

	repo.On("GetPendingByBookingAndQuote", mock.Anything, "booking-1", "quote-1").Return(&domain.Transaction{
		ID:         "txn-pending",
		CustomerID: "customer-1",
		BookingID:  "booking-1",
		QuoteID:    "quote-1",
		Provider:   "nomba",
		Status:     domain.TxPending,
		Metadata: map[string]string{
			"payment_reservation_key": "reservation-existing",
		},
	}, nil)

	res, err := h.InitializePayment(context.Background(), &handlers.InitializePaymentRequest{
		BookingID: "booking-1",
		QuoteID:   "quote-1",
		Provider:  "stripe",
	})

	require.Error(t, err)
	require.Nil(t, res)
	assert.Contains(t, err.Error(), "existing pending transaction uses provider nomba")
	repo.AssertNotCalled(t, "GetCurrentAttemptByTransactionID")
	repo.AssertNotCalled(t, "CountAttemptsByTransactionID")
	repo.AssertNotCalled(t, "CreateAttempt")
	repo.AssertNotCalled(t, "Update")
	stripeProvider.AssertNotCalled(t, "Initialize")
	repo.AssertExpectations(t)
}

func TestInitializePaymentReleasesReservationWhenProviderInitializationFails(t *testing.T) {
	repo := new(mockRepository)
	provider := new(mockProvider)
	released := false

	h := handlers.NewPaymentsHandler(
		repo,
		map[string]domain.PaymentProvider{"nomba": provider},
		handlers.WithBookingGetter(func(ctx context.Context, id string) (*bookinghandlers.BookingResponse, error) {
			return &bookinghandlers.BookingResponse{
				ID:         id,
				CustomerID: "customer-1",
				Status:     bdomain.BookingQuoteAccepted,
			}, nil
		}),
		handlers.WithQuoteGetter(func(ctx context.Context, id string) (*quote.PaymentQuoteResponse, error) {
			return &quote.PaymentQuoteResponse{
				ID:          id,
				BookingID:   "booking-1",
				State:       "accepted",
				AmountCents: 500000,
				Currency:    "NGN",
			}, nil
		}),
		handlers.WithBookingPaymentPendingUpdater(func(ctx context.Context, id string, req *booking.MarkPaymentPendingRequest) (*booking.BookingPaymentStatusResponse, error) {
			return &booking.BookingPaymentStatusResponse{
				BookingID:      id,
				Status:         bdomain.BookingPaymentPending,
				QuoteID:        req.QuoteID,
				ReservationKey: req.ReservationKey,
				ReservedUntil:  &req.ReservedUntil,
				UpdatedAt:      time.Now(),
			}, nil
		}),
		handlers.WithBookingPaymentReservationReleaser(func(ctx context.Context, id string, req *booking.ReleasePaymentReservationRequest) (*booking.BookingPaymentStatusResponse, error) {
			released = true
			assert.Equal(t, "booking-1", id)
			assert.Equal(t, "quote-1", req.QuoteID)
			assert.NotEmpty(t, req.ReservationKey)
			require.NotNil(t, req.Reason)
			assert.Equal(t, "provider_initialization_failed", *req.Reason)
			return &booking.BookingPaymentStatusResponse{
				BookingID: id,
				Status:    bdomain.BookingQuoteAccepted,
				UpdatedAt: time.Now(),
			}, nil
		}),
		handlers.WithCustomerEmailGetter(func(ctx context.Context, customerID string) (string, error) {
			assert.Equal(t, "customer-1", customerID)
			return "customer@example.com", nil
		}),
		handlers.WithUserIDProvider(func() (string, bool) {
			return "customer-1", true
		}),
		handlers.WithPaymentConfig(handlers.PaymentConfig{
			PublicBaseURL:       "https://pay.example.com",
			AppReturnURL:        "protisan://payment-return",
			ReturnContextSecret: "test-secret",
		}),
	)

	repo.On("GetPendingByBookingAndQuote", mock.Anything, "booking-1", "quote-1").Return(nil, nil)
	repo.On("Create", mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.Metadata["payment_reservation_key"] != ""
	})).Return(nil)
	repo.On("CountAttemptsByTransactionID", mock.Anything, "test-txn-id").Return(int64(0), nil)
	repo.On("CreateAttempt", mock.Anything, mock.AnythingOfType("*domain.PaymentAttempt")).Return(nil)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.CurrentAttemptID != nil && *txn.CurrentAttemptID == "test-attempt-id" &&
			txn.Metadata["payment_reservation_key"] != "" &&
			txn.Metadata["last_initialize_attempt_id"] == ""
	})).Return(nil).Once()
	provider.On("Initialize", mock.Anything, mock.AnythingOfType("*domain.InitializationRequest")).Return((*domain.InitializationResponse)(nil), assert.AnError)
	repo.On("UpdateAttempt", mock.Anything, mock.MatchedBy(func(attempt *domain.PaymentAttempt) bool {
		return attempt.Status == domain.AttemptFailed
	})).Return(nil)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.Metadata["last_initialize_attempt_id"] == "test-attempt-id" &&
			txn.Metadata["last_initialize_error"] != ""
	})).Return(nil)

	res, err := h.InitializePayment(context.Background(), &handlers.InitializePaymentRequest{
		BookingID: "booking-1",
		QuoteID:   "quote-1",
		Provider:  "nomba",
	})

	require.Error(t, err)
	require.Nil(t, res)
	assert.True(t, released)
	repo.AssertExpectations(t)
	provider.AssertExpectations(t)
}

func TestGetPaymentReturnsBookingStatus(t *testing.T) {
	repo := new(mockRepository)

	h := handlers.NewPaymentsHandler(
		repo,
		nil,
		handlers.WithBookingGetter(func(ctx context.Context, id string) (*bookinghandlers.BookingResponse, error) {
			return &bookinghandlers.BookingResponse{
				ID:         id,
				CustomerID: "customer-1",
				Status:     bdomain.BookingPaymentPending,
			}, nil
		}),
		handlers.WithUserIDProvider(func() (string, bool) {
			return "customer-1", true
		}),
	)

	repo.On("GetByID", mock.Anything, "txn-1").Return(&domain.Transaction{
		ID:          "txn-1",
		CustomerID:  "customer-1",
		BookingID:   "booking-1",
		QuoteID:     "quote-1",
		AmountCents: 1050000,
		Currency:    "NGN",
		Provider:    "nomba",
		Status:      domain.TxPending,
		InternalRef: "TXN-1",
		UpdatedAt:   time.Now(),
	}, nil)
	repo.On("GetCurrentAttemptByTransactionID", mock.Anything, "txn-1").Return(&domain.PaymentAttempt{
		ID:          "attempt-1",
		InternalRef: "TXN-ATTEMPT-1",
	}, nil)

	res, err := h.GetPayment(context.Background(), "txn-1")

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "payment_pending", res.BookingStatus)
	assert.Equal(t, "TXN-ATTEMPT-1", res.Reference)
	assert.Equal(t, int64(1050000), res.AmountCents)
	assert.Equal(t, 10500.0, res.Amount)
	repo.AssertExpectations(t)
}

func TestRepairPaymentOutboxEventsReenqueuesMissingCompletedPayment(t *testing.T) {
	repo := new(mockRepository)
	h := handlers.NewPaymentsHandler(repo, nil)

	repo.On("ListTransactionsMissingPaymentEvent", mock.Anything, mock.AnythingOfType("time.Time"), 100).Return([]*domain.Transaction{{
		ID:     "txn-success",
		Status: domain.TxSuccessful,
	}}, nil)
	repo.On("WithTransaction", mock.Anything).Return(nil)
	repo.On("GetByID", mock.Anything, "txn-success").Return(&domain.Transaction{
		ID:          "txn-success",
		BookingID:   "booking-1",
		QuoteID:     "quote-1",
		CustomerID:  "customer-1",
		Provider:    "nomba",
		AmountCents: 1050000,
		Currency:    "NGN",
		Status:      domain.TxSuccessful,
		InternalRef: "TXN-success",
		ProviderRef: "PAY-success",
		Metadata: map[string]string{
			"payment_reservation_key": "reservation-1",
		},
	}, nil)
	repo.On("CreatePaymentEventInOutbox", mock.Anything, "payment-v1-confirmed", mock.MatchedBy(func(event *eventscommon.EventEnvelope[eventscommon.PaymentEvent]) bool {
		return event != nil &&
			event.Data.TransactionID == "txn-success" &&
			event.Data.ReservationKey == "reservation-1" &&
			event.Data.Status == string(domain.TxSuccessful)
	})).Return(nil)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.ID == "txn-success" && txn.Metadata["payment_status_event_enqueued_for"] == string(domain.TxSuccessful)
	})).Return(nil)

	repaired, err := h.RepairPaymentOutboxEvents(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, repaired)
	repo.AssertExpectations(t)
}

func TestRepairPaymentOutboxEventsContinuesAfterCandidateFailure(t *testing.T) {
	repo := new(mockRepository)
	h := handlers.NewPaymentsHandler(repo, nil)

	repo.On("ListTransactionsMissingPaymentEvent", mock.Anything, mock.AnythingOfType("time.Time"), 100).Return([]*domain.Transaction{
		{ID: "txn-fail", Status: domain.TxSuccessful},
		{ID: "txn-success", Status: domain.TxSuccessful},
	}, nil)
	repo.On("WithTransaction", mock.Anything).Return(nil)
	repo.On("GetByID", mock.Anything, "txn-fail").Return((*domain.Transaction)(nil), assert.AnError).Once()
	repo.On("GetByID", mock.Anything, "txn-success").Return(&domain.Transaction{
		ID:          "txn-success",
		BookingID:   "booking-1",
		QuoteID:     "quote-1",
		CustomerID:  "customer-1",
		Provider:    "nomba",
		AmountCents: 1050000,
		Currency:    "NGN",
		Status:      domain.TxSuccessful,
		InternalRef: "TXN-success",
		ProviderRef: "PAY-success",
		Metadata: map[string]string{
			"payment_reservation_key": "reservation-1",
		},
	}, nil).Once()
	repo.On("CreatePaymentEventInOutbox", mock.Anything, "payment-v1-confirmed", mock.Anything).Return(nil)
	repo.On("Update", mock.Anything, mock.MatchedBy(func(txn *domain.Transaction) bool {
		return txn.ID == "txn-success" && txn.Metadata["payment_status_event_enqueued_for"] == string(domain.TxSuccessful)
	})).Return(nil)

	repaired, err := h.RepairPaymentOutboxEvents(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, repaired)
	repo.AssertExpectations(t)
}

func TestPaymentDiagnosticsReportsOperationalCounts(t *testing.T) {
	repo := new(mockRepository)
	h := handlers.NewPaymentsHandler(repo, nil)

	repo.On("CountPendingTransactionsOlderThan", mock.Anything, mock.AnythingOfType("time.Time")).Return(int64(2), nil)
	repo.On("CountTransactionsMissingPaymentEvent", mock.Anything, mock.AnythingOfType("time.Time")).Return(int64(1), nil)
	repo.On("CountWebhookEventsSince", mock.Anything, mock.AnythingOfType("time.Time")).Return(int64(7), nil)

	res, err := h.Diagnostics(context.Background())

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, int64(2), res.StuckPendingTransactions)
	assert.Equal(t, int64(1), res.CompletedMissingOutboxMarker)
	assert.Equal(t, int64(7), res.RecentWebhookEvents)
	assert.Equal(t, 30, res.StuckPendingOlderThanMinutes)
	assert.Equal(t, 24*60, res.WebhookEventsWindowMinutes)
	repo.AssertExpectations(t)
}
