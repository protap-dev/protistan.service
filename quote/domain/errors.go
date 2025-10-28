package domain

import "fmt"

// QuoteError represents a domain error
type QuoteError struct {
	Code    string
	Message string
	Details map[string]interface{}
}

func (e *QuoteError) Error() string {
	return e.Message
}

// Predefined errors
var (
	ErrQuoteNotFound        = &QuoteError{Code: "quote_not_found", Message: "quote not found"}
	ErrInvalidState         = &QuoteError{Code: "invalid_state", Message: "invalid quote state"}
	ErrInvalidTransition    = &QuoteError{Code: "invalid_transition", Message: "invalid state transition"}
	ErrQuoteExpired         = &QuoteError{Code: "quote_expired", Message: "quote has expired"}
	ErrQuoteAlreadyAccepted = &QuoteError{Code: "quote_already_accepted", Message: "quote already accepted"}
	ErrBreakdownMismatch    = &QuoteError{Code: "breakdown_mismatch", Message: "breakdown total does not match quote amount"}
	ErrInvalidAmount        = &QuoteError{Code: "invalid_amount", Message: "quote amount must be positive"}
	ErrInvalidBreakdownType = &QuoteError{Code: "invalid_breakdown_type", Message: "invalid breakdown item type"}
)

// ErrOptimisticLockFailure represents optimistic locking failure
type ErrOptimisticLockFailure struct {
	EntityID string
}

func (e *ErrOptimisticLockFailure) Error() string {
	return fmt.Sprintf("optimistic lock failure for quote: %s", e.EntityID)
}
