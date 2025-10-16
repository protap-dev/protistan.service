package internal

import "encore.dev/beta/errs"

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
