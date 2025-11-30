package auth

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Test_EmailVerificationFlow tests the complete email verification process
func Test_EmailVerificationFlow(t *testing.T) {
	ctx := context.Background()

	t.Run("complete happy path flow", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		email := "newuser@example.com"
		userID := "user-123"

		// Step 1: User registers (email_verified = false)
		password := "SecureP!ss123"

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(email, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "users"`)).
			WithArgs(email, sqlmock.AnyArg(), false, "customer", true, sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "email_verified"}).
				AddRow(userID, email, false))
		mock.ExpectCommit()

		// Mock refresh token creation using GORM Create with RETURNING
		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "refresh_tokens"`)).
			WithArgs(userID, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("refresh-token-id"))
		mock.ExpectCommit()

		regReq := &RegisterRequest{Email: email, Password: password, UserType: "customer"}
		regResp, err := svc.Register(ctx, regReq)
		require.NoError(t, err)
		assert.NotEmpty(t, regResp.Token)
		assert.NotEmpty(t, regResp.RefreshToken)

		// Step 2: Request verification email
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(email, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "email_verified"}).
				AddRow(userID, email, false))

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "email_verification_tokens"`)).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("token-id"))
		mock.ExpectCommit()

		verifyReq := &SendVerificationEmailRequest{Email: email}
		err = svc.SendVerificationEmail(ctx, verifyReq)
		require.NoError(t, err)

		// Step 3: Verify email with token
		token := "valid-verification-token"

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "email_verification_tokens" WHERE token = $1 AND expires_at > $2 ORDER BY "email_verification_tokens"."id" LIMIT $3`)).
			WithArgs(token, sqlmock.AnyArg(), 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "token", "expires_at"}).
				AddRow("token-id", userID, token, time.Now().Add(1*time.Hour)))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE id = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "email_verified"}).
				AddRow(userID, email, false))

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "users" SET "email_verified"=$1,"updated_at"=$2 WHERE "id" = $3`)).
			WithArgs(true, sqlmock.AnyArg(), userID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "email_verification_tokens" WHERE "email_verification_tokens"."id" = $1`)).
			WithArgs("token-id").
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		verifyEmailReq := &VerifyEmailRequest{Token: token}
		err = svc.VerifyEmail(ctx, verifyEmailReq)
		require.NoError(t, err)

		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// Test_SendVerificationEmail tests sending verification emails
func Test_SendVerificationEmail(t *testing.T) {
	ctx := context.Background()

	t.Run("successful verification email send", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		email := "user@example.com"
		userID := "user-123"

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(email, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "email_verified"}).
				AddRow(userID, email, false))

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "email_verification_tokens"`)).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("token-id"))
		mock.ExpectCommit()

		req := &SendVerificationEmailRequest{Email: email}
		err := svc.SendVerificationEmail(ctx, req)

		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("already verified user - silent success", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		email := "verified@example.com"

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(email, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "email_verified"}).
				AddRow("user-123", email, true))

		req := &SendVerificationEmailRequest{Email: email}
		err := svc.SendVerificationEmail(ctx, req)

		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("non-existent email - silent success for security", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs("nonexistent@example.com", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		req := &SendVerificationEmailRequest{Email: "nonexistent@example.com"}
		err := svc.SendVerificationEmail(ctx, req)

		require.NoError(t, err)
	})

	t.Run("invalid email format", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		req := &SendVerificationEmailRequest{Email: "invalid-email"}
		err := svc.SendVerificationEmail(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid email format")
	})

	t.Run("empty email", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		req := &SendVerificationEmailRequest{Email: ""}
		err := svc.SendVerificationEmail(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid request")
	})

	t.Run("nil request", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		err := svc.SendVerificationEmail(ctx, nil)
		assert.Error(t, err)
	})

	t.Run("rate limiting", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		email := "ratelimit@example.com"

		// Exhaust rate limit (3 per hour)
		for i := 0; i < 3; i++ {
			svc.rateLimiter.getVisitor("verify:"+email, 3).Allow()
		}

		req := &SendVerificationEmailRequest{Email: email}
		err := svc.SendVerificationEmail(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too many verification requests")
	})

	t.Run("database error", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs("user@example.com", 1).
			WillReturnError(sql.ErrConnDone)

		req := &SendVerificationEmailRequest{Email: "user@example.com"}
		err := svc.SendVerificationEmail(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database error")
	})

	t.Run("token generation failure", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs("user@example.com", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "email_verified"}).
				AddRow("user-123", "user@example.com", false))

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "email_verification_tokens"`)).
			WillReturnError(sql.ErrTxDone)
		mock.ExpectRollback()

		req := &SendVerificationEmailRequest{Email: "user@example.com"}
		err := svc.SendVerificationEmail(ctx, req)

		assert.Error(t, err)
	})
}

