package domain

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

var (
	ErrNotFound = errors.New("artisan not found")
)

// ArtisanRepository defines data access operations
// ArtisanRepository handles artisan profile data operations
type ArtisanRepository interface {
	Create(ctx context.Context, profile *ArtisanProfile) error
	GetByUserID(ctx context.Context, userID string) (*ArtisanProfile, error)
	GetByID(ctx context.Context, id string) (*ArtisanProfile, error)
	Update(ctx context.Context, id string, updates map[string]any) error
	Delete(ctx context.Context, id string) error

	// WithTransaction executes a function within a database transaction
	WithTransaction(ctx context.Context, fn func(ArtisanRepository) error) error

	// WithReadTransaction executes a function within a read-only transaction for consistency
	WithReadTransaction(ctx context.Context, fn func(ArtisanRepository) error) error

	GetDB() *gorm.DB // Expose DB connection for complex queries
}

// RatesRepository handles artisan rates data operations
type RatesRepository interface {
	Create(ctx context.Context, rate *ArtisanRate) error
	GetByArtisanID(ctx context.Context, artisanID string) ([]ArtisanRate, error)
	GetByArtisanAndServiceID(ctx context.Context, artisanID, serviceID string) (*ArtisanRate, error)
	GetByID(ctx context.Context, id string) (*ArtisanRate, error)
	Update(ctx context.Context, id string, updates map[string]interface{}) error
	Delete(ctx context.Context, id string) error
	GetServiceByID(ctx context.Context, serviceID string) (*Service, error)
	GetCategoryByID(ctx context.Context, categoryID string) (*ServiceCategory, error)

	// Add transaction support for atomic operations
	WithTransaction(ctx context.Context, fn func(RatesRepository) error) error
	WithReadTransaction(ctx context.Context, fn func(RatesRepository) error) error
}
