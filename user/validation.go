package user

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
)

// UserValidator handles input validation
type UserValidator interface {
	ValidateEmail(email string) error
	ValidatePhone(phone string) error
	ValidatePassword(password string) []string
	ValidateName(name string, maxLength int) error
}

type userValidator struct{}

func NewUserValidator() UserValidator {
	return &userValidator{}
}

func (v *userValidator) ValidateEmail(email string) error {
	if email == "" {
		return errors.New("email is required")
	}

	email = strings.ToLower(strings.TrimSpace(email))

	// After trimming, check if it's still empty
	if email == "" {
		return errors.New("email is required")
	}

	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(email) {
		return errors.New("invalid email format")
	}

	if len(email) > 255 {
		return errors.New("email too long")
	}

	return nil
}

func (v *userValidator) ValidatePhone(phone string) error {
	if phone == "" {
		return nil // Phone is optional
	}

	// Remove common separators and international prefix
	cleaned := strings.ReplaceAll(phone, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, "(", "")
	cleaned = strings.ReplaceAll(cleaned, ")", "")

	// Handle international format (+)
	cleaned = strings.TrimPrefix(cleaned, "+")

	// Basic length check (after removing + prefix)
	if len(cleaned) < 10 || len(cleaned) > 15 {
		return errors.New("phone number must be 10-15 digits")
	}

	// Check if all characters are digits
	for _, char := range cleaned {
		if char < '0' || char > '9' {
			return errors.New("phone number can only contain digits and separators")
		}
	}

	return nil
}

func (v *userValidator) ValidatePassword(password string) []string {
	var errors []string

	if len(password) < 8 {
		errors = append(errors, "at least 8 characters")
	}

	if len(password) > 128 {
		errors = append(errors, "no more than 128 characters")
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if !hasUpper {
		errors = append(errors, "uppercase letter")
	}
	if !hasLower {
		errors = append(errors, "lowercase letter")
	}
	if !hasDigit {
		errors = append(errors, "number")
	}
	if !hasSpecial {
		errors = append(errors, "special character")
	}

	return errors
}

func (v *userValidator) ValidateName(name string, maxLength int) error {
	if name == "" {
		return errors.New("name is required")
	}

	if len(name) > maxLength {
		return errors.New("name too long")
	}

	// Check for control characters (0-31) and delete character (127)
	for _, char := range name {
		if char < 32 || char == 127 {
			return errors.New("name contains invalid characters")
		}
	}

	return nil
}
