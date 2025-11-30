package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// CreateBookingRequest represents the request to create a booking (for validation)
type CreateBookingRequest struct {
	IdempotencyKey    *string           `json:"idempotency_key,omitempty"`
	SpecificArtisanID string            `json:"specific_artisan_id,omitempty"`
	ServiceCategoryID string            `json:"service_category_id"`
	ServiceID         string            `json:"service_id"`
	Description       string            `json:"description,omitempty"`
	CustomerAddressID string            `json:"customer_address_id"`
	ScheduledAt       *time.Time        `json:"scheduled_at,omitempty"`
	MediaURLs         []string          `json:"media_urls"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	IsFlexible        bool              `json:"is_flexible"`
}

// BookingValidator defines the interface for booking validation
type BookingValidator interface {
	ValidateCreateRequest(req *CreateBookingRequest) error
	ValidateStatusTransition(from, to BookingStatus) error
}

// bookingValidator implements BookingValidator
type bookingValidator struct{}

// NewBookingValidator creates a new booking validator
func NewBookingValidator() BookingValidator {
	return &bookingValidator{}
}

// ValidateCreateRequest validates a booking creation request
func (v *bookingValidator) ValidateCreateRequest(req *CreateBookingRequest) error {
	if req.ServiceCategoryID == "" {
		return errors.New("service category is required")
	}

	if req.CustomerAddressID == "" {
		return errors.New("customer address is required")
	}

	if req.Description == "" {
		return errors.New("description is required to help the artisan understand the task")
	}

	if len(req.Description) < 10 {
		return errors.New("description must be at least 10 characters")
	}

	if len(req.Description) > 1000 { // Increased limit for detailed descriptions
		return errors.New("description must be less than 1000 characters")
	}

	if req.ScheduledAt != nil && req.ScheduledAt.Before(time.Now()) {
		return errors.New("scheduled time cannot be in the past")
	}

	return nil
}

// ValidateStatusTransition validates if a status transition is allowed
func (v *bookingValidator) ValidateStatusTransition(from, to BookingStatus) error {
	if !CanTransition(from, to) {
		return fmt.Errorf("invalid transition from %s to %s", from, to)
	}
	return nil
}

// ValidateString validates a string field
func ValidateString(value, fieldName string, minLen, maxLen int) error {
	if value == "" {
		return fmt.Errorf("%s is required", fieldName)
	}

	if len(value) < minLen {
		return fmt.Errorf("%s must be at least %d characters", fieldName, minLen)
	}

	if len(value) > maxLen {
		return fmt.Errorf("%s must be less than %d characters", fieldName, maxLen)
	}

	return nil
}

// ValidateEmail validates an email address
func ValidateEmail(email string) error {
	if email == "" {
		return errors.New("email is required")
	}

	// Basic email validation
	if !strings.Contains(email, "@") {
		return errors.New("invalid email format")
	}

	if !strings.Contains(email, ".") {
		return errors.New("invalid email format")
	}

	return nil
}

// ValidatePhone validates a phone number
func ValidatePhone(phone string) error {
	if phone == "" {
		return errors.New("phone is required")
	}

	// Remove common formatting
	re := regexp.MustCompile(`[\s\-\(\)]`)
	phone = re.ReplaceAllString(phone, "")

	// Remove + prefix
	phone = strings.TrimPrefix(phone, "+")

	// Should be 10-15 digits
	if len(phone) < 10 || len(phone) > 15 {
		return errors.New("phone number must be 10-15 digits")
	}

	// Check all characters are digits
	for _, r := range phone {
		if !unicode.IsDigit(r) {
			return errors.New("phone number must contain only digits")
		}
	}

	return nil
}

// SanitizeString removes potentially harmful characters
func SanitizeString(s string) string {
	// Remove control characters
	var clean []rune
	for _, r := range s {
		if r >= 32 && r <= 126 { // Printable ASCII
			clean = append(clean, r)
		}
	}

	return string(clean)
}
