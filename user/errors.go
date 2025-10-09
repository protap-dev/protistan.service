package user

import "encore.dev/beta/errs"

// User service specific errors
var (
	ErrUserNotFound      = errs.B().Code(errs.NotFound).Msg("user not found").Err()
	ErrProfileNotFound   = errs.B().Code(errs.NotFound).Msg("profile not found").Err()
	ErrSettingsNotFound  = errs.B().Code(errs.NotFound).Msg("settings not found").Err()
	ErrUnauthenticated   = errs.B().Code(errs.Unauthenticated).Msg("not authenticated").Err()
	ErrInvalidInput      = errs.B().Code(errs.InvalidArgument).Msg("invalid input").Err()
	ErrEmailExists       = errs.B().Code(errs.AlreadyExists).Msg("email already exists").Err()
	ErrPhoneInvalid      = errs.B().Code(errs.InvalidArgument).Msg("invalid phone number").Err()
	ErrNameTooLong       = errs.B().Code(errs.InvalidArgument).Msg("name too long").Err()
	ErrWeakPassword      = errs.B().Code(errs.InvalidArgument).Msg("password does not meet requirements").Err()
)
