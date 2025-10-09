package user

import (
	"context"
	"errors"
	"time"

	"encore.app/core"
	"encore.app/core/db"
	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"encore.dev/types/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Global variable to allow testing override of auth.UserID
var authUserID func() (string, bool) = func() (string, bool) {
	uid, ok := auth.UserID()
	if !ok {
		return "", false
	}
	return string(uid), true
}

//encore:service
type Service struct {
	userRepo     UserRepository
	profileRepo  ProfileRepository
	settingsRepo SettingsRepository
	validator    UserValidator
	logger       ServiceLogger
	coreSvc      *core.CoreService // Core service for shared infrastructure
}

func initService() (*Service, error) {
	// Initialize GORM connection using Encore's database
	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: db.ProtisanDB.Stdlib(),
	}), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Initialize core service for shared infrastructure
	coreSvc := core.NewCoreService(gormDB)

	return &Service{
		userRepo:     NewUserRepository(coreSvc.DB()),
		profileRepo:  NewProfileRepository(coreSvc.DB()),
		settingsRepo: NewSettingsRepository(coreSvc.DB()),
		validator:    NewUserValidator(),
		logger:       NewServiceLogger(),
		coreSvc:      coreSvc,
	}, nil
}

// ============================================================================
// DOMAIN MODELS
// ============================================================================

type User struct {
	ID                  string     `json:"id" gorm:"primarykey;type:uuid;default:generate_uuid()"`
	Email               string     `json:"email"`
	PasswordHash        string     `json:"-" gorm:"column:password_hash"`
	EmailVerified       bool       `json:"email_verified"`
	UserType            string     `json:"user_type"`
	ProfileComplete     bool       `json:"profile_complete"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	FailedLoginAttempts int        `json:"-"`
	LockedUntil         *time.Time `json:"-"`
	LastFailedLogin     *time.Time `json:"-"`
}

type UserProfile struct {
	ID        string    `json:"id" gorm:"primarykey;type:uuid;default:generate_uuid()"`
	UserID    string    `json:"user_id"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Phone     string    `json:"phone"`
	AvatarURL string    `json:"avatar_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserSettings struct {
	ID                 string    `json:"id" gorm:"primarykey;type:uuid;default:generate_uuid()"`
	UserID             string    `json:"user_id"`
	EmailNotifications bool      `json:"email_notifications"`
	SMSNotifications   bool      `json:"sms_notifications"`
	PushNotifications  bool      `json:"push_notifications"`
	Language           string    `json:"language"`
	Timezone           string    `json:"timezone"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type CompleteUserProfile struct {
	User     User         `json:"user"`
	Profile  UserProfile  `json:"profile"`
	Settings UserSettings `json:"settings"`
}

// PublicUserProfile represents only publicly viewable user information
type PublicUserProfile struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	AvatarURL string `json:"avatar_url"`
}

// ============================================================================
// API REQUEST/RESPONSE TYPES
// ============================================================================

type UpdateProfileRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Phone     string `json:"phone"`
	AvatarURL string `json:"avatar_url"`
}

type UpdateSettingsRequest struct {
	EmailNotifications *bool   `json:"email_notifications,omitempty"`
	SMSNotifications   *bool   `json:"sms_notifications,omitempty"`
	PushNotifications  *bool   `json:"push_notifications,omitempty"`
	Language           *string `json:"language,omitempty"`
	Timezone           *string `json:"timezone,omitempty"`
}

// ============================================================================
// PUBLIC API ENDPOINTS
// ============================================================================

// Get complete user profile
//
//encore:api auth method=GET path=/v0/user/profile
func (s *Service) GetProfile(ctx context.Context) (*CompleteUserProfile, error) {
	userID, ok := authUserID()
	if !ok {
		return nil, ErrUnauthenticated
	}

	userIDStr := string(userID)
	s.logger.LogUserAction(ctx, "get_profile", userIDStr)

	return s.getCompleteProfile(ctx, userIDStr)
}

