package auth

import (
	"fmt"
	"log"
)

// EmailService interface for sending verification emails
// This can be easily replaced with any email service provider
type EmailService interface {
	SendVerificationEmail(to, token string) error
}

// LoggingEmailService is a simple implementation that just logs emails
// In production, replace this with a real email service like SendGrid, SES, etc.
type LoggingEmailService struct {
	logger *log.Logger
}

// SendVerificationEmail logs the verification email that would be sent
// In production, this should actually send an email via your email service
func (s *LoggingEmailService) SendVerificationEmail(to, token string) error {
	if s.logger != nil {
		// Log the email that would be sent
		verificationURL := fmt.Sprintf("https://yourapp.com/verify?token=%s", token)
		s.logger.Printf("[EMAIL] Verification email would be sent to: %s", to)
		s.logger.Printf("[EMAIL] Verification URL: %s", verificationURL)
		s.logger.Printf("[EMAIL] Subject: Please verify your email address")
		s.logger.Printf("[EMAIL] Body: Click the link to verify your account: %s", verificationURL)
	}
	return nil
}

// NewLoggingEmailService creates a new logging email service
var emailService EmailService = &LoggingEmailService{
	logger: log.Default(),
}

// SetEmailService allows replacing the email service implementation
// This makes it easy to switch to a real email service in production
func SetEmailService(service EmailService) {
	emailService = service
}
