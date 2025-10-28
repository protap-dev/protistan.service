package domain

import (
	"context"
	"time"
)

// Validator validates quote business rules
type Validator struct{}

// NewQuoteValidator creates a new validator
func NewQuoteValidator() *Validator {
	return &Validator{}
}

// ValidateProposeQuote validates quote proposal
func (v *Validator) ValidateProposeQuote(ctx context.Context, input *ProposeQuoteInput) error {
	if input.BookingID == "" {
		return &QuoteError{Code: "invalid_input", Message: "booking_id is required"}
	}

	if input.AmountCents <= 0 {
		return ErrInvalidAmount
	}

	if input.ProposedBy == "" {
		return &QuoteError{Code: "invalid_input", Message: "proposed_by is required"}
	}

	if input.EstimatedDurationMins < 0 {
		return &QuoteError{Code: "invalid_input", Message: "estimated_duration_mins cannot be negative"}
	}

	if input.ValidUntil != nil && input.ValidUntil.Before(time.Now()) {
		return &QuoteError{Code: "invalid_input", Message: "valid_until must be in the future"}
	}

	return nil
}

// ValidateAcceptQuote validates quote acceptance
func (v *Validator) ValidateAcceptQuote(ctx context.Context, quote *Quote, decisionBy string) error {
	if quote.State != QuoteProposed {
		return &QuoteError{Code: "invalid_state", Message: "can only accept quotes in proposed state"}
	}

	if quote.IsExpired() {
		return ErrQuoteExpired
	}

	if decisionBy == "" {
		return &QuoteError{Code: "invalid_input", Message: "decision_by is required"}
	}

	return nil
}

// ValidateRejectQuote validates quote rejection
func (v *Validator) ValidateRejectQuote(ctx context.Context, quote *Quote, input *RejectQuoteInput) error {
	if quote.State != QuoteProposed {
		return &QuoteError{Code: "invalid_state", Message: "can only reject quotes in proposed state"}
	}

	if input.DecisionBy == "" {
		return &QuoteError{Code: "invalid_input", Message: "decision_by is required"}
	}

	if input.ReasonCode == "" {
		return &QuoteError{Code: "invalid_input", Message: "rejection reason_code is required"}
	}

	return nil
}

// Input types for validation
type ProposeQuoteInput struct {
	BookingID             string
	AmountCents           int64
	Currency              string
	EstimatedDurationMins int
	ValidUntil            *time.Time
	Notes                 string
	ProposedBy            string // Artisan ID
	Breakdown             []BreakdownItem
}

type RejectQuoteInput struct {
	DecisionBy string
	ReasonCode string
	ReasonText *string
}
