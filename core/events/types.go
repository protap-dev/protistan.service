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

// BookingEvent represents a booking state change
// This is shared across services so they all speak the same language
type BookingEvent struct {
	BookingID      string            `json:"booking_id"`
	QuoteID        string            `json:"quote_id,omitempty"`
	PaymentID      string            `json:"payment_id,omitempty"`
	Amount         float64           `json:"amount,omitempty"`
	Status         string            `json:"status"`
	PreviousStatus string            `json:"previous_status"`
	Timestamp      time.Time         `json:"timestamp"`
	UserID         string            `json:"user_id"`
	ArtisanID      *string           `json:"artisan_id,omitempty"`
	Reason         *string           `json:"reason,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// Helper to generate IDs
func GenerateEventID() string {
	return time.Now().Format("20060102150405") + "-" + randomString(8)
}

func randomString(_ int) string {
	// Simple implementation
	return fmt.Sprintf("%d", time.Now().UnixNano()%10000000)
}
