package handlers

import (
	"context"

	"encore.dev/beta/errs"
)

// PaymentQuoteResponse is a payment-safe quote projection for internal callers.
type PaymentQuoteResponse struct {
	ID          string `json:"id"`
	BookingID   string `json:"booking_id"`
	State       string `json:"state"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
}

// GetQuoteForPayment returns the minimal quote data needed to initialize payment.
func (h *QuotesHandler) GetQuoteForPayment(ctx context.Context, id string) (*PaymentQuoteResponse, error) {
	quote, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, errs.B().Code(errs.NotFound).Msg("quote not found").Err()
	}

	return &PaymentQuoteResponse{
		ID:          quote.ID,
		BookingID:   quote.BookingID,
		State:       string(quote.State),
		AmountCents: quote.AmountCents,
		Currency:    quote.Currency,
	}, nil
}
