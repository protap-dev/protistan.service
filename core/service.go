package core

import (
	"context"

	"encore.app/core/cache"
	"gorm.io/gorm"
)

// CoreService provides shared infrastructure services including database, cache and health monitor
type CoreService struct {
	db            *gorm.DB
	cache         cache.CacheManager
	healthMonitor *healthMonitor
}

// NewCoreService creates a new core service with database, cache and health monitor
func NewCoreService(db *gorm.DB) *CoreService {
	return &CoreService{
		db:            db,
		cache:         cache.NewInMemoryCache(),
		healthMonitor: newHealthMonitor(db, cache.NewInMemoryCache()),
	}
}

// DB returns the database connection
func (c *CoreService) DB() *gorm.DB {
	return c.db
}

// Cache returns the cache manager
func (c *CoreService) Cache() cache.CacheManager {
	return c.cache
}

// ============================================================================
//  BASIC ERROR TRANSLATION
// ============================================================================

// TranslateError converts GORM/database errors into standardized service errors
// This method provides convenient access to the error translation functionality
func (c *CoreService) TranslateError(err error) error {
	return TranslateError(err)
}

// NewServiceError creates a new service error with the given code and message
func (c *CoreService) NewServiceError(code, message, details string) *ServiceError {
	return NewServiceError(code, message, details)
}

// NewServiceErrorf creates a new service error with formatted message
func (c *CoreService) NewServiceErrorf(code, message string, args ...any) *ServiceError {
	return NewServiceErrorf(code, message, args...)
}

// ============================================================================
// GENERIC REPOSITORY TRANSACTIONS
// ============================================================================

// RepositoryFactory creates a new repository instance with the given database connection
type RepositoryFactory[T any] func(*gorm.DB) T

// WithTransaction executes a function within a database transaction using the provided repository factory
func WithTransaction[T any](
	ctx context.Context,
	db *gorm.DB,
	repoFactory RepositoryFactory[T],
	fn func(T) error,
) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Create repository instance using the transaction
		repo := repoFactory(tx)
		return fn(repo)
	})
}

// WithReadTransaction executes a function within a read-only transaction for consistency
func WithReadTransaction[T any](
	ctx context.Context,
	db *gorm.DB,
	repoFactory RepositoryFactory[T],
	fn func(T) error,
) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Create repository instance using the transaction
		repo := repoFactory(tx)
		return fn(repo)
	})
}
