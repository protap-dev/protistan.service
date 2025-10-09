package internal

import "encore.dev/beta/errs"

var (
	ErrUnauthenticated      = errs.B().Code(errs.Unauthenticated).Msg("not authenticated").Err()
	ErrUnauthorizedAction   = errs.B().Code(errs.PermissionDenied).Msg("user must be an artisan to access this endpoint").Err()
	ErrArtisanNotFound      = errs.B().Code(errs.NotFound).Msg("artisan profile not found").Err()
	ErrDatabaseError        = errs.B().Code(errs.Internal).Msg("database error").Err()
	ErrValidationFailed     = errs.B().Code(errs.InvalidArgument).Msg("validation failed").Err()

	// Specific validation errors for better error messages
	ErrInvalidInput         = errs.B().Code(errs.InvalidArgument).Msg("invalid input").Err()
	ErrMissingRequiredField = errs.B().Code(errs.InvalidArgument).Msg("missing required fields").Err()
	ErrInvalidBio           = errs.B().Code(errs.InvalidArgument).Msg("invalid bio").Err()
	ErrInvalidCategories    = errs.B().Code(errs.InvalidArgument).Msg("invalid categories").Err()
	ErrInvalidSearchQuery   = errs.B().Code(errs.InvalidArgument).Msg("invalid search query").Err()
	ErrInvalidPagination    = errs.B().Code(errs.InvalidArgument).Msg("invalid pagination parameters").Err()
)