// Update user profile
//
//encore:api auth method=PUT path=/v0/user/profile
func (s *Service) UpdateProfile(ctx context.Context, req *UpdateProfileRequest) (*UserProfile, error) {
	userID, ok := authUserID()
	if !ok {
		return nil, ErrUnauthenticated
	}

	userIDStr := string(userID)
	s.logger.LogUserAction(ctx, "update_profile", userIDStr)

	// Use transaction for atomic profile update and profile_complete flag
	var profile *UserProfile
	err := core.WithTransaction(ctx, s.coreSvc.DB(), func(db *gorm.DB) ProfileRepository {
		return NewProfileRepository(db)
	}, func(profileRepo ProfileRepository) error {
		// Validation
		if req.FirstName != "" {
			if err := s.validator.ValidateName(req.FirstName, 100); err != nil {
				return err
			}
		}
		if req.LastName != "" {
			if err := s.validator.ValidateName(req.LastName, 255); err != nil {
				return err
			}
		}
		if req.Phone != "" {
			if err := s.validator.ValidatePhone(req.Phone); err != nil {
				return err
			}
		}

		// Upsert profile (create if doesn't exist, update if exists)
		profile = &UserProfile{
			UserID:    userIDStr,
			FirstName: req.FirstName,
			LastName:  req.LastName,
			Phone:     req.Phone,
			AvatarURL: req.AvatarURL,
		}

		if err := profileRepo.Upsert(ctx, profile); err != nil {
			s.logger.LogError(ctx, "upsert_profile", err)
			return errs.B().Msg("failed to update profile").Err()
		}

		return nil
	})

	if err != nil {
		s.logger.LogError(ctx, "update_profile_transaction", err)
		return nil, err
	}

	// Mark profile as complete if key fields are set (outside transaction for flexibility)
	if profile != nil && profile.FirstName != "" && profile.LastName != "" && profile.Phone != "" {
		if err := s.userRepo.UpdateProfileComplete(ctx, userIDStr, true); err != nil {
			s.logger.LogError(ctx, "update_profile_complete", err)
			// Don't fail the request for this, just log it
		}
	}

	// Reload profile to get updated_at timestamp
	reloadedProfile, err := s.profileRepo.GetByUserID(ctx, userIDStr)
	if err != nil {
		s.logger.LogError(ctx, "reload_profile", err)
		// Return the profile we updated in the transaction
		return profile, nil
	}

	return reloadedProfile, nil
}

// Update user settings
//
//encore:api auth method=PUT path=/v0/user/settings
func (s *Service) UpdateSettings(ctx context.Context, req *UpdateSettingsRequest) (*UserSettings, error) {
	userID, ok := authUserID()
	if !ok {
		return nil, ErrUnauthenticated
	}

	userIDStr := string(userID)
	s.logger.LogUserAction(ctx, "update_settings", userIDStr)

	// Use transaction for atomic settings update
	var settings *UserSettings
	err := core.WithTransaction(ctx, s.coreSvc.DB(), func(db *gorm.DB) SettingsRepository {
		return NewSettingsRepository(db)
	}, func(settingsRepo SettingsRepository) error {
		// Upsert settings (create if doesn't exist, update if exists)
		settings = &UserSettings{
			UserID:             userIDStr,
			EmailNotifications: true, // defaults
			SMSNotifications:   false,
			PushNotifications:  true,
			Language:           "en",
			Timezone:           "Africa/Lagos",
		}

		// Apply provided updates
		if req.EmailNotifications != nil {
			settings.EmailNotifications = *req.EmailNotifications
		}
		if req.SMSNotifications != nil {
			settings.SMSNotifications = *req.SMSNotifications
		}
		if req.PushNotifications != nil {
			settings.PushNotifications = *req.PushNotifications
		}
		if req.Language != nil {
			settings.Language = *req.Language
		}
		if req.Timezone != nil {
			settings.Timezone = *req.Timezone
		}

		// Check if any updates were provided
		hasUpdates := req.EmailNotifications != nil || req.SMSNotifications != nil ||
			req.PushNotifications != nil || req.Language != nil || req.Timezone != nil

		if hasUpdates {
			if err := settingsRepo.Upsert(ctx, settings); err != nil {
				s.logger.LogError(ctx, "upsert_settings", err)
				return errs.B().Msg("failed to update settings").Err()
			}
		}

		return nil
	})

	if err != nil {
		s.logger.LogError(ctx, "update_settings_transaction", err)
		return nil, err
	}

	// Reload settings to get updated_at timestamp (outside transaction for flexibility)
	reloadedSettings, err := s.settingsRepo.GetByUserID(ctx, userIDStr)
	if err != nil {
		s.logger.LogError(ctx, "reload_settings", err)
		// Return the settings we updated in the transaction
		return settings, nil
	}

	return reloadedSettings, nil
}

