package domain

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"encore.app/artisans/internal"
	"encore.app/core"
	"encore.app/user"
	"encore.dev/types/uuid"
)

// PublicProfileCache defines the interface for caching public user profiles
type PublicProfileCache interface {
	Get(userID string) (*user.UserProfile, bool)
	Set(userID string, profile *user.UserProfile, ttl time.Duration)
}

// publicProfileCacheAdapter adapts the core cache for public profile operations
type publicProfileCacheAdapter struct {
	coreCache interface {
		Get(ctx context.Context, key string) (any, bool)
		Set(ctx context.Context, key string, value any, ttl time.Duration) error
	}
}

// NewPublicProfileCache creates a public profile cache using the core cache manager
func NewPublicProfileCache(coreCache interface {
	Get(ctx context.Context, key string) (any, bool)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
}) PublicProfileCache {
	return &publicProfileCacheAdapter{coreCache: coreCache}
}

// Get retrieves a cached user profile by user ID
func (p *publicProfileCacheAdapter) Get(userID string) (*user.UserProfile, bool) {
	data, found := p.coreCache.Get(context.Background(), userID)
	if !found {
		return nil, false
	}

	profile, ok := data.(*user.UserProfile)
	if !ok {
		return nil, false
	}

	return profile, true
}

// Set stores a user profile in cache with TTL
func (p *publicProfileCacheAdapter) Set(userID string, profile *user.UserProfile, ttl time.Duration) {
	p.coreCache.Set(context.Background(), userID, profile, ttl)
}

type ProfileService struct {
	repo      ArtisanRepository
	validator *Validator
	logger    internal.Logger
	coreSvc   *core.CoreService // Core service for shared infrastructure
}

func NewProfileService(repo ArtisanRepository, validator *Validator, logger internal.Logger, coreSvc *core.CoreService) *ProfileService {
	return &ProfileService{
		repo:      repo,
		validator: validator,
		logger:    logger,
		coreSvc:   coreSvc,
	}
}

// CreateProfile creates or updates an artisan profile with transaction support
func (s *ProfileService) CreateProfile(ctx context.Context, userCtx *internal.UserContext, input *CreateProfileInput) (*CompleteProfile, error) {
	var result *CompleteProfile

	err := s.repo.WithTransaction(ctx, func(txRepo ArtisanRepository) error {
		// 1. Validate input first
		if err := s.validator.ValidateCreateProfile(input); err != nil {
			return err
		}

		// 2. Verify user is an artisan (external API call)
		completeProfile, err := s.verifyArtisanUser(ctx, userCtx.UUID)
		if err != nil {
			return err
		}

		// 3. Check if basic user profile is complete (prerequisite for artisan profile)
		if err := s.validateBasicProfileComplete(&completeProfile.Profile); err != nil {
			return err
		}

		// 4. Check if profile exists within transaction
		existing, err := txRepo.GetByID(ctx, userCtx.ID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}

		var artisan *ArtisanProfile

		if existing != nil {
			updates := map[string]any{
				"category_ids":           input.CategoryIDs,
				"bio":                    input.Bio,
				"years_experience":       input.YearsExperience,
				"languages":              input.Languages,
				"max_travel_distance_km": input.MaxTravelDistanceKm,
				"avatar_url":             input.AvatarURL,
				"preferred_city":         input.PreferredCity,
				"preferred_state":        input.PreferredState,
				"preferred_country":      input.PreferredCountry,
			}

			if err := txRepo.Update(ctx, existing.ID, updates); err != nil {
				return err
			}

			artisan, err = txRepo.GetByID(ctx, existing.ID)
			if err != nil {
				return err
			}
		} else {
			artisan = &ArtisanProfile{
				UserID:              userCtx.ID,
				CategoryIDs:         input.CategoryIDs,
				Bio:                 input.Bio,
				YearsExperience:     input.YearsExperience,
				Languages:           input.Languages,
				MaxTravelDistanceKm: input.MaxTravelDistanceKm,
				AvatarURL:           input.AvatarURL,
				PreferredCity:       input.PreferredCity,
				PreferredState:      input.PreferredState,
				PreferredCountry:    input.PreferredCountry,
				Verified:            false,
				Rating:              0.0,
				ReviewsCount:        0,
				AvailabilityStatus:  "unavailable",
			}

			if err := txRepo.Create(ctx, artisan); err != nil {
				return err
			}
		}

		result = &CompleteProfile{
			User: UserData{
				ID:         completeProfile.User.ID,
				Email:      completeProfile.User.Email,
				Roles:      completeProfile.User.Roles,
				ActiveRole: internal.GetActiveRole(completeProfile.User.ActiveRole),
			},
			Profile: ProfileData{
				FirstName: completeProfile.Profile.FirstName,
				LastName:  completeProfile.Profile.LastName,
				Phone:     completeProfile.Profile.Phone,
				AvatarURL: completeProfile.Profile.AvatarURL,
			},
			Settings: SettingsData{},
			Artisan:  *artisan,
		}

		return nil
	})

	if err != nil {
		s.logger.LogError(ctx, "create_profile_transaction_failed", err)
		return nil, err
	}

	if result != nil {
		s.logger.LogArtisanAction(ctx, "profile_created_or_updated", result.Artisan.ID, userCtx.ID)
	}
	return result, nil
}

