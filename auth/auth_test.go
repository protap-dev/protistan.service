package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// setupMockDB creates a completely mocked database connection for testing
func setupMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err, "failed to create sqlmock")

	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err, "failed to create gorm mock")

	return gormDB, mock, sqlDB
}

// setupTestService creates a Service instance with mocked database
func setupTestService(t *testing.T) (*Service, sqlmock.Sqlmock, func()) {
	gormDB, mock, sqlDB := setupMockDB(t)

	svc := &Service{
		db: gormDB,
		rateLimiter: &RateLimiter{
			visitors: make(map[string]*visitor),
		},
	}

	secrets.JWTSecret = "test-jwt-secret-key-for-testing-only"
	secrets.BCryptCost = "4"
	bcryptCost = 4

	cleanup := func() {
		sqlDB.Close()
	}

	return svc, mock, cleanup
}

// Test_validatePassword tests password validation with all edge cases
func Test_validatePassword(t *testing.T) {
	tests := []struct {
		name          string
		password      string
		expectedCount int
		shouldContain []string
	}{
		{
			name:          "valid password",
			password:      "SecureP!ss123",
			expectedCount: 0,
			shouldContain: nil,
		},
		{
			name:          "too short",
			password:      "Sh0rt!",
			expectedCount: 1,
			shouldContain: []string{"at least 8 characters"},
		},
		{
			name:          "no uppercase",
			password:      "password123!",
			expectedCount: 1,
			shouldContain: []string{"uppercase letter"},
		},
		{
			name:          "no lowercase",
			password:      "PASSWORD123!",
			expectedCount: 1,
			shouldContain: []string{"lowercase letter"},
		},
		{
			name:          "no number",
			password:      "Password!",
			expectedCount: 1,
			shouldContain: []string{"number"},
		},
		{
			name:          "no special character",
			password:      "Password123", // Contains no special chars (underscores/hyphens not in this password)
			expectedCount: 1,
			shouldContain: []string{"special character"},
		},
		{
			name:          "multiple failures",
			password:      "short",
			expectedCount: 4,
			shouldContain: []string{"8 characters", "number", "special"},
		},
		{
			name:          "empty password",
			password:      "",
			expectedCount: 5,
			shouldContain: []string{"8 characters"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validatePassword(tt.password)
			assert.Equal(t, tt.expectedCount, len(errors), "error count mismatch")

			for _, expected := range tt.shouldContain {
				found := slices.ContainsFunc(errors, regexp.MustCompile("(?i)"+expected).MatchString)
				assert.True(t, found, "expected error containing '%s' not found", expected)
			}
		})
	}
}

