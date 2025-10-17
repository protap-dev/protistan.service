package internal

import (
	"errors"

	"encore.app/booking/domain"
	"encore.dev/beta/errs"
	"gorm.io/gorm"
)

// Unified error definitions for the booking service
var (
	ErrUnauthenticated   = errs.B().Code(errs.Unauthenticated).Msg("not authenticated").Err()
	ErrPermissionDenied  = errs.B().Code(errs.PermissionDenied).Msg("permission denied").Err()
	ErrNotFound          = errs.B().Code(errs.NotFound).Msg("resource not found").Err()
	ErrDatabaseError     = errs.B().Code(errs.Internal).Msg("database error").Err()
	ErrValidationFailed  = errs.B().Code(errs.InvalidArgument).Msg("validation failed").Err()
	ErrInvalidInput      = errs.B().Code(errs.InvalidArgument).Msg("invalid input").Err()
	ErrInvalidPagination = errs.B().Code(errs.InvalidArgument).Msg("invalid pagination parameters").Err()
)

// ============================================================================
// CENTRALIZED ERROR HANDLING HELPERS
// ============================================================================

// HandleRepositoryError converts repository errors to standardized service errors
func HandleRepositoryError(err error) error {
	if err == nil {
		return nil
	}

	// Handle domain-specific errors
	var optimisticLockErr *domain.ErrOptimisticLockFailure
	if errors.As(err, &optimisticLockErr) {
		return errs.B().Code(errs.FailedPrecondition).Msg("resource has been modified by another process").Err()
	}

	// Handle GORM-specific errors
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}

	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return errs.B().Code(errs.AlreadyExists).Msg("resource already exists").Err()
	}

	// Default to database error for other GORM errors
	return ErrDatabaseError
}

// HandleValidationError converts validation errors to standardized service errors
func HandleValidationError(err error) error {
	if err == nil {
		return nil
	}
	return errs.B().Code(errs.InvalidArgument).Msg(err.Error()).Err()
}

// HandleAuthorizationError converts authorization errors to standardized service errors
func HandleAuthorizationError(err error) error {
	if err == nil {
		return nil
	}
	return ErrPermissionDenied
}

// HandlePaginationParams validates and normalizes pagination parameters
func HandlePaginationParams(offset, limit int) (int, int, error) {
	if offset < 0 {
		offset = 0
	}

	if limit <= 0 {
		limit = DefaultPaginationConfig().DefaultLimit
	}

	if limit > DefaultPaginationConfig().MaxLimit {
		return 0, 0, ErrInvalidPagination
	}

	return offset, limit, nil
}
