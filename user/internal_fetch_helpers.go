package user

import (
	"context"
	"errors"

	"encore.dev/beta/errs"
	"gorm.io/gorm"
)

func (s *Service) fetchUserThinTx(ctx context.Context, tx *gorm.DB, userID string) (*User, error) {
	var row User
	if err := tx.
		Table("users").
		Select("id, email, email_verified, roles, active_role, profile_complete, created_at, updated_at").
		Where("id = ?", userID).
		Take(&row).Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &row, nil
}

func (s *Service) toInternalUserDTO(u *User) *InternalUserDTO {
	if u == nil {
		return nil
	}
	return &InternalUserDTO{
		ID:              u.ID,
		Email:           u.Email,
		EmailVerified:   u.EmailVerified,
		Roles:           u.Roles,
		ActiveRole:      u.ActiveRole,
		ProfileComplete: u.ProfileComplete,
		CreatedAt:       u.CreatedAt,
		UpdatedAt:       u.UpdatedAt,
	}
}

func (s *Service) fetchPublicProfileOnlyTx(
	tx *gorm.DB,
	userID string,
	ensureProfile bool,
) (*PublicUserProfile, error) {
	var pub PublicUserProfile
	err := tx.
		Table("user_profiles").
		Select("first_name, last_name, avatar_url").
		Where("user_id = ?", userID).
		Take(&pub).Error

	if err == nil {
		return &pub, nil
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		if !ensureProfile {
			return nil, ErrProfileNotFound
		}
		// create empty profile row, return empty public payload (no early-return from caller)
		if err := tx.Create(&UserProfile{UserID: userID}).Error; err != nil {
			return nil, err
		}
		return &PublicUserProfile{}, nil
	}

	return nil, err
}

func (s *Service) fetchFullProfileTx(
	ctx context.Context,
	profileRepo ProfileRepository,
	userID string,
	ensureProfile bool,
) (*UserProfile, error) {
	prof, err := profileRepo.GetByUserID(ctx, userID)
	if err == nil {
		return prof, nil
	}
	if errors.Is(err, ErrProfileNotFound) && ensureProfile {
		prof = &UserProfile{UserID: userID}
		if err := profileRepo.Create(ctx, prof); err != nil {
			return nil, err
		}
		return prof, nil
	}
	return nil, err
}

func (s *Service) fetchSettingsTx(
	ctx context.Context,
	settingsRepo SettingsRepository,
	userID string,
	ensureSettings bool,
) (*UserSettings, error) {
	st, err := settingsRepo.GetByUserID(ctx, userID)
	if err == nil {
		return st, nil
	}
	if errors.Is(err, ErrSettingsNotFound) && ensureSettings {
		st = &UserSettings{
			UserID:             userID,
			EmailNotifications: true,
			SMSNotifications:   false,
			PushNotifications:  true,
			Language:           "en",
			Timezone:           "Africa/Lagos",
		}
		if err := settingsRepo.Create(ctx, st); err != nil {
			return nil, err
		}
		return st, nil
	}
	return nil, err
}

func (s *Service) requireOnboardingInputs(u *User, p *UserProfile) error {
	// With current needUser/needProfile computation, these should already be present;
	// keeping this as a defensive guard makes intent explicit.
	if u == nil || p == nil {
		return errs.B().Msg("include_onboarding requires user + full profile").Err()
	}
	return nil
}
