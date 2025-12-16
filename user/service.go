package user

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"encore.app/core"
	"encore.app/core/db"
	"encore.dev/beta/auth"
	"encore.dev/beta/errs"
	"encore.dev/types/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// StringArray is a custom type for scanning string arrays from the database.
type StringArray []string

// Scan implements the sql.Scanner interface for StringArray.
func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}
	sv, err := driver.String.ConvertValue(value)
	if err != nil {
		return fmt.Errorf("failed to scan StringArray: %v", err)
	}
	s, ok := sv.(string)
	if !ok {
		return fmt.Errorf("failed to scan StringArray: expected string, got %T", sv)
	}

	s = strings.Trim(s, "{}")
	if s == "" {
		*a = []string{}
		return nil
	}
	parts := strings.Split(s, ",")
	*a = StringArray(parts)
	return nil
}

// Value implements the driver.Valuer interface for StringArray.
func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	if len(a) == 0 {
		return "{}", nil
	}
	return fmt.Sprintf("{%s}", strings.Join(a, ",")), nil
}

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
	ID                  string      `json:"id" gorm:"primarykey;type:uuid;default:generate_uuid()"`
	Email               string      `json:"email"`
	PasswordHash        string      `json:"-" gorm:"column:password_hash"`
	EmailVerified       bool        `json:"email_verified"`
	Roles               StringArray `json:"roles" gorm:"type:text[];default:'{}'"`
	ActiveRole          *string     `json:"-" gorm:"column:active_role"`
	ProfileComplete     bool        `json:"profile_complete"`
	CreatedAt           time.Time   `json:"created_at"`
	UpdatedAt           time.Time   `json:"updated_at"`
	FailedLoginAttempts int         `json:"-"`
	LockedUntil         *time.Time  `json:"-"`
	LastFailedLogin     *time.Time  `json:"-"`
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
	User            User         `json:"user"`
	Profile         UserProfile  `json:"profile"`
	Settings        UserSettings `json:"settings"`
	OnboardingState string       `json:"onboarding_state"`
	MissingFields   []string     `json:"missing_fields"`
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

type UserProfileResponse struct {
	User struct {
		ID              string    `json:"id"`
		Email           string    `json:"email"`
		EmailVerified   bool      `json:"email_verified"`
		RolesEnabled    []string  `json:"roles_enabled"`
		ActiveRole      *string   `json:"active_role"`
		ProfileComplete bool      `json:"profile_complete"`
		CreatedAt       time.Time `json:"created_at"`
		UpdatedAt       time.Time `json:"updated_at"`
	} `json:"user"`

	Profile  UserProfile  `json:"profile"`
	Settings UserSettings `json:"settings"`

	OnboardingState string   `json:"onboarding_state"`
	MissingFields   []string `json:"missing_fields"`
}

