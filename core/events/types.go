package eventscommon

import (
	"fmt"
	"time"
)

// EventEnvelope wraps events with observability metadata
// This is the CANONICAL event envelope structure used by ALL services
type EventEnvelope[T any] struct {
	EventID       string    `json:"event_id"`
	EventType     string    `json:"event_type"`
	OccurredAt    time.Time `json:"occurred_at"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	CausationID   string    `json:"causation_id,omitempty"`
	Producer      string    `json:"producer"`
	Data          T         `json:"data"`
}

type QuoteEvent struct {
	QuoteID               string     `json:"quote_id"`
	BookingID             string     `json:"booking_id"`
	Version               int        `json:"version"`
	State                 string     `json:"state"`
	PreviousState         string     `json:"previous_state"`
	AmountCents           int64      `json:"amount_cents"`
	Currency              string     `json:"currency"`
	Breakdown             []byte     `json:"breakdown,omitempty"`
	EstimatedDurationMins int        `json:"estimated_duration_mins,omitempty"`
	Notes                 string     `json:"notes,omitempty"`
	ValidUntil            *time.Time `json:"valid_until,omitempty"`
	ProposedBy            string     `json:"proposed_by"`
	DecisionBy            *string    `json:"decision_by,omitempty"`
	Timestamp             time.Time  `json:"timestamp"`
	UserID                string     `json:"user_id"`
	RejectionReasonCode   *string    `json:"rejection_reason_code,omitempty"`
}

// Helper to generate IDs
func GenerateEventID() string {
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

func randomString(_ int) string {
	// Simple implementation
	return fmt.Sprintf("%d", time.Now().UnixNano()%10000000)
}
