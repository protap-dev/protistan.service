package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"encore.app/chat/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ThreadRepositoryImpl implements domain.ThreadRepository
type ThreadRepositoryImpl struct {
	db *gorm.DB
}

// NewThreadRepository creates a new thread repository
func NewThreadRepository(db *gorm.DB) domain.ThreadRepository {
	return &ThreadRepositoryImpl{db: db}
}

// Create creates a new thread
func (r *ThreadRepositoryImpl) Create(ctx context.Context, thread *domain.Thread) error {
	result := r.db.WithContext(ctx).Clauses(clause.Returning{}).Create(thread)
	if result.Error != nil {
		// Check for unique constraint violation
		if isDuplicateKeyError(result.Error) {
			return domain.ErrThreadAlreadyExists
		}
		return fmt.Errorf("failed to create thread: %w", result.Error)
	}
	return nil
}

// GetByID retrieves a thread by ID
func (r *ThreadRepositoryImpl) GetByID(ctx context.Context, id string) (*domain.Thread, error) {
	var thread domain.Thread
	result := r.db.WithContext(ctx).Where("id = ?", id).First(&thread)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, domain.ErrThreadNotFound
		}
		return nil, fmt.Errorf("failed to get thread: %w", result.Error)
	}
	return &thread, nil
}

// GetByBookingID retrieves a thread by booking ID
func (r *ThreadRepositoryImpl) GetByBookingID(ctx context.Context, bookingID string) (*domain.Thread, error) {
	var thread domain.Thread
	result := r.db.WithContext(ctx).Where("booking_id = ?", bookingID).First(&thread)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, domain.ErrThreadNotFound
		}
		return nil, fmt.Errorf("failed to get thread by booking: %w", result.Error)
	}
	return &thread, nil
}

// GetByParticipant retrieves threads for a participant with pagination
func (r *ThreadRepositoryImpl) GetByParticipant(ctx context.Context, userID string, limit, offset int) ([]*domain.Thread, error) {
	var threads []*domain.Thread
	result := r.db.WithContext(ctx).
		Where("customer_id = ? OR artisan_id = ?", userID, userID).
		Order("last_message_at DESC NULLS LAST, created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&threads)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get threads by participant: %w", result.Error)
	}
	return threads, nil
}

// Update updates a thread
func (r *ThreadRepositoryImpl) Update(ctx context.Context, thread *domain.Thread) error {
	result := r.db.WithContext(ctx).Save(thread)
	if result.Error != nil {
		return fmt.Errorf("failed to update thread: %w", result.Error)
	}
	return nil
}

// WithTransaction executes a function within a database transaction
func (r *ThreadRepositoryImpl) WithTransaction(ctx context.Context, fn func(txRepo domain.ThreadRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := &ThreadRepositoryImpl{db: tx}
		return fn(txRepo)
	})
}

// Helper to check for duplicate key errors
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate key") || strings.Contains(message, "unique constraint")
}
