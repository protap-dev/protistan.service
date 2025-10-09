package admin

import (
	"errors"
	"strings"
)

// AdminValidator handles admin operation input validation
type AdminValidator interface {
	ValidateUpdateUserCoreRequest(req *UpdateUserCoreRequest) error
}

type adminValidator struct{}

func NewAdminValidator() AdminValidator {
	return &adminValidator{}
}

func (v *adminValidator) ValidateUpdateUserCoreRequest(req *UpdateUserCoreRequest) error {
	// Check if at least one field is provided for update
	if req.UserType == nil && req.EmailVerified == nil && req.ProfileComplete == nil {
		return errors.New("at least one field must be provided for update")
	}

	// Validate user_type if provided
	if req.UserType != nil {
		userType := strings.ToLower(*req.UserType)
		validUserTypes := []string{"customer", "artisan", "admin"}
		isValid := false

		for _, validType := range validUserTypes {
			if userType == validType {
				isValid = true
				// Update the value to ensure consistent casing
				*req.UserType = validType
				break
			}
		}

		if !isValid {
			return errors.New("invalid user_type: must be one of 'customer', 'artisan', 'admin'")
		}
	}

	return nil
}
