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
	ServiceCategoryID     string            `json:"service_category_id"`
	Title                 string            `json:"title"`
	Description           string            `json:"description,omitempty"`
	CustomerAddressID     string            `json:"customer_address_id"`
	Priority              string            `json:"priority,omitempty"`
	ScheduledAt           *time.Time        `json:"scheduled_at,omitempty"`
	EstimatedDurationMins int               `json:"estimated_duration_mins,omitempty"`
	Metadata              map[string]string `json:"metadata,omitempty"`
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
	if req.Title == "" {
		return errors.New("title is required")
	}

	if len(req.Title) < 5 {
		return errors.New("title must be at least 5 characters")
	}

	if len(req.Title) > 100 {
		return errors.New("title must be less than 100 characters")
	}

	if req.Description != "" && len(req.Description) > 500 {
		return errors.New("description must be less than 500 characters")
	}

	if req.ServiceCategoryID == "" {
		return errors.New("service category is required")
	}

	if req.CustomerAddressID == "" {
		return errors.New("customer address is required")
	}

	if req.EstimatedDurationMins < 0 {
		return errors.New("estimated duration cannot be negative")
	}

	if req.EstimatedDurationMins > 1440 { // 24 hours
		return errors.New("estimated duration cannot exceed 24 hours")
	}

	// Validate priority if provided
	if req.Priority != "" {
		validPriorities := []string{"low", "normal", "high", "urgent"}
		valid := false
		for _, p := range validPriorities {
			if req.Priority == p {
				valid = true
				break
			}
		}
		if !valid {
			return errors.New("priority must be one of: low, normal, high, urgent")
		}
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
