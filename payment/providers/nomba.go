package providers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"encore.app/core/cache"
	"encore.app/payment/domain"
)

// Nomba API endpoints.
const (
	EndpointTokenIssue         = "/v1/auth/token/issue"
	EndpointCheckoutProduction = "/v1/checkout/order"
	EndpointCheckoutCancel     = "/v1/checkout/order/cancel"
	EndpointTransaction        = "/v1/transactions/accounts/single"
	EndpointAccountFilter      = "/v1/transactions/accounts/filter"
	EndpointCheckoutFetchLive  = "/v1/checkout/transaction"
	EndpointCheckoutFetchTest  = "/sandbox/checkout/transaction"

	CacheKeyToken = "nomba_access_token"
	TokenTTL      = 25 * time.Minute // 5min buffer on 30min expiry

	maxNombaErrorBodyBytes = 4096
	maxWebhookClockSkew    = 5 * time.Minute
)

// Endpoint selection is environment-specific per Nomba's documentation and
// observed runtime behavior as of April 28, 2026:
//   - sandbox auth uses the sandbox host with `/v1/auth/token/issue`
//   - transaction verification remains `/v1/transactions/accounts/single`
//   - sandbox fetch transaction uses `/sandbox/checkout/transaction`
//   - sandbox create-order is currently forced to `/v1/checkout/order`
//     because a live sandbox request to `/sandbox/checkout/order` returned
//     HTTP 404 in this integration environment, despite the sandbox guide
//     documenting that path. Recheck this against Nomba periodically.

type nombaEnvironment string

const (
	nombaEnvironmentSandbox    nombaEnvironment = "sandbox"
	nombaEnvironmentProduction nombaEnvironment = "production"
)

// NombaProvider implements domain.PaymentProvider for Nomba Checkout.
type NombaProvider struct {
	environment  nombaEnvironment
	cache        cache.CacheManager
	baseURL      string
	clientID     string
	clientSecret string
	signatureKey string
	accountID    string
	httpClient   *http.Client
}

func NewNombaProvider(cache cache.CacheManager, baseURL, clientID, clientSecret, signatureKey, accountID string) (*NombaProvider, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("nomba API URL is required")
	}
	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil || parsedBaseURL.Scheme == "" || parsedBaseURL.Host == "" {
		return nil, fmt.Errorf("nomba API URL must be an absolute URL")
	}
	if parsedBaseURL.Scheme != "https" {
		if parsedBaseURL.Scheme != "http" || !isLocalNombaAPIHost(parsedBaseURL.Hostname()) {
			return nil, fmt.Errorf("nomba API URL must use https")
		}
	}

	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	signatureKey = strings.TrimSpace(signatureKey)
	accountID = strings.TrimSpace(accountID)
	if clientID == "" {
		return nil, fmt.Errorf("nomba client ID is required")
	}
	if clientSecret == "" {
		return nil, fmt.Errorf("nomba client secret is required")
	}
	if signatureKey == "" {
		return nil, fmt.Errorf("nomba signature key is required")
	}
	if accountID == "" {
		return nil, fmt.Errorf("nomba account ID is required")
	}

	baseURL = normalizeNombaBaseURL(baseURL)
	return &NombaProvider{
		environment:  detectNombaEnvironment(baseURL),
		cache:        cache,
		baseURL:      baseURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		signatureKey: signatureKey,
		accountID:    accountID,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func normalizeNombaBaseURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimRight(raw, "/")
	}

	return parsed.Scheme + "://" + parsed.Host
}

func isLocalNombaAPIHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (p *NombaProvider) Identifier() string { return "nomba" }

// FetchValidToken returns a cached token or issues a new one.
func (p *NombaProvider) FetchValidToken(ctx context.Context) (string, error) {
	val, err := p.cache.GetOrLoad(ctx, CacheKeyToken, TokenTTL, func() (any, error) {
		return p.issueToken(ctx)
	})
	if err != nil {
		return "", fmt.Errorf("failed to fetch nomba token: %w", err)
	}
	token, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("nomba token cache returned unexpected type")
	}
	return token, nil
}

