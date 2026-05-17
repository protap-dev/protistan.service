package domain

import (
	"context"
	"net/http"
	"time"
)

// InitializationRequest defines the inputs required to initialize a payment
type InitializationRequest struct {
	OrderReference        string
	AmountCents           int64
	Amount                float64
	Currency              string
	CustomerEmail         string
	CallbackURL           string
	AllowedPaymentMethods []string
	AccountID             string
}

// InitializationResponse represents the gateway response upon initialization
type InitializationResponse struct {
	CheckoutLink string
	OrderRef     string
}

// VerificationResponse represents the gateway response from a re-query
type VerificationResponse struct {
	Status        string
	TransactionID string
	AmountCents   int64
	Amount        float64
	Currency      string
	OrderRef      string
}

// WebhookEvent represents the normalized webhook payload fields
type WebhookEvent struct {
	TransactionID string
	OrderRef      string
	RequestID     string
	ReceivedAt    time.Time
	EventType     string
	Status        string
}

// PaymentProvider defines the contract for all external payment integrations
type PaymentProvider interface {
	// Identifier returns the name of the provider (e.g. "nomba")
	Identifier() string

	// Initialize triggers the external payment checkout/order
	Initialize(ctx context.Context, req *InitializationRequest) (*InitializationResponse, error)

	// Verify performs a synchronous re-query of the transaction status
	Verify(ctx context.Context, reference string) (*VerificationResponse, error)

	// Cancel invalidates an incomplete checkout order before a fresh retry is issued.
	Cancel(ctx context.Context, reference string) error

	// ParseWebhook decodes the provider payload and verifies the integrity (e.g., HMAC signature)
	ParseWebhook(ctx context.Context, req *http.Request) (*WebhookEvent, error)

	// Reconcile handles fetching settlement data
	Reconcile(ctx context.Context, accountID string, cursor string) (string, error)
}
