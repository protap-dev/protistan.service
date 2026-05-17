package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"time"
)

type callbackResponse struct {
	TransactionID string `json:"transaction_id"`
	AttemptID     string `json:"attempt_id,omitempty"`
	BookingID     string `json:"booking_id"`
	QuoteID       string `json:"quote_id"`
	Reference     string `json:"reference"`
	ReturnType    string `json:"return_type"`
}

// HostedCheckoutCallback serves the public callback surface used for browser returns,
// and safely forwards POST callbacks to the provider webhook handler when needed.
func (h *PaymentsHandler) HostedCheckoutCallback(w http.ResponseWriter, req *http.Request, returnType string) {
	switch req.Method {
	case http.MethodGet:
		h.serveCheckoutReturn(w, req, returnType)
	case http.MethodPost:
		h.NombaWebhook(w, req)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *PaymentsHandler) serveCheckoutReturn(w http.ResponseWriter, req *http.Request, returnType string) {
	ctxToken := req.URL.Query().Get("ctx")
	if ctxToken == "" {
		http.Error(w, "missing ctx", http.StatusBadRequest)
		return
	}

	payload, err := decodeReturnContext(ctxToken, h.config.ReturnContextSecret, time.Now())
	if err != nil {
		if payload == nil || !errors.Is(err, errReturnContextExpired) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	contextExpired := errors.Is(err, errReturnContextExpired)

	if orderReference := req.URL.Query().Get("orderReference"); orderReference != "" && orderReference != payload.InternalRef {
		http.Error(w, "order reference mismatch", http.StatusBadRequest)
		return
	}

	response := callbackResponse{
		TransactionID: payload.TransactionID,
		AttemptID:     payload.AttemptID,
		BookingID:     payload.BookingID,
		QuoteID:       payload.QuoteID,
		Reference:     payload.InternalRef,
		ReturnType:    returnType,
	}
	appReturnURL := buildAppReturnURL(h.config.AppReturnURL, response, contextExpired)

	body, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "failed to render callback", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Protisan Payment Return</title>
  <meta http-equiv="refresh" content="0;url=%s">
</head>
<body>
  <h1>Return to Protisan</h1>
  <p>%s</p>
  <p><a href="%s">Open Protisan app</a></p>
  <noscript><p>If the app does not open automatically, use the button above.</p></noscript>
  <script>window.__PROTISAN_PAYMENT_RETURN__ = %s;</script>
  <script>
    (function () {
      var appURL = %q;
      if (!appURL) return;
      window.__PROTISAN_PAYMENT_APP_RETURN__ = appURL;
      window.location.replace(appURL);
      setTimeout(function () {
        window.location.href = appURL;
      }, 250);
    })();
  </script>
  <pre>%s</pre>
</body>
</html>`, html.EscapeString(appReturnURL), html.EscapeString(callbackCopy(contextExpired)), html.EscapeString(appReturnURL), string(body), appReturnURL, html.EscapeString(string(body)))
}

func callbackCopy(contextExpired bool) string {
	if contextExpired {
		return "Payment callback received. This session took longer than expected, but Protisan will verify the final payment status in the app."
	}
	return "Payment callback received. Opening Protisan so the app can verify the final payment status."
}