// ============================================================================
// INTERNAL APIs (for service-to-service calls)
// ============================================================================

// GetUserByID - Internal API for other services
//
//encore:api private method=GET path=/internal/user/:userID
func (s *Service) GetUserByID(ctx context.Context, userID uuid.UUID) (*User, error) {
	user, err := s.userRepo.GetByID(ctx, userID.String())
	if err != nil {
		s.logger.LogError(ctx, "get_user_by_id", err)
		return nil, err
	}
	return user, nil
}

// GetProfileByUserID - Internal API
//
//encore:api private method=GET path=/internal/user/:userID/profile
func (s *Service) GetProfileByUserID(ctx context.Context, userID uuid.UUID) (*UserProfile, error) {
	profile, err := s.profileRepo.GetByUserID(ctx, userID.String())
	if err != nil {
		s.logger.LogError(ctx, "get_profile_by_user_id", err)
		return nil, err
	}
	return profile, nil
}

// GetPublicProfileByUserID - Internal API for public data only
//
//encore:api private method=GET path=/internal/user/:userID/public
func (s *Service) GetPublicProfileByUserID(ctx context.Context, userID uuid.UUID) (*PublicUserProfile, error) {
	profile, err := s.profileRepo.GetByUserID(ctx, userID.String())
	if err != nil {
		s.logger.LogError(ctx, "get_public_profile_by_user_id", err)
		return nil, err
	}

	// Return only public fields - no private data
	publicProfile := &PublicUserProfile{
		FirstName: profile.FirstName,
		LastName:  profile.LastName,
		AvatarURL: profile.AvatarURL,
	}

	s.logger.LogUserAction(ctx, "get_public_profile", userID.String())
	return publicProfile, nil
}

// GetCompleteProfileByUserID - Internal API
//
//encore:api private method=GET path=/internal/user/:userID/complete
func (s *Service) GetCompleteProfileByUserID(ctx context.Context, userID uuid.UUID) (*CompleteUserProfile, error) {
	return s.getCompleteProfile(ctx, userID.String())
}

// ============================================================================
// PRIVATE HELPER METHODS
// ============================================================================

func (s *Service) getCompleteProfile(ctx context.Context, userID string) (*CompleteUserProfile, error) {
	// Use read transaction for consistency when fetching multiple related records
	var result *CompleteUserProfile
	err := core.WithReadTransaction(ctx, s.coreSvc.DB(), func(db *gorm.DB) UserRepository {
		return NewUserRepository(db)
	}, func(userRepo UserRepository) error {
		// Fetch user
		user, err := userRepo.GetByID(ctx, userID)
		if err != nil {
			return err
		}

		// Fetch or create profile and settings using the transaction repos
		profileRepo := NewProfileRepository(s.coreSvc.DB())
		settingsRepo := NewSettingsRepository(s.coreSvc.DB())

		// Fetch or create profile
		profile, err := profileRepo.GetByUserID(ctx, userID)
		if err != nil && errors.Is(err, ErrProfileNotFound) {
			// Create default profile
			profile = &UserProfile{UserID: userID}
			if err := profileRepo.Create(ctx, profile); err != nil {
				return errs.B().Msg("failed to create profile").Err()
			}
		} else if err != nil {
			return err
		}

		// Fetch or create settings
		settings, err := settingsRepo.GetByUserID(ctx, userID)
		if err != nil && errors.Is(err, ErrSettingsNotFound) {
			// Create default settings
			settings = &UserSettings{
				UserID:             userID,
				EmailNotifications: true,
				SMSNotifications:   false,
				PushNotifications:  true,
				Language:           "en",
				Timezone:           "Africa/Lagos",
			}
			if err := settingsRepo.Create(ctx, settings); err != nil {
				return errs.B().Msg("failed to create settings").Err()
			}
		} else if err != nil {
			return err
		}

		result = &CompleteUserProfile{
			User:     *user,
			Profile:  *profile,
			Settings: *settings,
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}
