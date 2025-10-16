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

	// WithTransaction executes a function within a database transaction.
	WithTransaction(ctx context.Context, fn func(BookingRepository) error) error

	// WithReadTransaction executes a function within a read-only transaction.
	WithReadTransaction(ctx context.Context, fn func(BookingRepository) error) error

	// GetDB returns the underlying database connection.
	GetDB() *gorm.DB
}