func detectNombaEnvironment(baseURL string) nombaEnvironment {
	if strings.Contains(strings.ToLower(baseURL), "sandbox.nomba.com") {
		return nombaEnvironmentSandbox
	}
	return nombaEnvironmentProduction
}

// ---------------------------------------------------------------------------
// Internal DTOs
// ---------------------------------------------------------------------------

type tokenReq struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	GrantType    string `json:"grant_type"`
}

type tokenRes struct {
	Data struct {
		AccessToken string `json:"access_token"`
	} `json:"data"`
}

type checkoutReq struct {
	Order struct {
		OrderReference        string   `json:"orderReference"`
		CustomerEmail         string   `json:"customerEmail"`
		Amount                float64  `json:"amount"`
		Currency              string   `json:"currency"`
		CallbackURL           string   `json:"callbackUrl"`
		CustomerID            string   `json:"customerId,omitempty"`
		AccountID             string   `json:"accountId,omitempty"`
		AllowedPaymentMethods []string `json:"allowedPaymentMethods,omitempty"`
	} `json:"order"`
}

type checkoutRes struct {
	Data struct {
		OrderReference string `json:"orderReference"`
		CheckoutLink   string `json:"checkoutLink"`
	} `json:"data"`
}

type cancelReq struct {
	OrderReference string `json:"orderReference"`
}

type cancelRes struct {
	Data struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	} `json:"data"`
}

type verifyRes struct {
	Data struct {
		TransactionID  string      `json:"transactionId"`
		Status         string      `json:"status"`
		Amount         json.Number `json:"amount"`
		Currency       string      `json:"currency"`
		OrderReference string      `json:"orderReference"`
	} `json:"data"`
}

type reconcileReq struct {
	AccountID string `json:"accountId"`
	Cursor    string `json:"cursor,omitempty"`
}

type reconcileRes struct {
	Data struct {
		Cursor struct {
			Next string `json:"next"`
		} `json:"cursor"`
	} `json:"data"`
}

type webhookPayload struct {
	EventType string `json:"event_type"`
	RequestID string `json:"requestId"`
	Timestamp string `json:"timestamp"`
	Data      struct {
		Merchant struct {
			UserID   string `json:"userId"`
			WalletID string `json:"walletId"`
		} `json:"merchant"`
		Transaction struct {
			TransactionID string  `json:"transactionId"`
			Type          string  `json:"type"`
			Time          string  `json:"time"`
			ResponseCode  string  `json:"responseCode"`
			Amount        float64 `json:"transactionAmount"`
			MerchantTxRef string  `json:"merchantTxRef"`
		} `json:"transaction"`
		Order struct {
			OrderID        string `json:"orderId"`
			OrderReference string `json:"orderReference"`
			AccountID      string `json:"accountId"`
			CustomerEmail  string `json:"customerEmail"`
			PaymentMethod  string `json:"paymentMethod"`
			Currency       string `json:"currency"`
		} `json:"order"`
	} `json:"data"`
}

// ---------------------------------------------------------------------------
// HTTP transport
// ---------------------------------------------------------------------------

// doRequest is a shared helper for JSON API calls against the requested base URL.
func (p *NombaProvider) doRequest(ctx context.Context, method, baseURL, endpoint string, reqBody any, headers map[string]string, resBody any) error {
	var body io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(b)
	}

	requestURL := strings.TrimRight(baseURL, "/") + endpoint
	httpReq, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		httpReq.Header[k] = []string{v} // preserve exact casing (e.g. "accountId")
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("nomba request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		drainLimited(resp.Body)
		return fmt.Errorf("nomba %s %s status %d", method, requestURL, resp.StatusCode)
	}

	if resBody != nil {
		if err := json.NewDecoder(resp.Body).Decode(resBody); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Core implementations
// ---------------------------------------------------------------------------

// issueToken obtains a fresh access token from the environment-specific auth gateway.
func (p *NombaProvider) issueToken(ctx context.Context) (string, error) {
	body, err := json.Marshal(tokenReq{
		ClientID:     p.clientID,
		ClientSecret: p.clientSecret,
		GrantType:    "client_credentials",
	})
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.authURL(), bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header["accountId"] = []string{p.accountID}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		drainLimited(resp.Body)
		return "", fmt.Errorf("nomba POST %s status %d", p.authURL(), resp.StatusCode)
	}

	var res tokenRes
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	if res.Data.AccessToken == "" {
		return "", fmt.Errorf("nomba returned empty access token")
	}
	return res.Data.AccessToken, nil
}

