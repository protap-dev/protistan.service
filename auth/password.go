package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"encore.dev/beta/errs"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

//encore:api public method=POST path=/v0/auth/forgot-password
func (s *Service) ForgotPassword(ctx context.Context, req *ForgotPasswordRequest) error {
	if req == nil || req.Email == "" {
		return errs.B().Msg("invalid request").Err()
	}

	// Rate limiting for password reset requests
	if !s.rateLimiter.isAllowed("forgot:"+req.Email, 3) {
		return errs.B().Msg("too many password reset requests, please try again later").Err()
	}

	// Normalize and validate email format
	normalizedEmail := strings.ToLower(strings.TrimSpace(req.Email))
	if err := validateEmail(normalizedEmail); err != nil {
		return err
	}

	// Find user by email
	var user User
	if err := s.db.Where("email = ?", normalizedEmail).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// For security, don't reveal if email exists or not
			return nil
		}
		return errs.B().Msg("database error").Err()
	}

	// Generate password reset token
	token, err := generateSecureToken(32)
	if err != nil {
		return errs.B().Msg("failed to generate reset token").Err()
	}

	// Save token to database
	resetToken := PasswordResetToken{
		UserID:    user.ID,
		Token:     token,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Used:      false,
	}

	if err := s.db.Create(&resetToken).Error; err != nil {
		return errs.B().Msg("failed to create reset token").Err()
	}

	// Send email with reset token
	// For now, we'll just return success
	return nil
}

//encore:api public method=POST path=/v0/auth/reset-password
func (s *Service) ResetPassword(ctx context.Context, req *ResetPasswordRequest) error {
	if req == nil || req.Token == "" || req.NewPassword == "" {
		return errs.B().Msg("invalid request").Err()
	}

	// Validate new password strength
	if passwordErrors := validatePassword(req.NewPassword); len(passwordErrors) > 0 {
		return errs.B().Msg("password requirements not met: " + strings.Join(passwordErrors, ", ")).Err()
	}

	// Find valid, unused token that hasn't expired
	var resetToken PasswordResetToken
	if err := s.db.Where("token = ? AND used = ? AND expires_at > ?", req.Token, false, time.Now()).First(&resetToken).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errs.B().Msg("invalid or expired token").Err()
		}
		return errs.B().Msg("database error").Err()
	}

	// Find the user associated with the token
	var user User
	if err := s.db.Where("id = ?", resetToken.UserID).First(&user).Error; err != nil {
		return errs.B().Msg("user not found").Err()
	}

	// Hash new password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcryptCost)
	if err != nil {
		return err
	}

	// Use transaction to ensure atomicity of password update and token invalidation
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Update user password
		if err := tx.Model(&user).Update("password_hash", string(hash)).Error; err != nil {
			return errs.B().Msg("failed to update password").Err()
		}

		// Mark token as used - only after successful password update
		if err := tx.Model(&resetToken).Update("used", true).Error; err != nil {
			return errs.B().Msg("failed to invalidate reset token").Err()
		}

		return nil
	})

	if err != nil {
		return err
	}

	return nil
}
