package domain

import (
	"context"
	"errors"
	"fmt"

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
	GetOffersByArtisanID(ctx context.Context, artisanID string, status BookingOfferStatus) ([]*BookingOffer, error)
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
}
