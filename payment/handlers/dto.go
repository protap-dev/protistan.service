package handlers

import "time"

// InitializePaymentRequest is the client payload for starting a checkout flow.
type InitializePaymentRequest struct {
	BookingID             string   `json:"booking_id"`
	QuoteID               string   `json:"quote_id"`
	Provider              string   `json:"provider"`                          // e.g. "nomba"
	AllowedPaymentMethods []string `json:"allowed_payment_methods,omitempty"` // e.g. ["card", "transfer"]
}

// InitializePaymentResponse is returned to the client after checkout creation.
type InitializePaymentResponse struct {
	TransactionID string `json:"transaction_id"`
	BookingID     string `json:"booking_id,omitempty"`
	QuoteID       string `json:"quote_id,omitempty"`
	CheckoutURL   string `json:"checkout_url"`
	ReturnURL     string `json:"return_url,omitempty"`
	Reference     string `json:"reference"`
	Status        string `json:"status"`
}

// PaymentStatusResponse is returned to the client when verifying payment state.
type PaymentStatusResponse struct {
	TransactionID string    `json:"transaction_id"`
	BookingID     string    `json:"booking_id"`
	QuoteID       string    `json:"quote_id"`
	Reference     string    `json:"reference"`
	Provider      string    `json:"provider"`
	Status        string    `json:"status"`
	BookingStatus string    `json:"booking_status"`
	Amount        float64   `json:"amount"`
	Currency      string    `json:"currency"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type PaymentDiagnosticsResponse struct {
	StuckPendingTransactions       int64     `json:"stuck_pending_transactions"`
	CompletedMissingOutboxMarker   int64     `json:"completed_missing_outbox_marker"`
	RecentWebhookEvents            int64     `json:"recent_webhook_events"`
	StuckPendingOlderThanMinutes   int       `json:"stuck_pending_older_than_minutes"`
	WebhookEventsWindowMinutes     int       `json:"webhook_events_window_minutes"`
	MissingOutboxGracePeriodMinute int       `json:"missing_outbox_grace_period_minutes"`
	CheckedAt                      time.Time `json:"checked_at"`
}