func (p *NombaProvider) Initialize(ctx context.Context, req *domain.InitializationRequest) (*domain.InitializationResponse, error) {
	token, err := p.FetchValidToken(ctx)
	if err != nil {
		return nil, err
	}

	account := req.AccountID
	if account == "" || account == "default" {
		account = p.accountID
	}

	apiReq := checkoutReq{}
	apiReq.Order.OrderReference = req.OrderReference
	apiReq.Order.Amount = req.Amount
	if apiReq.Order.Amount <= 0 && req.AmountCents > 0 {
		apiReq.Order.Amount = float64(req.AmountCents) / 100.0
	}
	apiReq.Order.Currency = req.Currency
	apiReq.Order.CustomerEmail = req.CustomerEmail
	apiReq.Order.CallbackURL = req.CallbackURL
	apiReq.Order.AccountID = account
	apiReq.Order.AllowedPaymentMethods = req.AllowedPaymentMethods

	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"accountId":     account,
	}

	var apiRes checkoutRes
	if err := p.doRequest(ctx, http.MethodPost, p.baseURL, p.checkoutOrderPath(), apiReq, headers, &apiRes); err != nil {
		return nil, err
	}

	return &domain.InitializationResponse{
		CheckoutLink: apiRes.Data.CheckoutLink,
		OrderRef:     apiRes.Data.OrderReference,
	}, nil
}

func (p *NombaProvider) Verify(ctx context.Context, reference string) (*domain.VerificationResponse, error) {
	token, err := p.FetchValidToken(ctx)
	if err != nil {
		return nil, err
	}

	endpoint := p.verifyTransactionPath(reference)
	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"accountId":     p.accountID,
	}

	var res verifyRes
	if err := p.doRequest(ctx, http.MethodGet, p.baseURL, endpoint, nil, headers, &res); err != nil {
		return nil, err
	}

	amount, err := res.Data.Amount.Float64()
	if err != nil {
		return nil, fmt.Errorf("parse nomba transaction amount: %w", err)
	}
	return &domain.VerificationResponse{
		Status:        res.Data.Status,
		TransactionID: res.Data.TransactionID,
		AmountCents:   int64(math.Round(amount * 100)),
		Amount:        amount,
		Currency:      res.Data.Currency,
		OrderRef:      res.Data.OrderReference,
	}, nil
}

func (p *NombaProvider) Cancel(ctx context.Context, reference string) error {
	token, err := p.FetchValidToken(ctx)
	if err != nil {
		return err
	}

	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"accountId":     p.accountID,
	}

	var res cancelRes
	if err := p.doRequest(ctx, http.MethodPost, p.baseURL, EndpointCheckoutCancel, cancelReq{OrderReference: reference}, headers, &res); err != nil {
		return err
	}
	if !res.Data.Success {
		return fmt.Errorf("nomba cancel checkout order reported unsuccessful for %s", reference)
	}
	return nil
}

func (p *NombaProvider) Reconcile(ctx context.Context, accountID, cursor string) (string, error) {
	token, err := p.FetchValidToken(ctx)
	if err != nil {
		return "", err
	}

	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"accountId":     accountID,
	}

	var apiRes reconcileRes
	if err := p.doRequest(ctx, http.MethodPost, p.baseURL, EndpointAccountFilter, reconcileReq{AccountID: accountID, Cursor: cursor}, headers, &apiRes); err != nil {
		return "", err
	}
	return apiRes.Data.Cursor.Next, nil
}

func (p *NombaProvider) authURL() string {
	return p.baseURL + EndpointTokenIssue
}

