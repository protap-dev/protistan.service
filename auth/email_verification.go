package auth

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"encore.dev/beta/errs"
	"gorm.io/gorm"
)

//encore:api public method=POST path=/v0/auth/send-verification-email
func (s *Service) SendVerificationEmail(ctx context.Context, req *SendVerificationEmailRequest) error {
	if req == nil || req.Email == "" {
		return errs.B().Msg("invalid request").Err()
	}

	// Rate limiting: max 3 verification emails per hour per email
	if !s.rateLimiter.isAllowed("verify:"+req.Email, 3) {
		return errs.B().Msg("too many verification requests, please try again later").Err()
	}

	normalizedEmail := strings.ToLower(strings.TrimSpace(req.Email))
	if err := validateEmail(normalizedEmail); err != nil {
		return err
	}

	// Find user
	var user User
	if err := s.db.Where("email = ?", normalizedEmail).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// For security, don't reveal if email exists
			return nil
		}
		return errs.B().Msg("database error").Err()
	}

	// Already verified - don't send another email
	if user.EmailVerified {
		return nil
	}

	// Generate verification token (32 bytes = 64 hex chars)
	token, err := generateSecureToken(32)
	if err != nil {
		return errs.B().Msg("failed to generate verification token").Err()
	}

	// Save token to database
	verificationToken := EmailVerificationToken{
		UserID:    user.ID,
		Token:     token,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	if err := s.db.Create(&verificationToken).Error; err != nil {
		return errs.B().Msg("failed to create verification token").Err()
	}

	// Send actual email with token using the email service
	// This implementation logs the email but can easily be replaced with real email sending
	if err := emailService.SendVerificationEmail(user.Email, token); err != nil {
		// Log error but don't fail the request - email failures shouldn't block verification flow
		log.Printf("Failed to send verification email to %s: %v", user.Email, err)
	}

	return nil
}

//encore:api public method=POST path=/v0/auth/verify-email
func (s *Service) VerifyEmail(ctx context.Context, req *VerifyEmailRequest) error {
	if req == nil || req.Token == "" {
		return errs.B().Msg("invalid request").Err()
	}

	// Find valid, unexpired token
	var verificationToken EmailVerificationToken
	if err := s.db.Where("token = ? AND expires_at > ?", req.Token, time.Now()).First(&verificationToken).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errs.B().Msg("invalid or expired verification token").Err()
		}
		return errs.B().Msg("database error").Err()
	}

	// Find user
	var user User
	if err := s.db.Where("id = ?", verificationToken.UserID).First(&user).Error; err != nil {
		return errs.B().Msg("user not found").Err()
	}

	// Already verified - token should be deleted
	if user.EmailVerified {
		s.db.Delete(&verificationToken)
		return nil
	}

	// Use transaction for atomicity
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Mark user as verified
		if err := tx.Model(&user).Update("email_verified", true).Error; err != nil {
			return errs.B().Msg("failed to verify email").Err()
		}

		// Delete the token (single use)
		if err := tx.Delete(&verificationToken).Error; err != nil {
			return errs.B().Msg("failed to delete verification token").Err()
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}