func (s *Service) getProfileResponse(ctx context.Context, userID string) (*UserProfileResponse, error) {
	var out *UserProfileResponse

	err := s.coreSvc.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		u, p, st, err := s.getOrCreateUserProfileSettingsTx(ctx, tx, userID)
		if err != nil {
			return err
		}

		onboardingState, missing := s.computeOnboardingState(ctx, u, p)

		resp := &UserProfileResponse{
			Profile:         *p,
			Settings:        *st,
			OnboardingState: onboardingState,
			MissingFields:   missing,
		}

		resp.User.ID = u.ID
		resp.User.Email = u.Email
		resp.User.EmailVerified = u.EmailVerified
		resp.User.RolesEnabled = []string(u.Roles)
		resp.User.ActiveRole = u.ActiveRole
		resp.User.ProfileComplete = u.ProfileComplete
		resp.User.CreatedAt = u.CreatedAt
		resp.User.UpdatedAt = u.UpdatedAt

		out = resp
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ============================================================================
// PUBLIC API ENDPOINTS
// ============================================================================

// Get complete user profile
//
//encore:api auth method=GET path=/v0/user/profile
func (s *Service) GetProfile(ctx context.Context) (*UserProfileResponse, error) {
	userID, ok := authUserID()
	if !ok {
		return nil, ErrUnauthenticated
	}
	userIDStr := string(userID)
	s.logger.LogUserAction(ctx, "get_profile", userIDStr)

	return s.getProfileResponse(ctx, userIDStr)
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

		// Get existing profile
		var err error
		profile, err = profileRepo.GetByUserID(ctx, userIDStr)
		if err != nil {
			// If profile does not exist, create a new one
			if errors.Is(err, ErrProfileNotFound) {
				profile = &UserProfile{UserID: userIDStr}
			} else {
				s.logger.LogError(ctx, "get_profile_for_update", err)
				return errs.B().Msg("failed to get profile for update").Err()
			}
		}

		// Apply updates from request
		if req.FirstName != "" {
			profile.FirstName = req.FirstName
		}
		if req.LastName != "" {
			profile.LastName = req.LastName
		}
		if req.Phone != "" {
			profile.Phone = req.Phone
		}
		if req.AvatarURL != "" {
			profile.AvatarURL = req.AvatarURL
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

// computeOnboardingState determines the user's onboarding state and missing fields
func (s *Service) computeOnboardingState(ctx context.Context, user *User, profile *UserProfile) (string, []string) {
	var missing []string

	// -------------------------
	// 1) Basic profile checklist
	// -------------------------
	if strings.TrimSpace(profile.FirstName) == "" {
		missing = append(missing, "first_name")
	}
	if strings.TrimSpace(profile.LastName) == "" {
		missing = append(missing, "last_name")
	}
	if strings.TrimSpace(profile.Phone) == "" {
		missing = append(missing, "phone")
	}
	if strings.TrimSpace(profile.AvatarURL) == "" {
		missing = append(missing, "avatar_url")
	}
	if len(missing) > 0 {
		return "needs_basic_profile", missing
	}

	// --------------------------------
	// 2) Role selection
	// --------------------------------
	// New users can have no role yet.
	if len(user.Roles) == 0 {
		return "choose_role", []string{"role"}
	}

	// Resolve active role string safely (pointer may be nil)
	var activeRole string
	if user.ActiveRole != nil {
		activeRole = strings.TrimSpace(*user.ActiveRole)
	}

	if activeRole == "" {
		return "choose_active_role", []string{"active_role"}
	}

	// Ensure active_role is one of roles_enabled (defensive check).
	if !contains([]string(user.Roles), activeRole) {
		return "choose_active_role", []string{"active_role"}
	}

	// -------------------------
	// 3) Role-specific gating
	// -------------------------
	switch activeRole {
	case "customer":
		var addressCount int64
		if err := s.coreSvc.DB().WithContext(ctx).
			Table("customer_addresses").
			Where("user_id = ?", user.ID).
			Count(&addressCount).Error; err != nil {
			s.logger.LogError(ctx, "compute_onboarding_state_customer_address_check", err)
			// If DB check fails, keep UX safe: require address.
			return "needs_address", []string{"address"}
		}
		if addressCount == 0 {
			return "needs_address", []string{"address"}
		}
		return "ready_customer", []string{}

	case "artisan":
		// Query artisan record once and reuse the fields.
		type artisanRow struct {
			ID                 string
			RatesCount         int
			AvailabilityStatus string
		}
		var a artisanRow
		if err := s.coreSvc.DB().WithContext(ctx).
			Table("artisans").
			Select("id, rates_count, availability_status").
			Where("user_id = ?", user.ID).
			Take(&a).Error; err != nil {
			// Not found or DB error -> treat as missing profile
			return "needs_artisan_profile", []string{"artisan_profile"}
		}

		if a.RatesCount < 1 {
			return "needs_rates", []string{"rates"}
		}

		// Verification gating (use artisan_verifications table) – flag off for now.
		verificationRequired := false // TODO: wire from config when you want to enforce it
		if verificationRequired {
			var status string
			err := s.coreSvc.DB().WithContext(ctx).
				Table("artisan_verifications").
				Select("verification_status").
				Where("artisan_id = ?", a.ID).
				Scan(&status).Error

			// If there is no row yet or error, treat as unverified.
			if err != nil || strings.TrimSpace(status) == "" || status != "verified" {
				return "needs_verification", []string{"verification"}
			}
		}

		if a.AvailabilityStatus != "available" {
			return "needs_availability_on", []string{"availability"}
		}

		return "ready_artisan", []string{}

	default:
		// Unknown active_role: force user to pick a valid one.
		return "choose_active_role", []string{"active_role"}
	}
}
func contains(list []string, v string) bool {
	return slices.Contains(list, v)
}

func (s *Service) getCompleteProfile(ctx context.Context, userID string) (*CompleteUserProfile, error) {
	var result *CompleteUserProfile

	err := s.coreSvc.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		u, p, st, err := s.getOrCreateUserProfileSettingsTx(ctx, tx, userID)
		if err != nil {
			return err
		}

		onboardingState, missingFields := s.computeOnboardingState(ctx, u, p)

		result = &CompleteUserProfile{
			User:            *u,
			Profile:         *p,
			Settings:        *st,
			OnboardingState: onboardingState,
			MissingFields:   missingFields,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *Service) getOrCreateUserProfileSettingsTx(
	ctx context.Context,
	tx *gorm.DB,
	userID string,
) (*User, *UserProfile, *UserSettings, error) {
	userRepo := NewUserRepository(tx)
	profileRepo := NewProfileRepository(tx)
	settingsRepo := NewSettingsRepository(tx)

	// User must exist
	u, err := userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, nil, nil, err
	}

	// Profile: fetch or create
	p, err := profileRepo.GetByUserID(ctx, userID)
	if err != nil {
		if !errors.Is(err, ErrProfileNotFound) {
			return nil, nil, nil, err
		}
		p = &UserProfile{UserID: userID}
		if err := profileRepo.Create(ctx, p); err != nil {
			s.logger.LogError(ctx, "create_profile_in_get_or_create", err)
			return nil, nil, nil, err
		}
	}

	// Settings: fetch or create
	st, err := settingsRepo.GetByUserID(ctx, userID)
	if err != nil {
		if !errors.Is(err, ErrSettingsNotFound) {
			return nil, nil, nil, err
		}
		st = &UserSettings{
			UserID:             userID,
			EmailNotifications: true,
			SMSNotifications:   false,
			PushNotifications:  true,
			Language:           "en",
			Timezone:           "Africa/Lagos",
		}
		if err := settingsRepo.Create(ctx, st); err != nil {
			s.logger.LogError(ctx, "create_settings_in_get_or_create", err)
			return nil, nil, nil, err
		}
	}

	return u, p, st, nil
}