// UpdateProfile updates an existing artisan profile with transaction support
func (s *ProfileService) UpdateProfile(ctx context.Context, userCtx *internal.UserContext, input *UpdateProfileInput) (*ArtisanProfile, error) {
	var result *ArtisanProfile

	err := s.repo.WithTransaction(ctx, func(txRepo ArtisanRepository) error {
		// 1) Validate input first
		if err := s.validator.ValidateUpdateProfile(input); err != nil {
			return err
		}

		// 2) Verify user is an artisan and get profile data in one call
		completeProfile, err := s.verifyArtisanUser(ctx, userCtx.UUID)
		if err != nil {
			return err
		}

		// 3) Check if basic user profile is complete
		if err := s.validateBasicProfileComplete(&completeProfile.Profile); err != nil {
			return err
		}

		// 4) Get existing profile within transaction
		existing, err := txRepo.GetByID(ctx, userCtx.ID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return internal.ErrArtisanNotFound
			}
			return internal.ErrDatabaseError
		}

		// 5) Build updates within transaction
		updates := s.buildUpdates(input)
		if len(updates) == 0 {
			result = existing
			return nil
		}

		// 6) Apply updates within transaction
		if err := txRepo.Update(ctx, existing.ID, updates); err != nil {
			return internal.ErrDatabaseError
		}

		// 7) Reload updated profile within transaction
		result, err = txRepo.GetByArtisanID(ctx, existing.ID)
		if err != nil {
			return internal.ErrDatabaseError
		}

		return nil
	})

	if err != nil {
		s.logger.LogError(ctx, "update_profile_transaction_failed", err)
		return nil, err
	}

	if result != nil {
		s.logger.LogArtisanAction(ctx, "profile_updated", result.ID, userCtx.ID)
	}
	return result, nil
}

// UpdateAvailability updates the artisan's availability status
func (s *ProfileService) UpdateAvailability(ctx context.Context, userCtx *internal.UserContext, status string) (*ArtisanProfile, error) {
	if status != ArtisanAvailabilityAvailable && status != ArtisanAvailabilityUnavailable {
		return nil, fmt.Errorf("invalid status: %s", status)
	}

	var result *ArtisanProfile

	err := s.repo.WithTransaction(ctx, func(txRepo ArtisanRepository) error {
		// 1. Verify user is an artisan
		_, err := s.verifyArtisanUser(ctx, userCtx.UUID)
		if err != nil {
			return err
		}

		// 2. Get existing profile
		existing, err := txRepo.GetByID(ctx, userCtx.ID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return internal.ErrArtisanNotFound
			}
			return internal.ErrDatabaseError
		}

		// 3. Update status
		updates := map[string]any{
			"availability_status": status,
		}

		if err := txRepo.Update(ctx, existing.ID, updates); err != nil {
			return internal.ErrDatabaseError
		}

		// 4. Reload updated profile
		result, err = txRepo.GetByArtisanID(ctx, existing.ID)
		if err != nil {
			return internal.ErrDatabaseError
		}

		return nil
	})

	if err != nil {
		s.logger.LogError(ctx, "update_availability_failed", err)
		return nil, err
	}

	s.logger.LogArtisanAction(ctx, "availability_updated", result.ID, userCtx.ID)
	return result, nil
}

