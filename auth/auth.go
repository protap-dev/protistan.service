package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"encore.app/core"
	"encore.app/core/db"
	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

//encore:service
type Service struct {
	db          *gorm.DB
	rateLimiter *RateLimiter
	coreSvc     *core.CoreService // Core service for shared infrastructure
}

//encore:authhandler
func (s *Service) AuthHandler(ctx context.Context, token string) (auth.UID, *UserData, error) {
	if token == "" {
		return "", nil, nil
	}
	claims, err := parseJWT(token)
	if err != nil {
		return "", nil, errs.B().Msg("unauthorized: invalid token").Err()
	}
	return auth.UID(claims.UserID), &UserData{ID: claims.UserID, Email: claims.Email}, nil
}

func initService() (*Service, error) {
	// Initialize secrets
	initSecrets()

	// Initialize GORM database connection
	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: db.ProtisanDB.Stdlib(),
	}), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return nil, errs.B().Msg("failed to connect to database").Err()
	}

	// Initialize core service for shared infrastructure
	coreSvc := core.NewCoreService(gormDB)

	// Initialize rate limiter for auth endpoints
	rateLimiter := &RateLimiter{
		visitors: make(map[string]*visitor),
	}
	go rateLimiter.cleanupVisitors()

	return &Service{
		db:          gormDB,
		rateLimiter: rateLimiter,
		coreSvc:     coreSvc,
	}, nil
}

//encore:api public method=POST path=/v0/auth/register
func (s *Service) Register(ctx context.Context, req *RegisterRequest) (*AuthResponse, error) {
	if req == nil || req.Email == "" || req.Password == "" {
		return nil, errs.B().Msg("invalid request").Err()
	}

	// Rate limiting: max 3 registrations per hour per email
	if !s.rateLimiter.isAllowed("register:"+req.Email, 3) {
		return nil, errs.B().Msg("too many registration attempts, please try again later").Err()
	}

	// Normalize email to lowercase
	normalizedEmail := strings.ToLower(strings.TrimSpace(req.Email))

	// Validate email format first
	if err := validateEmail(normalizedEmail); err != nil {
		return nil, err
	}

	// Sanitize and validate user_type
	userType := strings.ToLower(strings.TrimSpace(req.UserType))
	if err := ValidateUserType(userType); err != nil {
		return nil, err
	}

	// Check if user already exists
	var existingUser User
	if err := s.db.Where("email = ?", normalizedEmail).First(&existingUser).Error; err == nil {
		return nil, errs.B().Msg("user already exists").Err()
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.B().Msg("database error").Err()
	}

	// Validate password strength
	if passwordErrors := validatePassword(req.Password); len(passwordErrors) > 0 {
		return nil, errs.B().Msg("password requirements not met: " + strings.Join(passwordErrors, ", ")).Err()
	}

	// Hash the password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcryptCost)
	if err != nil {
		return nil, errs.B().Msg("failed to hash password").Err()
	}

	// Create new user
	newUser := User{
		Email:           normalizedEmail,
		PasswordHash:    string(hashedPassword),
		EmailVerified:   false,
		UserType:        userType,
		ProfileComplete: userType == "customer",
	}

	if err := s.db.Create(&newUser).Error; err != nil {
		return nil, errs.B().Msg("failed to create user").Err()
	}

	// Generate JWT token
	token, err := s.generateJWT(newUser.ID, newUser.Email, newUser.UserType, newUser.ProfileComplete)
	if err != nil {
		return nil, errs.B().Msg("failed to generate token").Err()
	}

	refreshToken, err := s.createRefreshToken(newUser.ID) // or user.ID for login
	if err != nil {
		return nil, errs.B().Msg("failed to generate refresh token").Err()
	}

	return &AuthResponse{Token: token, RefreshToken: refreshToken}, nil
}

//encore:api public method=POST path=/v0/auth/login
func (s *Service) Login(ctx context.Context, req *LoginRequest) (*AuthResponse, error) {
	if req == nil || req.Email == "" || req.Password == "" {
		return nil, errs.B().Msg("invalid request").Err()
	}

	// Rate limiting: max 5 login attempts per minute per email
	if !s.rateLimiter.isAllowed("login:"+req.Email, 5) {
		return nil, errs.B().Msg("too many login attempts, please try again later").Err()
	}

	// Normalize and validate email format
	normalizedEmail := strings.ToLower(strings.TrimSpace(req.Email))
	if err := validateEmail(normalizedEmail); err != nil {
		return nil, err
	}

	// Find user by email
	var user User
	if err := s.db.Where("email = ?", normalizedEmail).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.B().Msg("invalid credentials").Err()
		}
		return nil, errs.B().Msg("database error").Err()
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, errs.B().Msg("invalid credentials").Err()
	}

	// Generate JWT token
	token, err := s.generateJWT(user.ID, user.Email, user.UserType, user.ProfileComplete)
	if err != nil {
		return nil, errs.B().Msg("failed to generate token").Err()
	}

	refreshToken, err := s.createRefreshToken(user.ID) // or user.ID for login
	if err != nil {
		return nil, errs.B().Msg("failed to generate refresh token").Err()
	}

	return &AuthResponse{Token: token, RefreshToken: refreshToken}, nil
}

//encore:api public method=POST path=/v0/auth/refresh
func (s *Service) Refresh(ctx context.Context, req *RefreshRequest) (*AuthResponse, error) {
	if req == nil || req.RefreshToken == "" {
		return nil, errs.B().Msg("invalid request").Err()
	}

	var refreshToken RefreshToken
	if err := s.db.Where("token = ? AND expires_at > ?", req.RefreshToken, time.Now()).First(&refreshToken).Error; err != nil {
		return nil, errs.B().Msg("invalid or expired refresh token").Err()
	}

	var user User
	if err := s.db.Where("id = ?", refreshToken.UserID).First(&user).Error; err != nil {
		return nil, errs.B().Msg("user not found").Err()
	}

	// Generate new access token
	newAccessToken, err := s.generateJWT(user.ID, user.Email, user.UserType, user.ProfileComplete)
	if err != nil {
		return nil, errs.B().Msg("failed to generate token").Err()
	}

	return &AuthResponse{
		Token:        newAccessToken,
		RefreshToken: req.RefreshToken, // Reuse same refresh token
	}, nil
}