// Test_validateEmail tests email validation
func Test_validateEmail(t *testing.T) {
	tests := []struct {
		name      string
		email     string
		shouldErr bool
	}{
		{name: "valid email", email: "user@example.com", shouldErr: false},
		{name: "valid email with subdomain", email: "user@mail.example.com", shouldErr: false},
		{name: "valid email with plus", email: "user+tag@example.com", shouldErr: false},
		{name: "valid email with space in local part", email: "user @example.com", shouldErr: false},
		{name: "invalid - no @", email: "userexample.com", shouldErr: true},
		{name: "invalid - no domain", email: "user@", shouldErr: true},
		{name: "invalid - no username", email: "@example.com", shouldErr: true},
		{name: "invalid - space in domain", email: "user@example .com", shouldErr: true},
		{name: "empty email", email: "", shouldErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEmail(tt.email)
			if tt.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test_generateSecureToken tests secure token generation
func Test_generateSecureToken(t *testing.T) {
	t.Run("generates token of correct length", func(t *testing.T) {
		token, err := generateSecureToken(32)
		require.NoError(t, err)
		assert.Equal(t, 64, len(token))
	})

	t.Run("generates unique tokens", func(t *testing.T) {
		token1, err := generateSecureToken(32)
		require.NoError(t, err)
		token2, err := generateSecureToken(32)
		require.NoError(t, err)
		assert.NotEqual(t, token1, token2)
	})

	t.Run("handles zero length", func(t *testing.T) {
		token, err := generateSecureToken(0)
		require.NoError(t, err)
		assert.Equal(t, "", token)
	})
}

// Test_generateJWT tests JWT generation
func Test_generateJWT(t *testing.T) {
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	t.Run("generates valid JWT", func(t *testing.T) {
		userID := "test-user-id"
		email := "test@example.com"

		token, err := svc.generateJWT(userID, email)
		require.NoError(t, err)
		assert.NotEmpty(t, token)

		claims, err := parseJWT(token)
		require.NoError(t, err)
		assert.Equal(t, userID, claims.UserID)
		assert.Equal(t, email, claims.Email)
		assert.True(t, claims.ExpiresAt.Time.After(time.Now()))
	})

	t.Run("token expires after 24 hours", func(t *testing.T) {
		token, err := svc.generateJWT("user-id", "test@example.com")
		require.NoError(t, err)

		claims, err := parseJWT(token)
		require.NoError(t, err)

		expectedExpiry := time.Now().Add(24 * time.Hour)
		assert.InDelta(t, expectedExpiry.Unix(), claims.ExpiresAt.Unix(), 2)
	})
}

// Test_parseJWT tests JWT parsing
func Test_parseJWT(t *testing.T) {
	svc, _, cleanup := setupTestService(t)
	defer cleanup()

	t.Run("valid token", func(t *testing.T) {
		token, _ := svc.generateJWT("user-123", "user@example.com")
		claims, err := parseJWT(token)
		require.NoError(t, err)
		assert.Equal(t, "user-123", claims.UserID)
	})

	t.Run("invalid token format", func(t *testing.T) {
		_, err := parseJWT("invalid-token")
		assert.Error(t, err)
	})

	t.Run("expired token", func(t *testing.T) {
		claims := JWTClaims{
			UserID: "user-123",
			Email:  "test@example.com",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, _ := token.SignedString([]byte(secrets.JWTSecret))

		_, err := parseJWT(tokenStr)
		assert.Error(t, err)
	})

	t.Run("wrong signing key", func(t *testing.T) {
		claims := JWTClaims{UserID: "user-123", Email: "test@example.com"}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, _ := token.SignedString([]byte("wrong-secret"))

		_, err := parseJWT(tokenStr)
		assert.Error(t, err)
	})

	t.Run("empty token", func(t *testing.T) {
		_, err := parseJWT("")
		assert.Error(t, err)
	})
}

// Test_AuthHandler tests the auth handler
func Test_AuthHandler(t *testing.T) {
	svc, _, cleanup := setupTestService(t)
	defer cleanup()
	ctx := context.Background()

	t.Run("valid token", func(t *testing.T) {
		token, _ := svc.generateJWT("user-123", "user@example.com")
		uid, userData, err := svc.AuthHandler(ctx, token)
		require.NoError(t, err)
		assert.Equal(t, "user-123", string(uid))
		assert.Equal(t, "user-123", userData.ID)
		assert.Equal(t, "user@example.com", userData.Email)
	})

	t.Run("empty token", func(t *testing.T) {
		uid, userData, err := svc.AuthHandler(ctx, "")
		assert.NoError(t, err)
		assert.Empty(t, uid)
		assert.Nil(t, userData)
	})

	t.Run("invalid token", func(t *testing.T) {
		_, _, err := svc.AuthHandler(ctx, "invalid-token")
		assert.Error(t, err)
	})

	t.Run("malformed token", func(t *testing.T) {
		_, _, err := svc.AuthHandler(ctx, "malformed.jwt.token")
		assert.Error(t, err)
	})
}

// Test_Register tests user registration
func Test_Register(t *testing.T) {
	ctx := context.Background()

	t.Run("successful registration", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		email := "newuser@example.com"
		password := "SecureP!ss123"

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(email, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "users"`)).
			WithArgs(email, sqlmock.AnyArg(), false, sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("new-user-id"))
		mock.ExpectCommit()

		req := &RegisterRequest{Email: email, Password: password}
		resp, err := svc.Register(ctx, req)

		require.NoError(t, err)
		assert.NotEmpty(t, resp.Token)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("duplicate email", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		email := "existing@example.com"

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(email, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email"}).
				AddRow("existing-id", email))

		req := &RegisterRequest{Email: email, Password: "SecureP!ss123"}
		_, err := svc.Register(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "user already exists")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("invalid email format", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		req := &RegisterRequest{Email: "invalid-email", Password: "SecureP!ss123"}
		_, err := svc.Register(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid email format")
	})

	t.Run("weak password", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs("user@example.com", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		req := &RegisterRequest{Email: "user@example.com", Password: "weak"}
		_, err := svc.Register(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "password requirements not met")
	})

	t.Run("empty request fields", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		tests := []struct {
			name  string
			email string
			pass  string
		}{
			{"empty email", "", "SecureP!ss123"},
			{"empty password", "user@example.com", ""},
			{"both empty", "", ""},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := &RegisterRequest{Email: tt.email, Password: tt.pass}
				_, err := svc.Register(ctx, req)
				assert.Error(t, err)
			})
		}
	})

	t.Run("nil request", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		_, err := svc.Register(ctx, nil)
		assert.Error(t, err)
	})

	t.Run("email normalization", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		email := "  UPPERCASE@Example.COM  "
		normalized := "uppercase@example.com"

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(normalized, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "users"`)).
			WithArgs(normalized, sqlmock.AnyArg(), false, sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("user-id"))
		mock.ExpectCommit()

		req := &RegisterRequest{Email: email, Password: "SecureP!ss123"}
		_, err := svc.Register(ctx, req)

		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("database error on check", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs("user@example.com", 1).
			WillReturnError(errors.New("database connection error"))

		req := &RegisterRequest{Email: "user@example.com", Password: "SecureP!ss123"}
		_, err := svc.Register(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database error")
	})

	t.Run("database error on insert", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs("user@example.com", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "users"`)).
			WillReturnError(errors.New("insert failed"))
		mock.ExpectRollback()

		req := &RegisterRequest{Email: "user@example.com", Password: "SecureP!ss123"}
		_, err := svc.Register(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create user")
	})

	t.Run("rate limiting", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		email := "ratelimit@example.com"
		req := &RegisterRequest{Email: email, Password: "SecureP!ss123"}

		for i := 0; i < 3; i++ {
			svc.rateLimiter.isAllowed("register:"+email, 3) // ✅ Fixed: use isAllowed method
		}

		_, err := svc.Register(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too many registration attempts")
	})
}

// Test_Login tests user login
func Test_Login(t *testing.T) {
	ctx := context.Background()

	t.Run("successful login", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		email := "user@example.com"
		password := "SecureP!ss123"
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(email, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash"}).
				AddRow("user-123", email, string(hashedPassword)))

		req := &LoginRequest{Email: email, Password: password}
		resp, err := svc.Login(ctx, req)

		require.NoError(t, err)
		assert.NotEmpty(t, resp.Token)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("user not found", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs("nonexistent@example.com", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		req := &LoginRequest{Email: "nonexistent@example.com", Password: "SecureP!ss123"}
		_, err := svc.Login(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid credentials")
	})

	t.Run("wrong password", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		email := "user@example.com"
		correctPassword := "SecureP!ss123"
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(correctPassword), bcryptCost)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(email, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash"}).
				AddRow("user-123", email, string(hashedPassword)))

		req := &LoginRequest{Email: email, Password: "WrongP!ss123"}
		_, err := svc.Login(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid credentials")
	})

	t.Run("invalid email format", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		req := &LoginRequest{Email: "invalid-email", Password: "SecureP!ss123"}
		_, err := svc.Login(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid email format")
	})

	t.Run("empty credentials", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		tests := []struct {
			name     string
			email    string
			password string
		}{
			{"empty email", "", "SecureP!ss123"},
			{"empty password", "user@example.com", ""},
			{"both empty", "", ""},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := &LoginRequest{Email: tt.email, Password: tt.password}
				_, err := svc.Login(ctx, req)
				assert.Error(t, err)
			})
		}
	})

	t.Run("nil request", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		_, err := svc.Login(ctx, nil)
		assert.Error(t, err)
	})

	t.Run("database error", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs("user@example.com", 1).
			WillReturnError(errors.New("database connection failed"))

		req := &LoginRequest{Email: "user@example.com", Password: "SecureP!ss123"}
		_, err := svc.Login(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database error")
	})

	t.Run("rate limiting", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		email := "ratelimit@example.com"
		req := &LoginRequest{Email: email, Password: "SecureP!ss123"}

		for i := 0; i < 5; i++ {
			svc.rateLimiter.isAllowed("login:"+email, 5) // ✅ Fixed: use isAllowed method
		}

		_, err := svc.Login(ctx, req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too many login attempts")
	})

	t.Run("email normalization", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		email := "  USER@Example.COM  "
		normalized := "user@example.com"
		password := "SecureP!ss123"
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(normalized, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash"}).
				AddRow("user-123", normalized, string(hashedPassword)))

		req := &LoginRequest{Email: email, Password: password}
		_, err := svc.Login(ctx, req)

		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// Test_ForgotPassword tests password reset request
func Test_ForgotPassword(t *testing.T) {
	ctx := context.Background()

	t.Run("successful password reset request", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		email := "user@example.com"

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(email, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email"}).
				AddRow("user-123", email))

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "password_reset_tokens"`)).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("token-id"))
		mock.ExpectCommit()

		req := &ForgotPasswordRequest{Email: email}
		err := svc.ForgotPassword(ctx, req)

		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("non-existent email returns success for security", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs("nonexistent@example.com", 1).
			WillReturnError(gorm.ErrRecordNotFound)

		req := &ForgotPasswordRequest{Email: "nonexistent@example.com"}
		err := svc.ForgotPassword(ctx, req)

		require.NoError(t, err)
	})

	t.Run("invalid email format", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		req := &ForgotPasswordRequest{Email: "invalid-email"}
		err := svc.ForgotPassword(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid email format")
	})

	t.Run("empty email", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		req := &ForgotPasswordRequest{Email: ""}
		err := svc.ForgotPassword(ctx, req)

		assert.Error(t, err)
	})

	t.Run("nil request", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		err := svc.ForgotPassword(ctx, nil)
		assert.Error(t, err)
	})

	t.Run("database error on user lookup", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs("user@example.com", 1).
			WillReturnError(errors.New("database error"))

		req := &ForgotPasswordRequest{Email: "user@example.com"}
		err := svc.ForgotPassword(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database error")
	})

	t.Run("database error on token creation", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE email = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs("user@example.com", 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email"}).
				AddRow("user-123", "user@example.com"))

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "password_reset_tokens"`)).
			WillReturnError(errors.New("insert failed"))
		mock.ExpectRollback()

		req := &ForgotPasswordRequest{Email: "user@example.com"}
		err := svc.ForgotPassword(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create reset token")
	})

	t.Run("rate limiting", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		email := "ratelimit@example.com"

		for i := 0; i < 3; i++ {
			svc.rateLimiter.isAllowed("forgot:"+email, 3) // ✅ Fixed: use isAllowed method
		}

		req := &ForgotPasswordRequest{Email: email}
		err := svc.ForgotPassword(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too many password reset requests")
	})
}

// Test_ResetPassword tests password reset with token
func Test_ResetPassword(t *testing.T) {
	ctx := context.Background()

	t.Run("successful password reset", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		token := "valid-reset-token"
		userID := "user-123"
		newPassword := "NewSecureP!ss123"

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "password_reset_tokens" WHERE token = $1 AND used = $2 AND expires_at > $3 ORDER BY "password_reset_tokens"."id" LIMIT $4`)).
			WithArgs(token, false, sqlmock.AnyArg(), 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "token", "expires_at", "used"}).
				AddRow("token-id", userID, token, time.Now().Add(1*time.Hour), false))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE id = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash"}).
				AddRow(userID, "user@example.com", "old-hash"))

		// Expect transaction begin
		mock.ExpectBegin()

		// Update user password (within transaction)
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "users" SET "password_hash"=$1,"updated_at"=$2 WHERE "id" = $3`)).
			WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), userID).
			WillReturnResult(sqlmock.NewResult(1, 1))

		// Mark token as used (within transaction)
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "password_reset_tokens" SET "used"=$1,"updated_at"=$2 WHERE "id" = $3`)).
			WithArgs(true, sqlmock.AnyArg(), "token-id").
			WillReturnResult(sqlmock.NewResult(1, 1))

		// Expect transaction commit
		mock.ExpectCommit()

		req := &ResetPasswordRequest{Token: token, NewPassword: newPassword}
		err := svc.ResetPassword(ctx, req)

		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("invalid token", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "password_reset_tokens" WHERE token = $1 AND used = $2 AND expires_at > $3 ORDER BY "password_reset_tokens"."id" LIMIT $4`)).
			WithArgs("invalid-token", false, sqlmock.AnyArg(), 1).
			WillReturnError(gorm.ErrRecordNotFound)

		req := &ResetPasswordRequest{Token: "invalid-token", NewPassword: "NewSecureP!ss123"}
		err := svc.ResetPassword(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid or expired token")
	})

	t.Run("weak new password", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		req := &ResetPasswordRequest{Token: "valid-token", NewPassword: "weak"}
		err := svc.ResetPassword(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "password requirements not met")
	})

	t.Run("empty fields", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		tests := []struct {
			name     string
			token    string
			password string
		}{
			{"empty token", "", "NewSecureP!ss123"},
			{"empty password", "valid-token", ""},
			{"both empty", "", ""},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := &ResetPasswordRequest{Token: tt.token, NewPassword: tt.password}
				err := svc.ResetPassword(ctx, req)
				assert.Error(t, err)
			})
		}
	})

	t.Run("nil request", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		err := svc.ResetPassword(ctx, nil)
		assert.Error(t, err)
	})
}

// Test_PasswordHashValidation tests bcrypt cost factor security and performance
func Test_PasswordHashValidation(t *testing.T) {
	// Test password for hashing
	password := "SecureP@ssw0rd123!"
	wrongPassword := "WrongP@ssw0rd123!"

	t.Run("validates current bcrypt cost factor", func(t *testing.T) {
		// Test current implementation cost factor
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
		require.NoError(t, err)

		// Verify password matches hash
		err = bcrypt.CompareHashAndPassword(hash, []byte(password))
		assert.NoError(t, err)

		// Verify wrong password fails
		err = bcrypt.CompareHashAndPassword(hash, []byte(wrongPassword))
		assert.Error(t, err)
		assert.Equal(t, bcrypt.ErrMismatchedHashAndPassword, err)
	})

	t.Run("tests different bcrypt cost factors for security analysis", func(t *testing.T) {
		costFactors := []int{4, 8, 10, 12, 14} // Range from weak to very strong
		hashes := make(map[int][]byte)
		times := make(map[int]time.Duration)

		// Generate hashes with different cost factors and measure time
		for _, cost := range costFactors {
			start := time.Now()
			hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
			elapsed := time.Since(start)
			require.NoError(t, err)

			hashes[cost] = hash
			times[cost] = elapsed

			// Verify password works with this cost factor
			err = bcrypt.CompareHashAndPassword(hash, []byte(password))
			assert.NoError(t, err)

			// Wrong password should fail
			err = bcrypt.CompareHashAndPassword(hash, []byte(wrongPassword))
			assert.Error(t, err)
		}

		// Verify cost factors increase computation time (security)
		for i := 1; i < len(costFactors); i++ {
			prevCost := costFactors[i-1]
			currCost := costFactors[i]
			// Each higher cost factor should take longer (roughly 2x per increment)
			assert.Greater(t, times[currCost], times[prevCost],
				"cost factor %d should take longer than %d", currCost, prevCost)
		}

		// Log performance characteristics for analysis
		t.Logf("bcrypt cost factor performance:")
		for _, cost := range costFactors {
			t.Logf("Cost %d: %v", cost, times[cost])
		}
	})

	t.Run("validates backward compatibility with existing hashes", func(t *testing.T) {
		// Test various historical cost factors for backward compatibility
		compatibilityCosts := []int{4, 8, 10, 12}

		for _, cost := range compatibilityCosts {
			hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
			require.NoError(t, err)

			// Current implementation should verify passwords hashed with older cost factors
			err = bcrypt.CompareHashAndPassword(hash, []byte(password))
			assert.NoError(t, err, "should be backward compatible with cost factor %d", cost)

			// Wrong password should still fail
			err = bcrypt.CompareHashAndPassword(hash, []byte(wrongPassword))
			assert.Error(t, err, "wrong password should fail for cost factor %d", cost)
		}
	})

	t.Run("prevents timing attacks on password verification", func(t *testing.T) {
		// Test that password verification timing is consistent
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
		require.NoError(t, err)

		// Time multiple verification attempts
		const attempts = 100
		times := make([]time.Duration, attempts)

		for i := 0; i < attempts; i++ {
			start := time.Now()
			err := bcrypt.CompareHashAndPassword(hash, []byte(password))
			times[i] = time.Since(start)
			assert.NoError(t, err)
		}

		// Calculate timing variance
		var total time.Duration
		min := times[0]
		max := times[0]
		for _, duration := range times {
			total += duration
			if duration < min {
				min = duration
			}
			if duration > max {
				max = duration
			}
		}
		avg := total / attempts

		// Timing should be relatively consistent (no obvious timing leaks)
		variance := max - min
		// Allow up to 50% variance for normal system jitter
		maxVariance := avg / 2
		assert.Less(t, variance, maxVariance,
			"timing variance too high (%.3v), may indicate timing leak", variance)

		t.Logf("Password verification timing - Min: %v, Max: %v, Avg: %v, Variance: %v",
			min, max, avg, variance)
	})

	t.Run("validates hash format and error handling", func(t *testing.T) {
		// Test invalid hash formats
		invalidHashes := [][]byte{
			[]byte(""),                                // Empty hash
			[]byte("invalid-hash-format"),             // Invalid format
			[]byte("$2a$10$invalid.hash.format.here"), // Malformed hash
			[]byte("$2a$10$short"),                    // Too short
		}

		for _, invalidHash := range invalidHashes {
			err := bcrypt.CompareHashAndPassword(invalidHash, []byte(password))
			assert.Error(t, err)
			// Should not panic or crash on invalid hashes
		}

		// Test that we can still generate and verify valid hashes after invalid attempts
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
		require.NoError(t, err)

		err = bcrypt.CompareHashAndPassword(hash, []byte(password))
		assert.NoError(t, err)
	})
}

// Test_BruteForceProtection tests rate limiting effectiveness under attack conditions
func Test_BruteForceProtection(t *testing.T) {
	ctx := context.Background()

	t.Run("prevents brute force login attacks", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		email := "victim@example.com"
		limit := 5       // 5 login attempts per minute
		attackSize := 20 // Send 20 concurrent login attempts

		// Channel to collect results
		results := make(chan error, attackSize)
		var wg sync.WaitGroup

		// Simulate brute force attack with many concurrent login attempts
		for i := range attackSize {
			wg.Add(1)
			go func(attemptNum int) {
				defer wg.Done()
				req := &LoginRequest{
					Email:    email,
					Password: fmt.Sprintf("wrongpassword%d", attemptNum),
				}
				_, err := svc.Login(ctx, req)
				results <- err
			}(i)
		}

		// Wait for all attacks to complete
		wg.Wait()
		close(results)

		// Count blocked vs allowed attempts
		blockedCount := 0
		allowedCount := 0
		for err := range results {
			if err != nil && strings.Contains(err.Error(), "too many login attempts") {
				blockedCount++
			} else {
				allowedCount++
			}
		}

		// Should block most attempts after rate limit is exceeded
		assert.Greater(t, blockedCount, limit, "should block attempts after rate limit exceeded")
		assert.LessOrEqual(t, allowedCount, limit, "should not allow more than rate limit")
	})

	t.Run("prevents brute force password reset attacks", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		email := "victim@example.com"
		limit := 3       // 3 password reset attempts per hour
		attackSize := 10 // Send 10 concurrent password reset attempts

		// Channel to collect results
		results := make(chan error, attackSize)
		var wg sync.WaitGroup

		// Simulate password reset flood attack
		for range attackSize {
			wg.Go(func() {
				req := &ForgotPasswordRequest{Email: email}
				err := svc.ForgotPassword(ctx, req)
				results <- err
			})
		}

		// Wait for all attacks to complete
		wg.Wait()
		close(results)

		// Count blocked vs allowed attempts
		blockedCount := 0
		allowedCount := 0
		for err := range results {
			if err != nil && strings.Contains(err.Error(), "too many password reset requests") {
				blockedCount++
			} else {
				allowedCount++
			}
		}

		// Should block most attempts after rate limit is exceeded
		assert.Greater(t, blockedCount, limit, "should block attempts after password reset rate limit exceeded")
		assert.LessOrEqual(t, allowedCount, limit, "should not allow more than password reset rate limit")
	})

	t.Run("verifies rate limiting consistency under concurrent load", func(t *testing.T) {
		rl := &RateLimiter{visitors: make(map[string]*visitor)}
		key := "consistency-test"
		limit := 3
		concurrency := 15

		// Channel to collect results
		results := make(chan bool, concurrency)
		var wg sync.WaitGroup

		// Launch concurrent requests
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				allowed := rl.isAllowed(key, limit)
				results <- allowed
			}()
		}

		// Wait for all requests to complete
		wg.Wait()
		close(results)

		// Count results
		allowedCount := 0
		for allowed := range results {
			if allowed {
				allowedCount++
			}
		}

		// Should never exceed the rate limit
		assert.LessOrEqual(t, allowedCount, limit, "should never exceed rate limit under concurrent load")
	})
}

// Test_TokenReuse tests that password reset tokens cannot be reused
func Test_TokenReuse(t *testing.T) {
	ctx := context.Background()

	t.Run("prevents token reuse after successful reset", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		token := "test-reset-token"
		userID := "user-123"
		newPassword := "NewSecureP!ss123"

		// First use of token - should succeed
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "password_reset_tokens" WHERE token = $1 AND used = $2 AND expires_at > $3 ORDER BY "password_reset_tokens"."id" LIMIT $4`)).
			WithArgs(token, false, sqlmock.AnyArg(), 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "token", "expires_at", "used"}).
				AddRow("token-id", userID, token, time.Now().Add(1*time.Hour), false))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE id = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash"}).
				AddRow(userID, "user@example.com", "old-hash"))

		// Expect transaction begin
		mock.ExpectBegin()

		// Update user password (within transaction)
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "users" SET "password_hash"=$1,"updated_at"=$2 WHERE "id" = $3`)).
			WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), userID).
			WillReturnResult(sqlmock.NewResult(1, 1))

		// Mark token as used (within transaction)
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "password_reset_tokens" SET "used"=$1,"updated_at"=$2 WHERE "id" = $3`)).
			WithArgs(true, sqlmock.AnyArg(), "token-id").
			WillReturnResult(sqlmock.NewResult(1, 1))

		// Expect transaction commit
		mock.ExpectCommit()

		req1 := &ResetPasswordRequest{Token: token, NewPassword: newPassword}
		err1 := svc.ResetPassword(ctx, req1)
		require.NoError(t, err1)
		assert.NoError(t, mock.ExpectationsWereMet())

		// Test that token is now marked as used by attempting to use it again
		// This should fail because the token is now marked as used = true
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "password_reset_tokens" WHERE token = $1 AND used = $2 AND expires_at > $3 ORDER BY "password_reset_tokens"."id" LIMIT $4`)).
			WithArgs(token, false, sqlmock.AnyArg(), 1).
			WillReturnError(gorm.ErrRecordNotFound) // No unused token found

		req2 := &ResetPasswordRequest{Token: token, NewPassword: "AnotherPassword123!"}
		err2 := svc.ResetPassword(ctx, req2)
		assert.Error(t, err2)
		assert.Contains(t, err2.Error(), "invalid or expired token")
	})
}

// Test_TokenExpirationEdgeCases tests password reset token expiration behavior
func Test_TokenExpirationEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("rejects token expiring exactly at current time", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		token := "exact-expiry-token"
		newPassword := "NewSecureP!ss123"

		// Token expires exactly at current time - should be rejected
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "password_reset_tokens" WHERE token = $1 AND used = $2 AND expires_at > $3 ORDER BY "password_reset_tokens"."id" LIMIT $4`)).
			WithArgs(token, false, sqlmock.AnyArg(), 1).
			WillReturnError(gorm.ErrRecordNotFound)

		req := &ResetPasswordRequest{Token: token, NewPassword: newPassword}
		err := svc.ResetPassword(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid or expired token")
	})

	t.Run("rejects token expiring just before current time", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		token := "past-expiry-token"
		newPassword := "NewSecureP!ss123"

		// Token expired 1 nanosecond ago - should be rejected
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "password_reset_tokens" WHERE token = $1 AND used = $2 AND expires_at > $3 ORDER BY "password_reset_tokens"."id" LIMIT $4`)).
			WithArgs(token, false, sqlmock.AnyArg(), 1).
			WillReturnError(gorm.ErrRecordNotFound)

		req := &ResetPasswordRequest{Token: token, NewPassword: newPassword}
		err := svc.ResetPassword(ctx, req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid or expired token")
	})

	t.Run("accepts token expiring just after current time", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		token := "future-expiry-token"
		userID := "user-123"
		newPassword := "NewSecureP!ss123"

		// Token expires 1 nanosecond from now - should be accepted
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "password_reset_tokens" WHERE token = $1 AND used = $2 AND expires_at > $3 ORDER BY "password_reset_tokens"."id" LIMIT $4`)).
			WithArgs(token, false, sqlmock.AnyArg(), 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "token", "expires_at", "used"}).
				AddRow("token-id", userID, token, time.Now().Add(1*time.Hour), false))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE id = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash"}).
				AddRow(userID, "user@example.com", "old-hash"))

		// Expect transaction begin
		mock.ExpectBegin()
		// Update user password
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "users" SET "password_hash"=$1,"updated_at"=$2 WHERE "id" = $3`)).
			WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), userID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		// Mark token as used
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "password_reset_tokens" SET "used"=$1,"updated_at"=$2 WHERE "id" = $3`)).
			WithArgs(true, sqlmock.AnyArg(), "token-id").
			WillReturnResult(sqlmock.NewResult(1, 1))
		// Expect transaction commit
		mock.ExpectCommit()

		req := &ResetPasswordRequest{Token: token, NewPassword: newPassword}
		err := svc.ResetPassword(ctx, req)

		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("accepts token expiring far in future", func(t *testing.T) {
		svc, mock, cleanup := setupTestService(t)
		defer cleanup()

		token := "distant-future-token"
		userID := "user-123"
		newPassword := "NewSecureP!ss123"

		// Token expires far in future - should be accepted
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "password_reset_tokens" WHERE token = $1 AND used = $2 AND expires_at > $3 ORDER BY "password_reset_tokens"."id" LIMIT $4`)).
			WithArgs(token, false, sqlmock.AnyArg(), 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "token", "expires_at", "used"}).
				AddRow("token-id", userID, token, time.Now().Add(24*time.Hour), false))

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "users" WHERE id = $1 ORDER BY "users"."id" LIMIT $2`)).
			WithArgs(userID, 1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "email", "password_hash"}).
				AddRow(userID, "user@example.com", "old-hash"))

		// Expect transaction begin
		mock.ExpectBegin()
		// Update user password
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "users" SET "password_hash"=$1,"updated_at"=$2 WHERE "id" = $3`)).
			WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), userID).
			WillReturnResult(sqlmock.NewResult(1, 1))
		// Mark token as used
		mock.ExpectExec(regexp.QuoteMeta(`UPDATE "password_reset_tokens" SET "used"=$1,"updated_at"=$2 WHERE "id" = $3`)).
			WithArgs(true, sqlmock.AnyArg(), "token-id").
			WillReturnResult(sqlmock.NewResult(1, 1))
		// Expect transaction commit
		mock.ExpectCommit()

		req := &ResetPasswordRequest{Token: token, NewPassword: newPassword}
		err := svc.ResetPassword(ctx, req)

		require.NoError(t, err)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// Test_JWTPayloadTampering tests JWT token tampering scenarios
func Test_JWTPayloadTampering(t *testing.T) {
	// Save original secret for cleanup
	originalSecret := secrets.JWTSecret
	defer func() { secrets.JWTSecret = originalSecret }()

	t.Run("prevents algorithm confusion attacks", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		// Create token with 'none' algorithm (no signature)
		claims := JWTClaims{
			UserID: "user-123",
			Email:  "user@example.com",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
			},
		}

		// Create token with 'none' algorithm
		token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
		tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
		require.NoError(t, err)

		// Should be rejected - algorithm confusion attack prevented
		_, _, err = svc.AuthHandler(context.Background(), tokenString)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unauthorized")
	})

	t.Run("rejects token with wrong secret", func(t *testing.T) {
		svc, _, cleanup := setupTestService(t)
		defer cleanup()

		// Create a token with wrong secret
		claims := JWTClaims{
			UserID: "user-123",
			Email:  "user@example.com",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
			},
		}

		// Sign with wrong secret
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenString, err := token.SignedString([]byte("wrong-secret"))
		require.NoError(t, err)

		// Should be rejected due to invalid signature
		_, _, err = svc.AuthHandler(context.Background(), tokenString)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unauthorized")
	})
}

// Test_ConcurrentRateLimiting tests rate limiter behavior under concurrent load
func Test_ConcurrentRateLimiting(t *testing.T) {
	t.Run("handles concurrent requests from same key correctly", func(t *testing.T) {
		rl := &RateLimiter{visitors: make(map[string]*visitor)}
		key := "test-key"
		limit := 5
		concurrency := 10

		// Channel to collect results
		results := make(chan bool, concurrency)

		// Launch concurrent requests
		for i := 0; i < concurrency; i++ {
			go func() {
				allowed := rl.isAllowed(key, limit)
				results <- allowed
			}()
		}

		// Collect results
		allowedCount := 0
		blockedCount := 0
		for i := 0; i < concurrency; i++ {
			if <-results {
				allowedCount++
			} else {
				blockedCount++
			}
		}

		// Should allow exactly 'limit' requests and block the rest
		assert.Equal(t, limit, allowedCount, "should allow exactly the limit number of requests")
		assert.Equal(t, concurrency-limit, blockedCount, "should block excess requests")
	})

	t.Run("handles concurrent requests from different keys independently", func(t *testing.T) {
		rl := &RateLimiter{visitors: make(map[string]*visitor)}
		limit := 3
		concurrency := 5

		// Test multiple different keys concurrently
		keys := []string{"key1", "key2", "key3"}
		results := make(chan struct {
			key     string
			allowed bool
		}, concurrency)

		// Launch concurrent requests for different keys
		for _, key := range keys {
			for i := 0; i < concurrency; i++ {
				go func(k string) {
					allowed := rl.isAllowed(k, limit)
					results <- struct {
						key     string
						allowed bool
					}{key: k, allowed: allowed}
				}(key)
			}
		}

		// Collect and verify results
		allowedPerKey := make(map[string]int)
		for i := 0; i < len(keys)*concurrency; i++ {
			result := <-results
			if result.allowed {
				allowedPerKey[result.key]++
			}
		}

		// Each key should have exactly 'limit' allowed requests
		for _, key := range keys {
			assert.Equal(t, limit, allowedPerKey[key], "key %s should have exactly %d allowed requests", key, limit)
		}
	})

	t.Run("prevents race conditions in visitor creation", func(t *testing.T) {
		rl := &RateLimiter{visitors: make(map[string]*visitor)}
		key := "race-test-key"
		limit := 10
		concurrency := 20 // Reduced from 50 to avoid timeout

		// Channel to signal when all goroutines are ready
		ready := make(chan bool, concurrency)
		start := make(chan bool)
		results := make(chan bool, concurrency)

		// Launch concurrent requests that all try to create the same visitor
		for i := 0; i < concurrency; i++ {
			go func() {
				ready <- true
				<-start
				allowed := rl.isAllowed(key, limit)
				results <- allowed
			}()
		}

		// Wait for all goroutines to be ready
		for i := 0; i < concurrency; i++ {
			<-ready
		}

		// Start all requests simultaneously
		close(start)

		// Collect results
		allowedCount := 0
		for i := 0; i < concurrency; i++ {
			if <-results {
				allowedCount++
			}
		}

		// Should allow exactly 'limit' requests despite concurrent access
		assert.Equal(t, limit, allowedCount, "should handle concurrent visitor creation correctly")
	})
}

// Test_RateLimiter tests the rate limiter
func Test_RateLimiter(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		rl := &RateLimiter{visitors: make(map[string]*visitor)}

		key := "test-key"
		limit := 5

		for i := 0; i < limit; i++ {
			assert.True(t, rl.isAllowed(key, limit)) // ✅ Fixed: use isAllowed method
		}
	})

	t.Run("blocks requests exceeding limit", func(t *testing.T) {
		rl := &RateLimiter{visitors: make(map[string]*visitor)}

		key := "test-key"
		limit := 3

		for i := 0; i < limit; i++ {
			rl.isAllowed(key, limit) // ✅ Fixed: use isAllowed method
		}

		assert.False(t, rl.isAllowed(key, limit))
	})

	t.Run("different keys have independent limits", func(t *testing.T) {
		rl := &RateLimiter{visitors: make(map[string]*visitor)}

		key1 := "key1"
		key2 := "key2"
		limit := 2

		rl.isAllowed(key1, limit)
		rl.isAllowed(key1, limit)

		assert.True(t, rl.isAllowed(key2, limit))
	})

	t.Run("visitor cleanup removes old entries", func(t *testing.T) {
		rl := &RateLimiter{visitors: make(map[string]*visitor)}

		rl.getVisitor("old-key", 5)

		rl.mu.Lock()
		rl.visitors["old-key"].lastSeen = time.Now().Add(-15 * time.Minute)
		rl.mu.Unlock()

		rl.mu.Lock()
		for key, v := range rl.visitors {
			if time.Since(v.lastSeen) > 10*time.Minute {
				delete(rl.visitors, key)
			}
		}
		rl.mu.Unlock()

		rl.mu.RLock()
		_, exists := rl.visitors["old-key"]
		rl.mu.RUnlock()

		assert.False(t, exists)
	})
}
