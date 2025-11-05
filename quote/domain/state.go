package domain

// QuoteState represents the state of a quote
type QuoteState string

const (
	QuoteProposed   QuoteState = "proposed"   // Sent to customer
	QuoteAccepted   QuoteState = "accepted"   // Customer accepted
	QuoteRejected   QuoteState = "rejected"   // Customer rejected
	QuoteExpired    QuoteState = "expired"    // Passed expiration time
	QuoteSuperseded QuoteState = "superseded" // Quote superseded
)

// Valid state transitions map
var validTransitions = map[QuoteState]map[QuoteState]bool{
	QuoteProposed: {
		QuoteAccepted:   true,
		QuoteRejected:   true,
		QuoteExpired:    true,
		QuoteSuperseded: true,
	},
	QuoteAccepted:   {}, // Terminal
	QuoteRejected:   {}, // Terminal
	QuoteExpired:    {}, // Terminal
	QuoteSuperseded: {}, // Terminal
}

// CanTransition checks if a state transition is valid
func CanTransition(from, to QuoteState) bool {
	transitions, exists := validTransitions[from]
	if !exists {
		return false
	}
	return transitions[to]
}

// IsTerminal checks if the state is terminal
func (qs QuoteState) IsTerminal() bool {
	return qs == QuoteAccepted || qs == QuoteRejected || qs == QuoteExpired || qs == QuoteSuperseded
}

// RejectionReasonCode represents standardized rejection reasons
type RejectionReasonCode string

const (
	RejectionPriceTooHigh     RejectionReasonCode = "price_too_high"
	RejectionTimelineMismatch RejectionReasonCode = "timeline_mismatch"
	RejectionFoundAlternative RejectionReasonCode = "found_alternative"
	RejectionNoLongerNeeded   RejectionReasonCode = "no_longer_needed"
	RejectionOther            RejectionReasonCode = "other"
)
