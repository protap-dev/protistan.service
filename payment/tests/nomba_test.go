package tests

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"encore.app/core/cache"
	"encore.app/payment/domain"
	"encore.app/payment/providers"
)

func TestNombaProviderUsesNormalizedBaseURLAndDocumentedEndpoints(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests = append(requests, req.Method+" "+req.URL.RequestURI())

		if req.Header.Get("accountId") != "acct" {
			http.Error(w, "missing accountId header", http.StatusBadRequest)
			return
		}

		switch req.URL.Path {
		case providers.EndpointTokenIssue:
			_, _ = w.Write([]byte(`{"data":{"access_token":"token-1"}}`))
		case providers.EndpointCheckoutProduction:
			if req.Method != http.MethodPost {
				http.Error(w, "unexpected checkout method", http.StatusMethodNotAllowed)
				return
			}
			_, _ = w.Write([]byte(`{"data":{"orderReference":"ref-created","checkoutLink":"https://checkout.example/order/ref-created"}}`))
		case providers.EndpointTransaction:
			if req.Method != http.MethodGet {
				http.Error(w, "unexpected verify method", http.StatusMethodNotAllowed)
				return
			}
			if got := req.URL.Query().Get("orderReference"); got != "ref-1" {
				http.Error(w, "unexpected order reference", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"data":{"transactionId":"provider-txn-1","status":"SUCCESS","amount":10500}}`))
		default:
			http.NotFound(w, req)
		}
	}))
	defer server.Close()

	provider, err := providers.NewNombaProvider(cache.NewInMemoryCache(), server.URL+"/v1", "id", "secret", "sig", "acct")
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	initRes, err := provider.Initialize(context.Background(), &domain.InitializationRequest{
		OrderReference: "ref-1",
		Amount:         10500,
		Currency:       "NGN",
		CustomerEmail:  "customer@example.com",
		CallbackURL:    "https://pay.example.com/checkout/complete?ctx=token",
	})
	if err != nil {
		t.Fatalf("initialize payment: %v", err)
	}
	if initRes.CheckoutLink != "https://checkout.example/order/ref-created" {
		t.Fatalf("unexpected checkout link: %s", initRes.CheckoutLink)
	}

	verifyRes, err := provider.Verify(context.Background(), "ref-1")
	if err != nil {
		t.Fatalf("verify payment: %v", err)
	}
	if verifyRes.TransactionID != "provider-txn-1" || verifyRes.Status != "SUCCESS" || verifyRes.Amount != 10500 {
		t.Fatalf("unexpected verification response: %+v", verifyRes)
	}

	expectedRequests := []string{
		"POST /v1/auth/token/issue",
		"POST /v1/checkout/order",
		"GET /v1/transactions/accounts/single?orderReference=ref-1",
	}
	if !reflect.DeepEqual(requests, expectedRequests) {
		t.Fatalf("unexpected nomba requests:\nwant: %#v\n got: %#v", expectedRequests, requests)
	}
	if providers.EndpointCheckoutCancel != "/v1/checkout/order/cancel" {
		t.Fatalf("unexpected checkout cancel path: %s", providers.EndpointCheckoutCancel)
	}
}

func TestNombaProviderRequiresExplicitConfiguration(t *testing.T) {
	tests := []struct {
		name          string
		baseURL       string
		clientID      string
		clientSecret  string
		signatureKey  string
		accountID     string
		errorContains string
	}{
		{
			name:          "api URL",
			clientID:      "id",
			clientSecret:  "secret",
			signatureKey:  "sig",
			accountID:     "acct",
			errorContains: "API URL",
		},
		{
			name:          "client ID",
			baseURL:       "https://api.nomba.com",
			clientSecret:  "secret",
			signatureKey:  "sig",
			accountID:     "acct",
			errorContains: "client ID",
		},
		{
			name:          "client secret",
			baseURL:       "https://api.nomba.com",
			clientID:      "id",
			signatureKey:  "sig",
			accountID:     "acct",
			errorContains: "client secret",
		},
		{
			name:          "signature key",
			baseURL:       "https://api.nomba.com",
			clientID:      "id",
			clientSecret:  "secret",
			accountID:     "acct",
			errorContains: "signature key",
		},
		{
			name:          "account ID",
			baseURL:       "https://api.nomba.com",
			clientID:      "id",
			clientSecret:  "secret",
			signatureKey:  "sig",
			errorContains: "account ID",
		},
		{
			name:          "http API URL",
			baseURL:       "http://api.nomba.com",
			clientID:      "id",
			clientSecret:  "secret",
			signatureKey:  "sig",
			accountID:     "acct",
			errorContains: "https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := providers.NewNombaProvider(
				cache.NewInMemoryCache(),
				tt.baseURL,
				tt.clientID,
				tt.clientSecret,
				tt.signatureKey,
				tt.accountID,
			)
			if err == nil {
				t.Fatal("expected provider configuration error")
			}
			if provider != nil {
				t.Fatal("expected provider to be nil on configuration error")
			}
			if !strings.Contains(err.Error(), tt.errorContains) {
				t.Fatalf("expected error to contain %q, got %q", tt.errorContains, err.Error())
			}
		})
	}
}

func TestNombaWebhookRejectsStaleTimestamp(t *testing.T) {
	provider, err := providers.NewNombaProvider(cache.NewInMemoryCache(), "http://127.0.0.1:8080", "id", "secret", "sig", "acct")
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	timestamp := time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	body := []byte(`{"event_type":"payment_success","requestId":"req-1","data":{"merchant":{"userId":"merchant-1","walletId":"wallet-1"},"transaction":{"transactionId":"pay-1","type":"online_checkout","time":"2026-05-13T12:00:00Z","responseCode":"","transactionAmount":1000},"order":{"orderReference":"TXN-1"}}}`)
	seq := strings.Join([]string{
		"payment_success",
		"req-1",
		"merchant-1",
		"wallet-1",
		"pay-1",
		"online_checkout",
		"2026-05-13T12:00:00Z",
		"",
		timestamp,
	}, ":")
	mac := hmac.New(sha256.New, []byte("sig"))
	mac.Write([]byte(seq))

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("nomba-signature", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	req.Header.Set("nomba-timestamp", timestamp)

	event, err := provider.ParseWebhook(context.Background(), req)
	if err == nil {
		t.Fatal("expected stale webhook timestamp to be rejected")
	}
	if event != nil {
		t.Fatal("expected no webhook event for stale timestamp")
	}
}
