package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// UserData returned by auth handler
type UserData struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// User represents a user in the database
type User struct {
	ID            string    `json:"id" gorm:"primarykey;type:uuid;default:uuid_generate_v7()"`
	Email         string    `json:"email" gorm:"index;not null;type:varchar(255)"`
	PasswordHash  string    `json:"password_hash" gorm:"not null;type:varchar(255)"`
	EmailVerified bool      `json:"email_verified" gorm:"default:false"`
	CreatedAt     time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// EmailVerificationToken represents an email verification token in the database
type EmailVerificationToken struct {
	ID        string    `json:"id" gorm:"primarykey;type:uuid;default:uuid_generate_v7()"`
	UserID    string    `json:"user_id" gorm:"not null;index"`
	Token     string    `json:"token" gorm:"unique;not null"`
	ExpiresAt time.Time `json:"expires_at" gorm:"not null"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}

// SendVerificationEmailRequest represents a request to send an email verification token
type SendVerificationEmailRequest struct {
	Email string `json:"email"`
}

// VerifyEmailRequest represents a request to verify an email verification token
type VerifyEmailRequest struct {
	Token string `json:"token"`
}

// PasswordResetToken represents a password reset token in the database
type PasswordResetToken struct {
	ID        string    `json:"id" gorm:"primarykey;type:uuid;default:uuid_generate_v7()"`
	UserID    string    `json:"user_id" gorm:"not null;index"`
	Token     string    `json:"token" gorm:"unique;not null"`
	ExpiresAt time.Time `json:"expires_at" gorm:"not null"`
	Used      bool      `json:"used" gorm:"default:false"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// JWTClaims holds custom JWT claims.
type JWTClaims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// Request/response types and basic endpoints
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}
