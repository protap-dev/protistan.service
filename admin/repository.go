package admin

import (
	"context"
	"encore.dev/beta/errs"
	"encore.app/core"
	"gorm.io/gorm"
)

// UserRepository handles user data operations for admin service
type UserRepository interface {
	GetByID(ctx context.Context, id string) (*User, error)
	UpdateCore(ctx context.Context, id string, updates map[string]interface{}) error
	WithTransaction(ctx context.Context, fn func(UserRepository) error) error
	WithReadTransaction(ctx context.Context, fn func(UserRepository) error) error
	GetDB() *gorm.DB
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) GetByID(ctx context.Context, id string) (*User, error) {
	var user User
	result := r.db.WithContext(ctx).Where("id = ?", id).First(&user)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, ErrUserNotFound
		}
		return nil, errs.B().Msg("failed to get user").Err()
	}
	return &user, nil
}

func (r *userRepository) UpdateCore(ctx context.Context, id string, updates map[string]interface{}) error {
	result := r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return errs.B().Msg("failed to update user core data").Err()
	}
	if result.RowsAffected == 0 {
		return ErrUserNotFound
	}
	return nil
}

// WithTransaction executes a function within a database transaction
func (r *userRepository) WithTransaction(ctx context.Context, fn func(UserRepository) error) error {
	return core.WithTransaction(ctx, r.db, func(db *gorm.DB) UserRepository {
		return &userRepository{db: db}
	}, fn)
}

// WithReadTransaction executes a function within a read-only transaction for consistency
func (r *userRepository) WithReadTransaction(ctx context.Context, fn func(UserRepository) error) error {
	return core.WithReadTransaction(ctx, r.db, func(db *gorm.DB) UserRepository {
		return &userRepository{db: db}
	}, fn)
}

// GetDB returns the underlying database connection for complex queries
func (r *userRepository) GetDB() *gorm.DB {
	return r.db
}
