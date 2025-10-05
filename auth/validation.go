package auth

import (
	"regexp"

	"encore.dev/beta/errs"
	"github.com/badoux/checkmail"
)

// validatePassword checks if password meets security requirements
func validatePassword(password string) []string {
	var errors []string

	if len(password) < 8 {
		errors = append(errors, "Password must be at least 8 characters long")
	}

	if !regexp.MustCompile(`[A-Z]`).MatchString(password) {
		errors = append(errors, "Password must contain at least one uppercase letter")
	}

	if !regexp.MustCompile(`[a-z]`).MatchString(password) {
		errors = append(errors, "Password must contain at least one lowercase letter")
	}

	if !regexp.MustCompile(`\d`).MatchString(password) {
		errors = append(errors, "Password must contain at least one number")
	}

	if !regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{};':"\\|,.<>/?]`).MatchString(password) {
		errors = append(errors, "Password must contain at least one special character")
	}

	return errors
}

// validateEmail checks if email format is valid
func validateEmail(email string) error {
	if err := checkmail.ValidateFormat(email); err != nil {
		return errs.B().Msg("invalid email format").Err()
	}
	return nil
}