// Test_VerifyEmail tests email verification with token
func Test_VerifyEmail(t *testing.T) {
	ctx := context.Background()

	t.Run("successful email verification", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		token := "valid-token"
		userID := "user-123"

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "email_verification_tokens" WHERE token = $1 AND expires_at > $2 ORDER BY "email_verification_tokens"."id" LIMIT $3`)).
			WithArgs(token, sqlmock.AnyArg(), 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "token", "expires_at"}).
				AddRow("token-id", userID, token, time.Now().Add(1*time.Hour)))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE id = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "email_verified"}).
				AddRow(userID, "user@example.com", false))

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "users" SET "email_verified"=$1,"updated_at"=$2 WHERE "id" = $3`)).
			WithArgs(true, sqlmock.AnyArg(), userID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "email_verification_tokens" WHERE "email_verification_tokens"."id" = $1`)).
			WithArgs("token-id").
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		req := &VerifyEmailRequest{Token: token}
		err := svc.VerifyEmail(ctx, req)

		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("invalid token", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "email_verification_tokens" WHERE token = $1 AND expires_at > $2 ORDER BY "email_verification_tokens"."id" LIMIT $3`)).
			WithArgs("invalid-token", sqlmock.AnyArg(), 1).
			WillReturnError(gorm.ErrRecordNotFound)

		req := &VerifyEmailRequest{Token: "invalid-token"}
		err := svc.VerifyEmail(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid or expired verification token")
	})

	t.Run("expired token", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		// Token expired 1 hour ago
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "email_verification_tokens" WHERE token = $1 AND expires_at > $2 ORDER BY "email_verification_tokens"."id" LIMIT $3`)).
			WithArgs("expired-token", sqlmock.AnyArg(), 1).
			WillReturnError(gorm.ErrRecordNotFound)

		req := &VerifyEmailRequest{Token: "expired-token"}
		err := svc.VerifyEmail(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid or expired")
	})

	t.Run("already verified user - idempotent", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		token := "valid-token"
		userID := "user-123"

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "email_verification_tokens" WHERE token = $1 AND expires_at > $2 ORDER BY "email_verification_tokens"."id" LIMIT $3`)).
			WithArgs(token, sqlmock.AnyArg(), 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "token"}).
				AddRow("token-id", userID, token))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE id = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "email_verified"}).
				AddRow(userID, "user@example.com", true))

		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "email_verification_tokens" WHERE "email_verification_tokens"."id" = $1`)).
			WithArgs("token-id").
			WillReturnResult(sqlmock.NewResult(1, 1))

		req := &VerifyEmailRequest{Token: token}
		err := svc.VerifyEmail(ctx, req)

		require.NoError(t, err)
	})

	t.Run("empty token", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		req := &VerifyEmailRequest{Token: ""}
		err := svc.VerifyEmail(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid request")
	})

	t.Run("nil request", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		err := svc.VerifyEmail(ctx, nil)
		assert.Error(t, err)
	})

	t.Run("user not found", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		token := "orphan-token"

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "email_verification_tokens" WHERE token = $1 AND expires_at > $2 ORDER BY "email_verification_tokens"."id" LIMIT $3`)).
			WithArgs(token, sqlmock.AnyArg(), 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "token"}).
				AddRow("token-id", "nonexistent-user", token))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE id = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs("nonexistent-user", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		req := &VerifyEmailRequest{Token: token}
		err := svc.VerifyEmail(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "user not found")
	})

	t.Run("database error on verification", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		token := "valid-token"
		userID := "user-123"

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "email_verification_tokens" WHERE token = $1 AND expires_at > $2 ORDER BY "email_verification_tokens"."id" LIMIT $3`)).
			WithArgs(token, sqlmock.AnyArg(), 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "token"}).
				AddRow("token-id", userID, token))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE id = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "email_verified"}).
				AddRow(userID, "user@example.com", false))

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "users" SET "email_verified"=$1,"updated_at"=$2 WHERE "id" = $3`)).
			WillReturnError(sql.ErrConnDone)
		mock.ExpectRollback()

		req := &VerifyEmailRequest{Token: token}
		err := svc.VerifyEmail(ctx, req)

		assert.Error(t, err)
	})

	t.Run("token reuse prevention - single use", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		token := "used-token"

		// First use
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "email_verification_tokens" WHERE token = $1 AND expires_at > $2 ORDER BY "email_verification_tokens"."id" LIMIT $3`)).
			WithArgs(token, sqlmock.AnyArg(), 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "token"}).
				AddRow("token-id", "user-123", token))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE id = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs("user-123", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "email_verified"}).
				AddRow("user-123", "user@example.com", false))

		mock.ExpectBegin()
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "users" SET "email_verified"=$1,"updated_at"=$2 WHERE "id" = $3`)).
			WithArgs(true, sqlmock.AnyArg(), "user-123").
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "email_verification_tokens" WHERE "email_verification_tokens"."id" = $1`)).
			WithArgs("token-id").
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		req := &VerifyEmailRequest{Token: token}
		err := svc.VerifyEmail(ctx, req)
		require.NoError(t, err)

		// Second use - token should be gone
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "email_verification_tokens" WHERE token = $1 AND expires_at > $2 ORDER BY "email_verification_tokens"."id" LIMIT $3`)).
			WithArgs(token, sqlmock.AnyArg(), 1).
			WillReturnError(gorm.ErrRecordNotFound)

		err = svc.VerifyEmail(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid or expired")
	})
}

// Test_TokenSecurityProperties tests OWASP security requirements
func Test_TokenSecurityProperties(t *testing.T) {
	t.Run("token is cryptographically secure", func(t *testing.T) {
		token1, err := generateSecureToken(32)
		require.NoError(t, err)

		token2, err := generateSecureToken(32)
		require.NoError(t, err)

		// Tokens should be unique
		assert.NotEqual(t, token1, token2)

		// Token should be 64 hex chars (32 bytes)
		assert.Equal(t, 64, len(token1))
		assert.Regexp(t, regexp.MustCompile("^[a-f0-9]{64}$"), token1)
	})

	t.Run("token has sufficient entropy", func(t *testing.T) {
		// Generate multiple tokens and ensure they're all different
		tokens := make(map[string]bool)
		for i := 0; i < 100; i++ {
			token, err := generateSecureToken(32)
			require.NoError(t, err)
			assert.False(t, tokens[token], "duplicate token generated")
			tokens[token] = true
		}
	})
}
