package domain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

var (
	ErrBookingNotFound = errors.New("booking not found")
)

// ErrOptimisticLockFailure represents an optimistic locking failure.
// It's defined in the domain so that service layers can handle this specific error.
type ErrOptimisticLockFailure struct {
	BookingID string
}

func (e *ErrOptimisticLockFailure) Error() string {
	return fmt.Sprintf("optimistic lock failure for booking %s", e.BookingID)
}

// BookingRepository defines the persistence interface for booking aggregates.
type BookingRepository interface {
	Create(ctx context.Context, booking *Booking) error
	GetByID(ctx context.Context, id string) (*Booking, error)
	GetByCustomerID(ctx context.Context, customerID string) ([]*Booking, error)
	GetByArtisanID(ctx context.Context, artisanID string) ([]*Booking, error)
	Update(ctx context.Context, booking *Booking) error
	CreateStatusHistory(ctx context.Context, history *BookingEvent) error
	SoftDelete(ctx context.Context, id string) error

	// Offer management
	CreateOffer(ctx context.Context, offer *BookingOffer) error
	GetOfferByID(ctx context.Context, offerID string) (*BookingOffer, error)
	GetOffersByBookingID(ctx context.Context, bookingID string) ([]*BookingOffer, error)
	GetOffersByArtisanID(ctx context.Context, artisanID string, filter OfferFilter) ([]*BookingOffer, error)
	GetArtisanOffersWithDetails(ctx context.Context, artisanID string, filter OfferFilter) ([]*ArtisanOfferDetails, error)
	UpdateOfferStatus(ctx context.Context, offerID string, status BookingOfferStatus, reason *string) error
	CancelPendingOffers(ctx context.Context, bookingID string) error

	// WithTransaction executes a function within a database transaction.
	WithTransaction(ctx context.Context, fn func(BookingRepository) error) error

	// WithReadTransaction executes a function within a read-only transaction.
	WithReadTransaction(ctx context.Context, fn func(BookingRepository) error) error

	// GetDB returns the underlying database connection.
	GetDB() *gorm.DB

	// CreateEventInOutbox writes an event directly to the outbox table within a transaction.
	// This ensures events are published atomically with database changes.
	CreateEventInOutbox(ctx context.Context, event *BookingEvent) error

	// CreateRematchEventInOutbox writes a rematch event directly to the outbox table within a transaction.
	// This ensures rematch events are published atomically.
	CreateRematchEventInOutbox(ctx context.Context, event *RematchEvent) error

	// Offer methods
	FindExpiredOffers(ctx context.Context) ([]*BookingOffer, error)
	GetArtisanAvailability(ctx context.Context, artisanID string) (string, error)
	UpdateOffer(ctx context.Context, offer *BookingOffer) error
	CreateOfferExpiredEventInOutbox(ctx context.Context, event *BookingEvent) error

	// Idempotency key methods
	CheckIdempotencyKey(ctx context.Context, key string, userID string, requestHash string) (*IdempotencyRecord, error)
	StoreIdempotencyKey(ctx context.Context, key string, userID string, requestHash string, expiresAt time.Time) error
	CompleteIdempotencyKey(ctx context.Context, key string, bookingID string, response interface{}) error
}

// IdempotencyRecord represents the idempotency key storage (domain model)
type IdempotencyRecord struct {
	IdempotencyKey string
	UserID         string
	RequestHash    string
	BookingID      *string
	ResponseBody   []byte
	Status         string
	CreatedAt      time.Time
	CompletedAt    *time.Time
	ExpiresAt      time.Time
}
