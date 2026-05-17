package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"encore.app/booking"
	bookinghandlers "encore.app/booking/handlers"
	"encore.app/quote"
	"encore.dev/beta/auth"
)

const (
	defaultReturnContextTTL = 24 * time.Hour
)

type PaymentConfig struct {
	PublicBaseURL       string
	AppReturnURL        string
	ReturnContextSecret string
	ReturnContextTTL    time.Duration
}

type HandlerOption func(*PaymentsHandler)

func NewPaymentConfig(publicBaseURL, appReturnURL, returnContextSecret string) (PaymentConfig, error) {
	if publicBaseURL == "" {
		return PaymentConfig{}, fmt.Errorf("payment publicBaseURL secret is required")
	}
	parsedPublicBaseURL, err := url.Parse(publicBaseURL)
	if err != nil || parsedPublicBaseURL.Scheme == "" || parsedPublicBaseURL.Host == "" {
		return PaymentConfig{}, fmt.Errorf("payment publicBaseURL must be an absolute URL")
	}
	if parsedPublicBaseURL.Scheme != "https" {
		return PaymentConfig{}, fmt.Errorf("payment publicBaseURL must use https")
	}
	if isLocalCallbackHost(parsedPublicBaseURL.Hostname()) {
		return PaymentConfig{}, fmt.Errorf("payment publicBaseURL cannot point to a local host")
	}
	if appReturnURL == "" {
		return PaymentConfig{}, fmt.Errorf("payment AppReturnURL secret is required")
	}
	if returnContextSecret == "" {
		return PaymentConfig{}, fmt.Errorf("payment ReturnContextKey secret is required")
	}

	return PaymentConfig{
		PublicBaseURL:       publicBaseURL,
		AppReturnURL:        appReturnURL,
		ReturnContextSecret: returnContextSecret,
	}, nil
}

func isLocalCallbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func WithPaymentConfig(cfg PaymentConfig) HandlerOption {
	return func(h *PaymentsHandler) {
		h.config = normalizePaymentConfig(cfg)
	}
}

func WithBookingGetter(fn func(context.Context, string) (*bookinghandlers.BookingResponse, error)) HandlerOption {
	return func(h *PaymentsHandler) {
		h.getBooking = fn
	}
}

func WithQuoteGetter(fn func(context.Context, string) (*quote.PaymentQuoteResponse, error)) HandlerOption {
	return func(h *PaymentsHandler) {
		h.getQuoteForPayment = fn
	}
}

func WithBookingPaymentPendingUpdater(fn func(context.Context, string, *booking.MarkPaymentPendingRequest) (*booking.BookingPaymentStatusResponse, error)) HandlerOption {
	return func(h *PaymentsHandler) {
		h.markBookingPaymentPending = fn
	}
}

func WithBookingPaymentReservationReleaser(fn func(context.Context, string, *booking.ReleasePaymentReservationRequest) (*booking.BookingPaymentStatusResponse, error)) HandlerOption {
	return func(h *PaymentsHandler) {
		h.releaseBookingPaymentReservation = fn
	}
}

func WithUserIDProvider(fn func() (string, bool)) HandlerOption {
	return func(h *PaymentsHandler) {
		h.currentUserID = fn
	}
}

func normalizePaymentConfig(cfg PaymentConfig) PaymentConfig {
	if cfg.ReturnContextTTL <= 0 {
		cfg.ReturnContextTTL = defaultReturnContextTTL
	}
	return cfg
}

func defaultUserIDProvider() (string, bool) {
	userID, ok := auth.UserID()
	if !ok {
		return "", false
	}
	return string(userID), true
}

func buildCheckoutReturnURL(baseURL, token string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	return fmt.Sprintf("%s/checkout/complete?ctx=%s", trimmed, token)
}

func buildAppReturnURL(baseURL string, payload callbackResponse, contextExpired bool) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}

	query := parsed.Query()
	query.Set("transaction_id", payload.TransactionID)
	if payload.AttemptID != "" {
		query.Set("attempt_id", payload.AttemptID)
	}
	query.Set("booking_id", payload.BookingID)
	query.Set("quote_id", payload.QuoteID)
	query.Set("reference", payload.Reference)
	query.Set("return_type", payload.ReturnType)
	if contextExpired {
		query.Set("ctx_status", "expired")
	} else {
		query.Set("ctx_status", "ok")
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

type returnContext struct {
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

var errReturnContextExpired = errors.New("return context expired")

func encodeReturnContext(payload returnContext, secret string) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal return context: %w", err)
	}

	sig := signHMACSHA256(body, []byte(secret))
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func decodeReturnContext(token, secret string, now time.Time) (*returnContext, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid return context format")
	}

	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode return context payload: %w", err)
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode return context signature: %w", err)
	}

	expected := signHMACSHA256(body, []byte(secret))
	if !hmacEqual(signature, expected) {
		return nil, fmt.Errorf("invalid return context signature")
	}

	var payload returnContext
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal return context payload: %w", err)
	}

	if payload.ExpiresAt == 0 {
		return nil, fmt.Errorf("return context expired")
	}
	if now.Unix() > payload.ExpiresAt {
		return &payload, errReturnContextExpired
	}

	return &payload, nil
}
