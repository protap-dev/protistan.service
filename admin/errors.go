package admin

import "encore.dev/beta/errs"

// Admin service specific errors
var (
	ErrCannotDemoteSelf   = errs.B().Code(errs.InvalidArgument).Msg("cannot demote your own admin privileges").Err()
	ErrUnauthenticated    = errs.B().Code(errs.Unauthenticated).Msg("not authenticated").Err()
	ErrUnauthorizedAction = errs.B().Code(errs.PermissionDenied).Msg("unauthorized action").Err()
	ErrInvalidInput       = errs.B().Code(errs.InvalidArgument).Msg("invalid input").Err()
	ErrUserNotFound       = errs.B().Code(errs.NotFound).Msg("user not found").Err()
	ErrDatabaseError      = errs.B().Code(errs.Internal).Msg("database error").Err()
)
