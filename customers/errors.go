package customers

import "encore.dev/beta/errs"

// Customer service specific errors
var (
	ErrAddressNotFound      = errs.B().Code(errs.NotFound).Msg("address not found").Err()
	ErrUnauthenticated      = errs.B().Code(errs.Unauthenticated).Msg("not authenticated").Err()
	ErrInvalidInput         = errs.B().Code(errs.InvalidArgument).Msg("invalid input").Err()
	ErrMissingRequiredField = errs.B().Code(errs.InvalidArgument).Msg("missing required fields").Err()
	ErrInvalidLabel         = errs.B().Code(errs.InvalidArgument).Msg("invalid label").Err()
	ErrInvalidAddressLength = errs.B().Code(errs.InvalidArgument).Msg("invalid address length").Err()
	ErrDatabaseError        = errs.B().Code(errs.Internal).Msg("database error").Err()
)