func (p *NombaProvider) checkoutOrderPath() string {
	return EndpointCheckoutProduction
}

func (p *NombaProvider) verifyTransactionPath(reference string) string {
	return fmt.Sprintf("%s?orderReference=%s", EndpointTransaction, url.QueryEscape(reference))
}

func (p *NombaProvider) checkoutTransactionPath() string {
	if p.environment == nombaEnvironmentSandbox {
		return EndpointCheckoutFetchTest
	}
	return EndpointCheckoutFetchLive
}

// ParseWebhook decodes a Nomba webhook, verifies the HMAC signature, and returns a normalized event.
// Nomba's real checkout webhook nests fields under data.merchant, data.transaction, and data.order.
// The HMAC sequence is: eventType:requestId:userId:walletId:transactionId:type:time:responseCode:timestamp
// The signature is Base64-encoded HMAC-SHA256.
func (p *NombaProvider) ParseWebhook(ctx context.Context, req *http.Request) (*domain.WebhookEvent, error) {
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("read webhook body: %w", err)
	}

	signature := req.Header.Get("nomba-signature")
	if signature == "" {
		return nil, fmt.Errorf("missing nomba-signature header")
	}

	timestamp := req.Header.Get("nomba-timestamp")
	if timestamp == "" {
		return nil, fmt.Errorf("missing nomba-timestamp header")
	}
	receivedAt, err := parseNombaWebhookTimestamp(timestamp)
	if err != nil {
		return nil, err
	}
	if skew := time.Since(receivedAt); skew > maxWebhookClockSkew || skew < -maxWebhookClockSkew {
		return nil, fmt.Errorf("webhook timestamp outside accepted clock skew")
	}

	var parsed webhookPayload
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal webhook struct: %w", err)
	}
	if parsed.RequestID == "" {
		return nil, fmt.Errorf("missing webhook request id")
	}

	// Build the HMAC sequence string from the correct nested paths.
	// Format: eventType:requestId:userId:walletId:transactionId:type:time:responseCode:timestamp
	seq := strings.Join([]string{
		parsed.EventType,
		parsed.RequestID,
		parsed.Data.Merchant.UserID,
		parsed.Data.Merchant.WalletID,
		parsed.Data.Transaction.TransactionID,
		parsed.Data.Transaction.Type,
		parsed.Data.Transaction.Time,
		parsed.Data.Transaction.ResponseCode,
		timestamp,
	}, ":")

	if !p.verifyHMAC(seq, signature) {
		return nil, fmt.Errorf("webhook signature verification failed")
	}

	orderRef := parsed.Data.Order.OrderReference
	if orderRef == "" {
		return nil, fmt.Errorf("missing webhook order reference")
	}
	status := resolveWebhookStatus(parsed.EventType)

	return &domain.WebhookEvent{
		EventType:     parsed.EventType,
		TransactionID: parsed.Data.Transaction.TransactionID,
		OrderRef:      orderRef,
		RequestID:     parsed.RequestID,
		ReceivedAt:    receivedAt,
		Status:        status,
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// resolveWebhookStatus maps Nomba event type strings to normalized statuses.
func resolveWebhookStatus(eventType string) string {
	lower := strings.ToLower(eventType)
	switch {
	case strings.Contains(lower, "success"):
		return "SUCCESS"
	case strings.Contains(lower, "fail"), strings.Contains(lower, "cancel"):
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

// verifyHMAC checks the HMAC-SHA256 signature using timing-safe comparison.
func (p *NombaProvider) verifyHMAC(message, signature string) bool {
	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(p.signatureKey))
	mac.Write([]byte(message))
	expected := mac.Sum(nil)
	return hmac.Equal(sigBytes, expected)
}

func parseNombaWebhookTimestamp(raw string) (time.Time, error) {
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed, nil
	}
	if unix, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if unix > 1_000_000_000_000 {
			return time.UnixMilli(unix), nil
		}
		return time.Unix(unix, 0), nil
	}
	return time.Time{}, fmt.Errorf("invalid webhook timestamp")
}

func drainLimited(body io.Reader) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxNombaErrorBodyBytes))
}
