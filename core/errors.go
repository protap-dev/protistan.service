package core

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// ============================================================================
// ERROR HANDLING SYSTEM WITH BASIC ERROR TRANSLATION
// ============================================================================

// ServiceError represents a standardized service error with code and context
type ServiceError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
	Err     error  `json:"-"` // Original error (not serialized)
}

// Error implements the error interface
func (e *ServiceError) Error() string {
	if e.Details != "" {
		return e.Message + ": " + e.Details
	}
	return e.Message
}

// Unwrap returns the underlying error
func (e *ServiceError) Unwrap() error {
	return e.Err
}

// Common error codes
const (
	ErrCodeNotFound     = "NOT_FOUND"
	ErrCodeDuplicate    = "DUPLICATE_ENTRY"
	ErrCodeValidation   = "VALIDATION_ERROR"
	ErrCodeUnauthorized = "UNAUTHORIZED"
	ErrCodeInternal     = "INTERNAL_ERROR"
	ErrCodeDatabase     = "DATABASE_ERROR"
)

// TranslateError converts GORM/database errors into standardized service errors
func TranslateError(err error) error {
	if err == nil {
		return nil
	}

	// Handle GORM-specific errors
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &ServiceError{
			Code:    ErrCodeNotFound,
			Message: "Record not found",
			Details: "The requested resource does not exist",
			Err:     err,
		}
	}

	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return &ServiceError{
			Code:    ErrCodeDuplicate,
			Message: "Duplicate entry",
			Details: "A record with this information already exists",
			Err:     err,
		}
	}

	// Handle other common GORM errors
	if errors.Is(err, gorm.ErrInvalidTransaction) {
		return &ServiceError{
			Code:    ErrCodeDatabase,
			Message: "Transaction error",
			Details: "Database transaction could not be completed",
			Err:     err,
		}
	}

	if errors.Is(err, gorm.ErrDryRunModeUnsupported) {
		return &ServiceError{
			Code:    ErrCodeDatabase,
			Message: "Operation not supported",
			Details: "This database operation is not supported",
			Err:     err,
		}
	}

	// Default to internal error for unknown errors
	return &ServiceError{
		Code:    ErrCodeInternal,
		Message: "Internal error occurred",
		Details: "An unexpected error occurred while processing your request",
		Err:     err,
	}
}

// NewServiceError creates a new service error with the given code and message
func NewServiceError(code, message, details string) *ServiceError {
	return &ServiceError{
		Code:    code,
		Message: message,
		Details: details,
	}
}

// NewServiceErrorf creates a new service error with formatted message
func NewServiceErrorf(code, message string, args ...interface{}) *ServiceError {
	return &ServiceError{
		Code:    code,
		Message: fmt.Sprintf(message, args...),
	}
}
