package tests

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"encore.app/payment/domain"
	"encore.app/payment/handlers"
)

type signedReturnContext struct {
	TransactionID string `json:"transaction_id"`
	AttemptID     string `json:"attempt_id,omitempty"`
	InternalRef   string `json:"internal_ref"`
	BookingID     string `json:"booking_id"`
	QuoteID       string `json:"quote_id"`
	CustomerID    string `json:"customer_id"`
	Provider      string `json:"provider,omitempty"`
	ReturnType    string `json:"return_type,omitempty"`
	ExpiresAt     int64  `json:"exp"`
}

func signPaymentReturnContext(t *testing.T, payload signedReturnContext, secret string) string {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal return context: %v", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	signature := mac.Sum(nil)

	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func TestHostedCheckoutCallbackRejectsTamperedContext(t *testing.T) {
	token := signPaymentReturnContext(t, signedReturnContext{
		TransactionID: "txn-1",
		BookingID:     "booking-1",
		QuoteID:       "quote-1",
		InternalRef:   "TXN-1",
		ExpiresAt:     time.Now().Add(5 * time.Minute).Unix(),
	}, "test-secret")

	handler := handlers.NewPaymentsHandler(nil, map[string]domain.PaymentProvider{}, handlers.WithPaymentConfig(handlers.PaymentConfig{
		AppReturnURL:        "protisan://payment-return",
		ReturnContextSecret: "test-secret",
	}))

	req := httptest.NewRequest(http.MethodGet, "/checkout/complete?ctx="+token+"tampered", nil)
	w := httptest.NewRecorder()

	handler.HostedCheckoutCallback(w, req, "complete")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHostedCheckoutCallbackRejectsExpiredContext(t *testing.T) {
	token := signPaymentReturnContext(t, signedReturnContext{
		TransactionID: "txn-1",
		BookingID:     "booking-1",
		QuoteID:       "quote-1",
		InternalRef:   "TXN-1",
		ExpiresAt:     time.Now().Add(-1 * time.Minute).Unix(),
	}, "test-secret")

	handler := handlers.NewPaymentsHandler(nil, map[string]domain.PaymentProvider{}, handlers.WithPaymentConfig(handlers.PaymentConfig{
		AppReturnURL:        "protisan://payment-return",
		ReturnContextSecret: "test-secret",
	}))

	req := httptest.NewRequest(http.MethodGet, "/checkout/complete?ctx="+token, nil)
	w := httptest.NewRecorder()

	handler.HostedCheckoutCallback(w, req, "complete")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "protisan://payment-return") {
		t.Fatalf("expected app handoff link in body, got %s", w.Body.String())
	}
}

func TestHostedCheckoutCallbackReturnsVerifiedPayload(t *testing.T) {
	token := signPaymentReturnContext(t, signedReturnContext{
		TransactionID: "txn-1",
		BookingID:     "booking-1",
		QuoteID:       "quote-1",
		InternalRef:   "TXN-1",
		ExpiresAt:     time.Now().Add(5 * time.Minute).Unix(),
	}, "test-secret")

	handler := handlers.NewPaymentsHandler(nil, map[string]domain.PaymentProvider{}, handlers.WithPaymentConfig(handlers.PaymentConfig{
		AppReturnURL:        "protisan://payment-return",
		ReturnContextSecret: "test-secret",
	}))

	req := httptest.NewRequest(http.MethodGet, "/checkout/complete?ctx="+token+"&orderReference=TXN-1", nil)
	w := httptest.NewRecorder()

	handler.HostedCheckoutCallback(w, req, "complete")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"transaction_id":"txn-1"`) {
		t.Fatalf("expected callback payload in body, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "protisan://payment-return?") {
		t.Fatalf("expected app return redirect in body, got %s", w.Body.String())
	}
}

func TestHostedCheckoutCallbackEscapesJSONPayloadInHTML(t *testing.T) {
	token := signPaymentReturnContext(t, signedReturnContext{
		TransactionID: `txn-</script><script>alert(1)</script>`,
		BookingID:     "booking-1",
		QuoteID:       "quote-1",
		InternalRef:   "TXN-1",
		ExpiresAt:     time.Now().Add(5 * time.Minute).Unix(),
	}, "test-secret")

	handler := handlers.NewPaymentsHandler(nil, map[string]domain.PaymentProvider{}, handlers.WithPaymentConfig(handlers.PaymentConfig{
		AppReturnURL:        "protisan://payment-return",
		ReturnContextSecret: "test-secret",
	}))

	req := httptest.NewRequest(http.MethodGet, "/checkout/complete?ctx="+token+"&orderReference=TXN-1", nil)
	w := httptest.NewRecorder()

	handler.HostedCheckoutCallback(w, req, "complete")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if strings.Contains(w.Body.String(), `</script><script>alert(1)</script>`) {
		t.Fatalf("expected script-breaking JSON to be escaped, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `type="application/json"`) {
		t.Fatalf("expected inert json script tag, got %s", w.Body.String())
	}
}
