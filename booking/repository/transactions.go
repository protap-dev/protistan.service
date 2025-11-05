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
	return core.WithTransaction(ctx, r.db, func(db *gorm.DB) domain.BookingRepository {
		coreTx := r.coreDB.WithContext(ctx).Begin()

		return &bookingRepository{
			db:     db,
			coreDB: coreTx,
		}
	}, fn)
}

// WithReadTransaction executes a function within a read-only transaction.
func (r *bookingRepository) WithReadTransaction(ctx context.Context, fn func(BookingRepository) error) error {
	return core.WithReadTransaction(ctx, r.db, func(db *gorm.DB) domain.BookingRepository {
		coreTx := r.coreDB.WithContext(ctx).Begin()

		return &bookingRepository{
			db:     db,
			coreDB: coreTx,
		}
	}, fn)
}

// GetDB returns the underlying database connection.
func (r *bookingRepository) GetDB() *gorm.DB {
	return r.db
}
