package user

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Test cases for ValidateEmail
func TestValidateEmail(t *testing.T) {
	validator := NewUserValidator()

	tests := []struct {
		name        string
		email       string
		expectedErr error
	}{
		// Success cases
		{
			name:        "valid simple email",
			email:       "test@example.com",
			expectedErr: nil,
		},
		{
			name:        "valid email with subdomain",
			email:       "user@mail.example.com",
			expectedErr: nil,
		},
		{
			name:        "valid email with numbers",
			email:       "user123@test123.com",
			expectedErr: nil,
		},
		{
			name:        "valid email with special characters",
			email:       "user.name+tag@example-site.com",
			expectedErr: nil,
		},
		{
			name:        "valid email with uppercase",
			email:       "User.Name@Example.COM",
			expectedErr: nil,
		},
		{
			name:        "valid email with leading/trailing whitespace",
			email:       "  test@example.com  ",
			expectedErr: nil,
		},

		// Failure cases
		{
			name:        "empty email",
			email:       "",
			expectedErr: errors.New("email is required"),
		},
		{
			name:        "only whitespace",
			email:       "   ",
			expectedErr: errors.New("email is required"),
		},
		{
			name:        "missing @ symbol",
			email:       "testexample.com",
			expectedErr: errors.New("invalid email format"),
		},
		{
			name:        "missing domain",
			email:       "test@",
			expectedErr: errors.New("invalid email format"),
		},
		{
			name:        "missing local part",
			email:       "@example.com",
			expectedErr: errors.New("invalid email format"),
		},
		{
			name:        "multiple @ symbols",
			email:       "test@test@example.com",
			expectedErr: errors.New("invalid email format"),
		},
		{
			name:        "invalid characters in local part",
			email:       "test space@example.com",
			expectedErr: errors.New("invalid email format"),
		},
		{
			name:        "invalid characters in domain",
			email:       "test@exam ple.com",
			expectedErr: errors.New("invalid email format"),
		},
		{
			name:        "domain too short",
			email:       "test@e.c",
			expectedErr: errors.New("invalid email format"),
		},
		{
			name:        "email too long (over 255 chars)",
			email:       strings.Repeat("a", 250) + "@example.com",
			expectedErr: errors.New("email too long"),
		},
		{
			name:        "edge case - exactly 255 chars",
			email:       strings.Repeat("a", 244) + "@example.com", // 244 + 1 + 11 = 256 chars (over limit)
			expectedErr: errors.New("email too long"),
		},
		{
			name:        "edge case - exactly 255 chars valid",
			email:       strings.Repeat("a", 243) + "@example.com", // 243 + 1 + 11 = 255 chars (at limit)
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateEmail(tt.email)

			if tt.expectedErr == nil {
				if err != nil {
					t.Errorf("ValidateEmail(%q) = %v, expected no error", tt.email, err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateEmail(%q) = nil, expected error %v", tt.email, tt.expectedErr)
				} else if err.Error() != tt.expectedErr.Error() {
					t.Errorf("ValidateEmail(%q) = %v, expected error %v", tt.email, err, tt.expectedErr)
				}
			}
		})
	}
}