// GetPublicArtisanProfile retrieves public artisan profile information by ID (no auth required)
func (s *ProfileService) GetPublicArtisanProfile(ctx context.Context, artisanID string) (*PublicArtisanProfile, error) {
	var result *PublicArtisanProfile

	// Execute operation within a transaction for consistency
	err := s.repo.WithReadTransaction(ctx, func(txRepo ArtisanRepository) error {
		// 1. Get artisan profile from repository within transaction
		artisan, err := txRepo.GetByID(ctx, artisanID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return internal.ErrArtisanNotFound
			}
			return internal.ErrDatabaseError
		}

		// 2. Get user profile information efficiently (external API call)
		userProfile, err := s.getPublicUserProfile(ctx, artisan.UserID)
		if err != nil {
			return err // Transaction will rollback
		}

		// 3. Build public response within transaction
		result = &PublicArtisanProfile{
			ArtisanID:           artisan.ID,
			CategoryIDs:         artisan.CategoryIDs,
			Bio:                 artisan.Bio,
			YearsExperience:     artisan.YearsExperience,
			Languages:           artisan.Languages,
			Rating:              artisan.Rating,
			ReviewsCount:        artisan.ReviewsCount,
			Verified:            artisan.Verified,
			MaxTravelDistanceKm: artisan.MaxTravelDistanceKm,
			AvatarURL:           artisan.AvatarURL,
			PreferredCity:       artisan.PreferredCity,
			PreferredState:      artisan.PreferredState,
			PreferredCountry:    artisan.PreferredCountry,
			UserFirstName:       userProfile.FirstName,
			UserLastName:        userProfile.LastName,
			AvailabilityStatus:  artisan.AvailabilityStatus,
		}

		return nil // Transaction will commit
	})

	if err != nil {
		s.logger.LogError(ctx, "get_public_profile_transaction_failed", err)
		return nil, err
	}

	s.logger.LogUserAction(ctx, "public_artisan_profile_viewed", artisanID)
	return result, nil
}

// getPublicUserProfile gets only publicly viewable user information with caching
func (s *ProfileService) getPublicUserProfile(ctx context.Context, userID string) (*user.UserProfile, error) {
	// 1. Check cache first for performance
	cache := NewPublicProfileCache(s.coreSvc.Cache())
	if cached, found := cache.Get(userID); found {
		s.logger.LogUserAction(ctx, "public_profile_cache_hit", userID)
		return cached, nil
	}

	// 2) Convert to UUID for user service request
	userUUID, err := uuid.FromString(userID)
	if err != nil {
		return nil, err
	}

	// 3) Fetch via consolidated internal API (public-only)
	resp, err := user.FetchInternal(ctx, &user.InternalUserFetchRequest{
		UserID:               userUUID,
		IncludePublicProfile: true,
		EnsureProfile:        false,
	})
	if err != nil {
		return nil, err
	}
	if resp.PublicProfile == nil {
		return nil, user.ErrProfileNotFound
	}

	// 4) Convert to UserProfile format for compatibility
	profile := &user.UserProfile{
		FirstName: resp.PublicProfile.FirstName,
		LastName:  resp.PublicProfile.LastName,
		AvatarURL: resp.PublicProfile.AvatarURL,
	}

	// 5) Cache and return
	cache.Set(userID, profile, 5*time.Minute)
	s.logger.LogUserAction(ctx, "public_profile_cached", userID)
	return profile, nil
}

// GetProfile retrieves an artisan's complete profile with read consistency
func (s *ProfileService) GetProfile(ctx context.Context, userCtx *internal.UserContext) (*CompleteProfile, error) {
	var result *CompleteProfile

	err := s.repo.WithReadTransaction(ctx, func(txRepo ArtisanRepository) error {
		// 1) Fetch user + profile in one RPC (no writes)
		resp, err := user.FetchInternal(ctx, &user.InternalUserFetchRequest{
			UserID:         userCtx.UUID,
			IncludeUser:    true,
			IncludeProfile: true,

			EnsureProfile:  false,
			EnsureSettings: false,
		})
		if err != nil {
			return err
		}
		if resp.User == nil {
			return internal.ErrUnauthorizedAction
		}
		if resp.Profile == nil {
			return user.ErrProfileNotFound
		}

		// Authorize by roles_enabled (not active_role)
		if !slices.Contains([]string(resp.User.Roles), "artisan") {
			return internal.ErrUnauthorizedAction
		}

		// 2) Get artisan profile within this service's read transaction
		artisan, err := txRepo.GetByID(ctx, userCtx.ID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return internal.ErrArtisanNotFound
			}
			return internal.ErrDatabaseError
		}

		// 3) Build response
		result = &CompleteProfile{
			User: UserData{
				ID:         resp.User.ID,
				Email:      resp.User.Email,
				Roles:      resp.User.Roles,
				ActiveRole: internal.GetActiveRole(resp.User.ActiveRole),
			},
			Profile: ProfileData{
				FirstName: resp.Profile.FirstName,
				LastName:  resp.Profile.LastName,
				Phone:     resp.Profile.Phone,
				AvatarURL: resp.Profile.AvatarURL,
			},
			Settings: SettingsData{},
			Artisan:  *artisan,
		}

		return nil
	})

	if err != nil {
		s.logger.LogError(ctx, "get_profile_transaction_failed", err)
		return nil, err
	}
	return result, nil
}

