package repository

import (
	"context"

	"encore.app/booking/domain"
	"encore.app/core"
	"gorm.io/gorm"
)

// BookingRepository is an alias for the domain interface
type BookingRepository = domain.BookingRepository

// WithTransaction executes a function within a database transaction.
func (r *bookingRepository) WithTransaction(ctx context.Context, fn func(BookingRepository) error) error {
	// COORDINATED TRANSACTIONS: Since Outbox and Booking data live in different databases,
	// we nest their transactions. This ensures that if the business logic or the booking
	// commit fails, the outbox event is also rolled back.
	return r.coreDB.WithContext(ctx).Transaction(func(coreTx *gorm.DB) error {
		return core.WithTransaction(ctx, r.db, func(dbTx *gorm.DB) domain.BookingRepository {
			return &bookingRepository{
				db:     dbTx,
				coreDB: coreTx,
			}
		}, fn)
	})
}

// WithReadTransaction executes a function within a read-only transaction.
func (r *bookingRepository) WithReadTransaction(ctx context.Context, fn func(BookingRepository) error) error {
	return r.coreDB.WithContext(ctx).Transaction(func(coreTx *gorm.DB) error {
		return core.WithReadTransaction(ctx, r.db, func(dbTx *gorm.DB) domain.BookingRepository {
			return &bookingRepository{
				db:     dbTx,
				coreDB: coreTx,
			}
		}, fn)
	})
}

// GetDB returns the underlying database connection.
func (r *bookingRepository) GetDB() *gorm.DB {
	return r.db
}