// Test cases for ValidatePhone
func TestValidatePhone(t *testing.T) {
	validator := NewUserValidator()

	tests := []struct {
		name        string
		phone       string
		expectedErr error
	}{
		// Success cases
		{
			name:        "valid 10 digit phone",
			phone:       "1234567890",
			expectedErr: nil,
		},
		{
			name:        "valid 15 digit phone",
			phone:       "123456789012345",
			expectedErr: nil,
		},
		{
			name:        "valid phone with spaces",
			phone:       "(123) 456-7890",
			expectedErr: nil,
		},
		{
			name:        "valid phone with dashes",
			phone:       "123-456-7890",
			expectedErr: nil,
		},
		{
			name:        "valid phone with parentheses",
			phone:       "(123)456-7890",
			expectedErr: nil,
		},
		{
			name:        "valid international format",
			phone:       "+1-123-456-7890",
			expectedErr: nil,
		},
		{
			name:        "empty phone (optional field)",
			phone:       "",
			expectedErr: nil,
		},

		// Failure cases
		{
			name:        "phone too short (9 digits)",
			phone:       "123456789",
			expectedErr: errors.New("phone number must be 10-15 digits"),
		},
		{
			name:        "phone too long (16 digits)",
			phone:       "1234567890123456",
			expectedErr: errors.New("phone number must be 10-15 digits"),
		},
		{
			name:        "phone with letters",
			phone:       "1234567890abc",
			expectedErr: errors.New("phone number can only contain digits and separators"),
		},
		{
			name:        "phone with special characters",
			phone:       "123-456-7890!",
			expectedErr: errors.New("phone number can only contain digits and separators"),
		},
		{
			name:        "phone with mixed valid/invalid separators",
			phone:       "123.456-7890",
			expectedErr: errors.New("phone number can only contain digits and separators"),
		},
		{
			name:        "phone with only separators",
			phone:       "---",
			expectedErr: errors.New("phone number must be 10-15 digits"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidatePhone(tt.phone)

			if tt.expectedErr == nil {
				if err != nil {
					t.Errorf("ValidatePhone(%q) = %v, expected no error", tt.phone, err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidatePhone(%q) = nil, expected error %v", tt.phone, tt.expectedErr)
				} else if err.Error() != tt.expectedErr.Error() {
					t.Errorf("ValidatePhone(%q) = %v, expected error %v", tt.phone, err, tt.expectedErr)
				}
			}
		})
	}
}

// Test cases for ValidatePassword
func TestValidatePassword(t *testing.T) {
	validator := NewUserValidator()

	tests := []struct {
		name           string
		password       string
		expectedErrors []string
	}{
		// Success cases
		{
			name:           "valid password with all requirements",
			password:       "MyPassword123!",
			expectedErrors: []string{},
		},
		{
			name:           "valid password with minimum length",
			password:       "Pass123!",
			expectedErrors: []string{},
		},

		// Failure cases - single requirement missing
		{
			name:           "missing length requirement",
			password:       "Pass1!",
			expectedErrors: []string{"at least 8 characters"},
		},
		{
			name:           "missing uppercase requirement",
			password:       "mypassword123!",
			expectedErrors: []string{"uppercase letter"},
		},
		{
			name:           "missing lowercase requirement",
			password:       "MYPASSWORD123!",
			expectedErrors: []string{"lowercase letter"},
		},
		{
			name:           "missing digit requirement",
			password:       "MyPassword!",
			expectedErrors: []string{"number"},
		},
		{
			name:           "missing special character requirement",
			password:       "MyPassword123",
			expectedErrors: []string{"special character"},
		},

		// Failure cases - multiple requirements missing
		{
			name:           "missing length and uppercase",
			password:       "pass1!",
			expectedErrors: []string{"at least 8 characters", "uppercase letter"},
		},
		{
			name:           "missing length and digit",
			password:       "Password!",
			expectedErrors: []string{"number"},
		},
		{
			name:           "missing length and special",
			password:       "Password1",
			expectedErrors: []string{"special character"},
		},
		{
			name:           "missing all requirements except length",
			password:       "passwordpassword",
			expectedErrors: []string{"uppercase letter", "number", "special character"},
		},

		// Edge cases
		{
			name:           "exactly 8 characters valid",
			password:       "Pass123!",
			expectedErrors: []string{},
		},
		{
			name:           "exactly 128 characters valid",
			password:       strings.Repeat("A", 125) + "a1!", // 125 + 3 = 128 characters (at limit)
			expectedErrors: []string{},
		},
		{
			name:           "over 128 characters",
			password:       strings.Repeat("A", 126) + "a1!", // 126 + 3 = 129 characters (over limit)
			expectedErrors: []string{"no more than 128 characters"},
		},
		{
			name:           "empty password",
			password:       "",
			expectedErrors: []string{"at least 8 characters", "uppercase letter", "lowercase letter", "number", "special character"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidatePassword(tt.password)

			if len(errors) != len(tt.expectedErrors) {
				t.Errorf("ValidatePassword(%q) returned %d errors, expected %d errors: %v",
					tt.password, len(errors), len(tt.expectedErrors), errors)
				return
			}

			for i, expectedErr := range tt.expectedErrors {
				if i >= len(errors) || errors[i] != expectedErr {
					t.Errorf("ValidatePassword(%q) error at index %d = %q, expected %q",
						tt.password, i, errors[i], expectedErr)
				}
			}
		})
	}
}

// Test cases for ValidateName
func TestValidateName(t *testing.T) {
	validator := NewUserValidator()

	tests := []struct {
		name        string
		inputName   string
		maxLength   int
		expectedErr error
	}{
		// Success cases
		{
			name:        "valid name within limit",
			inputName:   "John Doe",
			maxLength:   50,
			expectedErr: nil,
		},
		{
			name:        "valid name at max length",
			inputName:   strings.Repeat("A", 20),
			maxLength:   20,
			expectedErr: nil,
		},
		{
			name:        "valid name with numbers",
			inputName:   "John123",
			maxLength:   50,
			expectedErr: nil,
		},
		{
			name:        "valid name with special characters",
			inputName:   "Jean-Pierre",
			maxLength:   50,
			expectedErr: nil,
		},
		{
			name:        "valid name with unicode characters",
			inputName:   "José",
			maxLength:   50,
			expectedErr: nil,
		},

		// Failure cases
		{
			name:        "empty name",
			inputName:   "",
			maxLength:   50,
			expectedErr: errors.New("name is required"),
		},
		{
			name:        "name too long",
			inputName:   strings.Repeat("A", 21),
			maxLength:   20,
			expectedErr: errors.New("name too long"),
		},
		{
			name:        "name with control characters (newline)",
			inputName:   "John\nDoe",
			maxLength:   50,
			expectedErr: errors.New("name contains invalid characters"),
		},
		{
			name:        "name with control characters (null)",
			inputName:   "John\x00Doe",
			maxLength:   50,
			expectedErr: errors.New("name contains invalid characters"),
		},
		{
			name:        "name with unicode characters (accented)",
			inputName:   "José",
			maxLength:   50,
			expectedErr: nil, // Should be valid (accented characters)
		},
		{
			name:        "name exactly at character limit",
			inputName:   strings.Repeat("A", 20),
			maxLength:   20,
			expectedErr: nil,
		},
		{
			name:        "name one character over limit",
			inputName:   strings.Repeat("A", 21),
			maxLength:   20,
			expectedErr: errors.New("name too long"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateName(tt.inputName, tt.maxLength)

			if tt.expectedErr == nil {
				if err != nil {
					t.Errorf("ValidateName(%q, %d) = %v, expected no error", tt.inputName, tt.maxLength, err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateName(%q, %d) = nil, expected error %v", tt.inputName, tt.maxLength, tt.expectedErr)
				} else if err.Error() != tt.expectedErr.Error() {
					t.Errorf("ValidateName(%q, %d) = %v, expected error %v", tt.inputName, tt.maxLength, err, tt.expectedErr)
				}
			}
		})
	}
}

// Test interface compliance
func TestUserValidator_InterfaceCompliance(t *testing.T) {
	var _ UserValidator = NewUserValidator()
}

// Test edge cases that could cause panics
func TestValidateEmail_EdgeCases(t *testing.T) {
	validator := NewUserValidator()

	// Test cases that might cause regex compilation issues or panics
	edgeCases := []struct {
		name  string
		email string
	}{
		{"very long email", strings.Repeat("a", 200) + "@" + strings.Repeat("b", 50) + ".com"},
		{"email with unicode in domain", "test@пример.испытание"},
		{"email with special regex chars", "test+tag@example.com"},
		{"email with quoted local part", `"test"@example.com`},
	}

	for _, tc := range edgeCases {
		t.Run(tc.name, func(t *testing.T) {
			// Should not panic, should return appropriate error
			err := validator.ValidateEmail(tc.email)
			if err != nil {
				// If there's an error, it should be a meaningful validation error, not a panic
				if !strings.Contains(err.Error(), "email") {
					t.Errorf("ValidateEmail(%q) returned unexpected error: %v", tc.email, err)
				}
			}
		})
	}
}

// Test concurrent safety (if applicable)
func TestUserValidator_ConcurrentSafety(t *testing.T) {
	validator := NewUserValidator()

	// Test that multiple goroutines can use the validator safely
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			email := fmt.Sprintf("test%d@example.com", id)
			err := validator.ValidateEmail(email)
			if err != nil {
				t.Errorf("Concurrent ValidateEmail failed: %v", err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}
