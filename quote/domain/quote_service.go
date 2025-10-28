package domain

import (
	"context"
	"fmt"
	"time"
)

// QuoteService handles quote business logic
type QuoteService struct {
	repo      QuoteRepository
	validator *Validator
}

// NewQuoteService creates a new quote service
func NewQuoteService(repo QuoteRepository, validator *Validator) *QuoteService {
	return &QuoteService{
		repo:      repo,
		validator: validator,
	}
}

// ProposeQuote creates and proposes a new quote
func (s *QuoteService) ProposeQuote(ctx context.Context, input *ProposeQuoteInput) (*Quote, error) {
	// Validate
	if err := s.validator.ValidateProposeQuote(ctx, input); err != nil {
		return nil, err
	}

	// Default currency
	if input.Currency == "" {
		input.Currency = "NGN"
	}

	// Default expiration (48 hours)
	validUntil := input.ValidUntil
	if validUntil == nil {
		defaultExpiry := time.Now().Add(48 * time.Hour)
		validUntil = &defaultExpiry
	}

	// Get existing quotes to determine version
	existingQuotes, err := s.repo.GetByBookingID(ctx, input.BookingID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing quotes: %w", err)
	}

	version := 1
	for _, q := range existingQuotes {
		if q.Version >= version {
			version = q.Version + 1
		}
	}

	// Create quote
	now := time.Now()
	quote := &Quote{
		BookingID:             input.BookingID,
		Version:               version,
		State:                 QuoteProposed,
		AmountCents:           input.AmountCents,
		Currency:              input.Currency,
		Notes:                 input.Notes,
		EstimatedDurationMins: input.EstimatedDurationMins,
		ValidUntil:            validUntil,
		ProposedBy:            input.ProposedBy,
		ProposedAt:            now,
		CreatedAt:             now,
		UpdatedAt:             now,
		DBVersion:             1,
	}

	// Save in transaction
	err = s.repo.WithTransaction(ctx, func(txRepo QuoteRepository) error {
		if err := txRepo.Create(ctx, quote); err != nil {
			return err
		}

		// Create event
		event := &QuoteEvent{
			QuoteID:       quote.ID,
			BookingID:     quote.BookingID,
			Version:       quote.Version,
			State:         string(QuoteProposed),
			PreviousState: "",
			AmountCents:   quote.AmountCents,
			Currency:      quote.Currency,
			ProposedBy:    quote.ProposedBy,
			Timestamp:     now,
			UserID:        input.ProposedBy,
		}

		return txRepo.CreateEventInOutbox(ctx, event)
	})

	if err != nil {
		return nil, err
	}

	return quote, nil
}

// AcceptQuote marks quote as accepted
func (s *QuoteService) AcceptQuote(ctx context.Context, quoteID, decisionBy string) (*Quote, error) {
	var updatedQuote *Quote

	err := s.repo.WithTransaction(ctx, func(txRepo QuoteRepository) error {
		quote, err := txRepo.GetByID(ctx, quoteID)
		if err != nil {
			return err
		}

		if err := s.validator.ValidateAcceptQuote(ctx, quote, decisionBy); err != nil {
			return err
		}

		previousState := quote.State
		quote.State = QuoteAccepted
		quote.DecisionBy = &decisionBy
		now := time.Now()
		quote.DecidedAt = &now
		quote.UpdatedAt = now

		if err := txRepo.Update(ctx, quote); err != nil {
			return err
		}

		event := &QuoteEvent{
			QuoteID:       quote.ID,
			BookingID:     quote.BookingID,
			Version:       quote.Version,
			State:         string(QuoteAccepted),
			PreviousState: string(previousState),
			AmountCents:   quote.AmountCents,
			Currency:      quote.Currency,
			ProposedBy:    quote.ProposedBy,
			DecisionBy:    &decisionBy,
			Timestamp:     now,
			UserID:        decisionBy,
		}

		if err := txRepo.CreateEventInOutbox(ctx, event); err != nil {
			return err
		}

		updatedQuote = quote
		return nil
	})

	return updatedQuote, err
}

// RejectQuote marks quote as rejected
func (s *QuoteService) RejectQuote(ctx context.Context, quoteID string, input *RejectQuoteInput) (*Quote, error) {
	var updatedQuote *Quote

	err := s.repo.WithTransaction(ctx, func(txRepo QuoteRepository) error {
		quote, err := txRepo.GetByID(ctx, quoteID)
		if err != nil {
			return err
		}

		if err := s.validator.ValidateRejectQuote(ctx, quote, input); err != nil {
			return err
		}

		previousState := quote.State
		quote.State = QuoteRejected
		quote.DecisionBy = &input.DecisionBy
		now := time.Now()
		quote.DecidedAt = &now
		quote.RejectionReasonCode = &input.ReasonCode
		quote.RejectionReasonText = input.ReasonText
		quote.UpdatedAt = now

		if err := txRepo.Update(ctx, quote); err != nil {
			return err
		}

		event := &QuoteEvent{
			QuoteID:             quote.ID,
			BookingID:           quote.BookingID,
			Version:             quote.Version,
			State:               string(QuoteRejected),
			PreviousState:       string(previousState),
			AmountCents:         quote.AmountCents,
			Currency:            quote.Currency,
			ProposedBy:          quote.ProposedBy,
			DecisionBy:          &input.DecisionBy,
			RejectionReasonCode: &input.ReasonCode,
			Timestamp:           now,
			UserID:              input.DecisionBy,
		}

		if err := txRepo.CreateEventInOutbox(ctx, event); err != nil {
			return err
		}

		updatedQuote = quote
		return nil
	})

	return updatedQuote, err
}

// ExpireQuote marks quote as expired (called by cron)
func (s *QuoteService) ExpireQuote(ctx context.Context, quoteID string) error {
	return s.repo.WithTransaction(ctx, func(txRepo QuoteRepository) error {
		quote, err := txRepo.GetByID(ctx, quoteID)
		if err != nil {
			return err
		}

		// Idempotent - skip if not proposed
		if quote.State != QuoteProposed {
			return nil
		}

		// Double-check expiration
		if !quote.IsExpired() {
			return nil
		}

		previousState := quote.State
		quote.State = QuoteExpired
		quote.UpdatedAt = time.Now()

		if err := txRepo.Update(ctx, quote); err != nil {
			return err
		}

		event := &QuoteEvent{
			QuoteID:       quote.ID,
			BookingID:     quote.BookingID,
			Version:       quote.Version,
			State:         string(QuoteExpired),
			PreviousState: string(previousState),
			AmountCents:   quote.AmountCents,
			Currency:      quote.Currency,
			ProposedBy:    quote.ProposedBy,
			Timestamp:     time.Now(),
			UserID:        "system",
		}

		return txRepo.CreateEventInOutbox(ctx, event)
	})
}
