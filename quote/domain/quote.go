package domain

import (
	"context"
	"time"

	eventscommon "encore.app/core/events"
)

// Quote represents a price quote for a booking
type Quote struct {
	// Identity
	ID        string `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	BookingID string `json:"booking_id" gorm:"type:uuid;not null;index"`
	Version   int    `json:"version" gorm:"not null;default:1"`

	// State
	State QuoteState `json:"state" gorm:"type:text;not null"`

	// Pricing
	AmountCents int64  `json:"amount_cents" gorm:"not null"`
	Currency    string `json:"currency" gorm:"type:text;not null;default:'NGN'"`

	// Metadata
	Notes                 string     `json:"notes,omitempty" gorm:"type:text"`
	EstimatedDurationMins int        `json:"estimated_duration_mins,omitempty"`
	ValidUntil            *time.Time `json:"valid_until,omitempty"`

	// Actor tracking
	ProposedBy string    `json:"proposed_by" gorm:"type:uuid;not null;index"` // Artisan ID
	ProposedAt time.Time `json:"proposed_at" gorm:"not null"`

	// Decision tracking
	DecisionBy          *string    `json:"decision_by,omitempty" gorm:"type:uuid"`
	DecidedAt           *time.Time `json:"decided_at,omitempty"`
	RejectionReasonCode *string    `json:"rejection_reason_code,omitempty"`
	RejectionReasonText *string    `json:"rejection_reason_text,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at" gorm:"not null"`
	UpdatedAt time.Time `json:"updated_at" gorm:"not null"`

	// Optimistic locking
	DBVersion int64 `json:"db_version" gorm:"column:db_version;not null;default:1"`
}

// IsExpired checks if quote has expired
func (q *Quote) IsExpired() bool {
	if q.ValidUntil == nil {
		return false
	}
	return time.Now().After(*q.ValidUntil)
}

// CanTransitionTo checks if quote can transition to target state
func (q *Quote) CanTransitionTo(target QuoteState) bool {
	return CanTransition(q.State, target)
}

// QuoteEvent represents a quote state change event
type QuoteEvent = eventscommon.QuoteEvent

// OutboxEvent represents event in outbox table
type OutboxEvent struct {
	ID          int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	Topic       string     `json:"topic" gorm:"type:text;not null"`
	Data        []byte     `json:"data" gorm:"type:jsonb;not null"`
	InsertedAt  time.Time  `json:"inserted_at" gorm:"not null"`
	ProcessedAt *time.Time `json:"processed_at"`
	RetryCount  int        `json:"retry_count" gorm:"default:0"`
	LastError   *string    `json:"last_error,omitempty" gorm:"type:text"`
}

// QuoteRepository defines repository interface
type QuoteRepository interface {
	// Basic CRUD
	Create(ctx context.Context, quote *Quote) error
	Update(ctx context.Context, quote *Quote) error
	GetByID(ctx context.Context, id string) (*Quote, error)
	GetByIDForUpdate(ctx context.Context, id string) (*Quote, error)
	GetByBookingID(ctx context.Context, bookingID string) ([]*Quote, error)

	// Query operations
	FindExpiredQuotes(ctx context.Context) ([]*Quote, error)

	// Transaction support
	WithTransaction(ctx context.Context, fn func(txRepo QuoteRepository) error) error

	// Outbox
	CreateEventInOutbox(ctx context.Context, event *QuoteEvent) error
}
