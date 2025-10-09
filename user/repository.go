package user

import (
	"context"
	"time"

	"encore.app/core"
	"gorm.io/gorm"
)

// Repository interfaces for data access abstraction

type UserRepository interface {
	GetByID(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	Create(ctx context.Context, user *User) error
	Update(ctx context.Context, id string, updates map[string]interface{}) error
	UpdateProfileComplete(ctx context.Context, id string, complete bool) error
}

type ProfileRepository interface {
	GetByUserID(ctx context.Context, userID string) (*UserProfile, error)
	Create(ctx context.Context, profile *UserProfile) error
	Update(ctx context.Context, userID string, updates map[string]interface{}) error
	Upsert(ctx context.Context, profile *UserProfile) error
}

type SettingsRepository interface {
	GetByUserID(ctx context.Context, userID string) (*UserSettings, error)
	Create(ctx context.Context, settings *UserSettings) error
	Update(ctx context.Context, userID string, updates map[string]interface{}) error
	Upsert(ctx context.Context, settings *UserSettings) error
}

// Repository implementations

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) GetByID(ctx context.Context, id string) (*User, error) {
	var user User
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) Create(ctx context.Context, user *User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *userRepository) Update(ctx context.Context, id string, updates map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Updates(updates).Error
}

func (r *userRepository) UpdateProfileComplete(ctx context.Context, id string, complete bool) error {
	return r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Update("profile_complete", complete).Error
}

// WithTransaction executes a function within a database transaction
func (r *userRepository) WithTransaction(ctx context.Context, fn func(UserRepository) error) error {
	return core.WithTransaction(ctx, r.db, func(db *gorm.DB) UserRepository {
		return &userRepository{db: db}
	}, fn)
}

// WithReadTransaction executes a function within a read-only transaction for consistency
func (r *userRepository) WithReadTransaction(ctx context.Context, fn func(UserRepository) error) error {
	return core.WithReadTransaction(ctx, r.db, func(db *gorm.DB) UserRepository {
		return &userRepository{db: db}
	}, fn)
}

// GetDB returns the underlying database connection for complex queries
func (r *userRepository) GetDB() *gorm.DB {
	return r.db
}

type profileRepository struct {
	db *gorm.DB
}

func NewProfileRepository(db *gorm.DB) ProfileRepository {
	return &profileRepository{db: db}
}

func (r *profileRepository) GetByUserID(ctx context.Context, userID string) (*UserProfile, error) {
	var profile UserProfile
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&profile).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrProfileNotFound
		}
		return nil, err
	}
	return &profile, nil
}

func (r *profileRepository) Create(ctx context.Context, profile *UserProfile) error {
	return r.db.WithContext(ctx).Create(profile).Error
}

func (r *profileRepository) Update(ctx context.Context, userID string, updates map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(&UserProfile{}).Where("user_id = ?", userID).Updates(updates).Error
}

func (r *profileRepository) Upsert(ctx context.Context, profile *UserProfile) error {
	// Try to update first, if no rows affected, create new
	result := r.db.WithContext(ctx).Model(&UserProfile{}).Where("user_id = ?", profile.UserID).Updates(map[string]interface{}{
		"first_name":  profile.FirstName,
		"last_name":   profile.LastName,
		"phone":       profile.Phone,
		"avatar_url":  profile.AvatarURL,
		"updated_at":  time.Now(),
	})

	if result.RowsAffected == 0 {
		// No existing profile, create new one
		return r.db.WithContext(ctx).Create(profile).Error
	}

	return result.Error
}

// WithTransaction executes a function within a database transaction
func (r *profileRepository) WithTransaction(ctx context.Context, fn func(ProfileRepository) error) error {
	return core.WithTransaction(ctx, r.db, func(db *gorm.DB) ProfileRepository {
		return &profileRepository{db: db}
	}, fn)
}

// WithReadTransaction executes a function within a read-only transaction for consistency
func (r *profileRepository) WithReadTransaction(ctx context.Context, fn func(ProfileRepository) error) error {
	return core.WithReadTransaction(ctx, r.db, func(db *gorm.DB) ProfileRepository {
		return &profileRepository{db: db}
	}, fn)
}

// GetDB returns the underlying database connection for complex queries
func (r *profileRepository) GetDB() *gorm.DB {
	return r.db
}

type settingsRepository struct {
	db *gorm.DB
}

func NewSettingsRepository(db *gorm.DB) SettingsRepository {
	return &settingsRepository{db: db}
}

func (r *settingsRepository) GetByUserID(ctx context.Context, userID string) (*UserSettings, error) {
	var settings UserSettings
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&settings).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrSettingsNotFound
		}
		return nil, err
	}
	return &settings, nil
}

func (r *settingsRepository) Create(ctx context.Context, settings *UserSettings) error {
	return r.db.WithContext(ctx).Create(settings).Error
}

func (r *settingsRepository) Update(ctx context.Context, userID string, updates map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(&UserSettings{}).Where("user_id = ?", userID).Updates(updates).Error
}

func (r *settingsRepository) Upsert(ctx context.Context, settings *UserSettings) error {
	// Try to update first, if no rows affected, create new
	result := r.db.WithContext(ctx).Model(&UserSettings{}).Where("user_id = ?", settings.UserID).Updates(map[string]interface{}{
		"email_notifications": settings.EmailNotifications,
		"sms_notifications":   settings.SMSNotifications,
		"push_notifications":  settings.PushNotifications,
		"language":           settings.Language,
		"timezone":           settings.Timezone,
		"updated_at":         time.Now(),
	})

	if result.RowsAffected == 0 {
		// No existing settings, create new ones
		return r.db.WithContext(ctx).Create(settings).Error
	}

	return result.Error
}

// WithTransaction executes a function within a database transaction
func (r *settingsRepository) WithTransaction(ctx context.Context, fn func(SettingsRepository) error) error {
	return core.WithTransaction(ctx, r.db, func(db *gorm.DB) SettingsRepository {
		return &settingsRepository{db: db}
	}, fn)
}

// WithReadTransaction executes a function within a read-only transaction for consistency
func (r *settingsRepository) WithReadTransaction(ctx context.Context, fn func(SettingsRepository) error) error {
	return core.WithReadTransaction(ctx, r.db, func(db *gorm.DB) SettingsRepository {
		return &settingsRepository{db: db}
	}, fn)
}

// GetDB returns the underlying database connection for complex queries
func (r *settingsRepository) GetDB() *gorm.DB {
	return r.db
}