// Private helpers
func (s *ProfileService) verifyArtisanUser(ctx context.Context, userUUID uuid.UUID) (*user.CompleteUserProfile, error) {
	resp, err := user.FetchInternal(ctx, &user.InternalUserFetchRequest{
		UserID:         userUUID,
		IncludeUser:    true,
		IncludeProfile: true, // needed because callers use completeProfile.Profile

		EnsureProfile:  false,
		EnsureSettings: false,
	})
	if err != nil {
		s.logger.LogError(ctx, "fetch_user_for_verify_artisan", err)
		return nil, err
	}
	if resp.User == nil {
		return nil, internal.ErrUnauthorizedAction
	}

	// Authorize by roles_enabled (not active_role)
	if !slices.Contains([]string(resp.User.Roles), "artisan") {
		return nil, internal.ErrUnauthorizedAction
	}

	if resp.Profile == nil {
		// No side effects here; keep semantics explicit.
		return nil, user.ErrProfileNotFound
	}

	// Return a CompleteUserProfile since CreateProfile/GetProfile use it for response building.
	out := &user.CompleteUserProfile{
		User: user.User{
			ID:         resp.User.ID,
			Email:      resp.User.Email,
			Roles:      resp.User.Roles,
			ActiveRole: resp.User.ActiveRole,
		},
		Profile:  *resp.Profile,
		Settings: user.UserSettings{}, // not requested here
	}
	return out, nil
}

// validateBasicProfileComplete checks if user's basic profile is complete before allowing artisan profile creation
func (s *ProfileService) validateBasicProfileComplete(profile *user.UserProfile) error {
	var missingFields []string

	if profile == nil {
		return fmt.Errorf("please complete your basic profile information first")
	}

	if strings.TrimSpace(profile.FirstName) == "" {
		missingFields = append(missingFields, "first_name")
	}
	if strings.TrimSpace(profile.LastName) == "" {
		missingFields = append(missingFields, "last_name")
	}
	if strings.TrimSpace(profile.Phone) == "" {
		missingFields = append(missingFields, "phone")
	}
	if strings.TrimSpace(profile.AvatarURL) == "" {
		missingFields = append(missingFields, "avatar_url")
	}

	if len(missingFields) > 0 {
		return fmt.Errorf(
			"please complete your basic profile information first. The following fields are required: %s",
			strings.Join(missingFields, ", "),
		)
	}
	return nil
}

func (s *ProfileService) createNew(ctx context.Context, input *CreateProfileInput, userID string) (*ArtisanProfile, error) {
	artisan := &ArtisanProfile{
		UserID:              userID,
		CategoryIDs:         input.CategoryIDs,
		Bio:                 input.Bio,
		YearsExperience:     input.YearsExperience,
		Languages:           input.Languages,
		MaxTravelDistanceKm: input.MaxTravelDistanceKm,
		AvatarURL:           input.AvatarURL,
		PreferredCity:       input.PreferredCity,
		PreferredState:      input.PreferredState,
		PreferredCountry:    input.PreferredCountry,
		Verified:            false,
		Rating:              0.0,
		ReviewsCount:        0,
		AvailabilityStatus:  ArtisanAvailabilityUnavailable,
	}

	if err := s.repo.Create(ctx, artisan); err != nil {
		s.logger.LogError(ctx, "create_profile", err)
		return nil, internal.ErrDatabaseError
	}

	s.logger.LogArtisanAction(ctx, "profile_created", artisan.ID, userID)
	return artisan, nil
}

func (s *ProfileService) buildUpdates(input *UpdateProfileInput) map[string]any {
	updates := make(map[string]any)

	if input.CategoryIDs != nil {
		updates["category_ids"] = *input.CategoryIDs
	}
	if input.Bio != nil {
		updates["bio"] = *input.Bio
	}
	if input.YearsExperience != nil {
		updates["years_experience"] = *input.YearsExperience
	}
	if input.Languages != nil {
		updates["languages"] = *input.Languages
	}
	if input.MaxTravelDistanceKm != nil {
		updates["max_travel_distance_km"] = *input.MaxTravelDistanceKm
	}
	if input.AvatarURL != nil {
		updates["avatar_url"] = *input.AvatarURL
	}
	if input.Coordinates != nil {
		updates["coordinates"] = *input.Coordinates
	}
	if input.PreferredCity != nil {
		updates["preferred_city"] = *input.PreferredCity
	}
	if input.PreferredState != nil {
		updates["preferred_state"] = *input.PreferredState
	}
	if input.PreferredCountry != nil {
		updates["preferred_country"] = *input.PreferredCountry
	}

	return updates
}
