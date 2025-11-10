package domain

import (
	"context"
	"errors"
	"fmt"
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

	// Execute entire operation within a transaction
	err := s.repo.WithTransaction(ctx, func(txRepo ArtisanRepository) error {
		// 1. Validate input first
		if err := s.validator.ValidateCreateProfile(input); err != nil {
			return err // Transaction will rollback
		}

		// 2. Verify user is an artisan (external API call)
		completeProfile, err := s.verifyArtisanUser(ctx, userCtx.UUID)
		if err != nil {
			return err // Transaction will rollback
		}

		// 3. Check if basic user profile is complete (prerequisite for artisan profile)
		if err := s.validateBasicProfileComplete(completeProfile); err != nil {
			return err // Transaction will rollback
		}

		// 4. Check if profile exists within transaction
		existing, err := txRepo.GetByID(ctx, userCtx.ID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err // Transaction will rollback
		}

		var artisan *ArtisanProfile

		// 3. Create or update artisan profile within transaction
		if existing != nil {
			// Update existing profile
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
				return err // Transaction will rollback
			}

			// Reload updated profile within transaction
			artisan, err = txRepo.GetByID(ctx, existing.ID)
			if err != nil {
				return err // Transaction will rollback
			}
		} else {
			// Create new profile
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
			}

			if err := txRepo.Create(ctx, artisan); err != nil {
				return err // Transaction will rollback
			}
		}

		// 4. Build complete response
		result = &CompleteProfile{
			User: UserData{
				ID:       completeProfile.User.ID,
				Email:    completeProfile.User.Email,
				UserType: completeProfile.User.UserType,
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

		return nil // Transaction will commit
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

	// Execute update operation within a transaction
	err := s.repo.WithTransaction(ctx, func(txRepo ArtisanRepository) error {
		// 1. Validate input first
		if err := s.validator.ValidateUpdateProfile(input); err != nil {
			return err // Transaction will rollback
		}

		// 2. Verify user is an artisan (external API call)
		_, err := s.verifyArtisanUser(ctx, userCtx.UUID)
		if err != nil {
			return err // Transaction will rollback
		}

		// 3. Check if basic user profile is complete (prerequisite for artisan profile operations)
		completeProfile, err := user.GetCompleteProfileByUserID(ctx, userCtx.UUID)
		if err != nil {
			return err // Transaction will rollback
		}
		if err := s.validateBasicProfileComplete(completeProfile); err != nil {
			return err // Transaction will rollback
		}

		// 4. Get existing profile within transaction
		existing, err := txRepo.GetByID(ctx, userCtx.ID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return internal.ErrArtisanNotFound
			}
			return internal.ErrDatabaseError
		}

		// 5. Build updates within transaction
		updates := s.buildUpdates(input)
		if len(updates) == 0 {
			// No updates needed, return existing profile
			result = existing
			return nil
		}

		// 6. Apply updates within transaction
		if err := txRepo.Update(ctx, existing.ID, updates); err != nil {
			return internal.ErrDatabaseError
		}

		// 7. Reload updated profile within transaction
		result, err = txRepo.GetByID(ctx, existing.ID)
		if err != nil {
			return internal.ErrDatabaseError
		}

		return nil // Transaction will commit
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

	// 2. Cache miss - fetch from user service using optimized public endpoint
	userUUID, err := uuid.FromString(userID)
	if err != nil {
		return nil, err
	}

	// 3. Use the new efficient public profile endpoint (only fetches public data)
	publicProfile, err := user.GetPublicProfileByUserID(ctx, userUUID)
	if err != nil {
		return nil, err
	}

	// 4. Convert to UserProfile format for compatibility
	profile := &user.UserProfile{
		FirstName: publicProfile.FirstName,
		LastName:  publicProfile.LastName,
		AvatarURL: publicProfile.AvatarURL,
		// Note: Phone is not included in public profile for privacy
	}

	// 5. Cache the result for future requests (5 minute TTL)
	cache.Set(userID, profile, 5*time.Minute)
	s.logger.LogUserAction(ctx, "public_profile_cached", userID)

	return profile, nil
}

// GetProfile retrieves an artisan's complete profile with read consistency
func (s *ProfileService) GetProfile(ctx context.Context, userCtx *internal.UserContext) (*CompleteProfile, error) {
	var result *CompleteProfile

	// Execute profile retrieval within a read-only transaction for consistency
	err := s.repo.WithReadTransaction(ctx, func(txRepo ArtisanRepository) error {
		// 1. Verify user is an artisan (external API call)
		completeProfile, err := s.verifyArtisanUser(ctx, userCtx.UUID)
		if err != nil {
			return err
		}

		// 2. Get artisan profile within transaction for consistency
		artisan, err := txRepo.GetByID(ctx, userCtx.ID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return internal.ErrArtisanNotFound
			}
			return internal.ErrDatabaseError
		}

		// 3. Build complete response within transaction
		result = &CompleteProfile{
			User: UserData{
				ID:       completeProfile.User.ID,
				Email:    completeProfile.User.Email,
				UserType: completeProfile.User.UserType,
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

		return nil // Transaction will commit
	})

	if err != nil {
		s.logger.LogError(ctx, "get_profile_transaction_failed", err)
		return nil, err
	}

	return result, nil
}

// Private helpers
func (s *ProfileService) verifyArtisanUser(ctx context.Context, userUUID uuid.UUID) (*user.CompleteUserProfile, error) {
	completeProfile, err := user.GetCompleteProfileByUserID(ctx, userUUID)
	if err != nil {
		s.logger.LogError(ctx, "get_user_profile", err)
		return nil, err
	}

	if completeProfile == nil {
		return nil, internal.ErrUnauthorizedAction
	}

	if completeProfile.User.UserType != "artisan" {
		return nil, internal.ErrUnauthorizedAction
	}

	return completeProfile, nil
}

// validateBasicProfileComplete checks if user's basic profile is complete before allowing artisan profile creation
func (s *ProfileService) validateBasicProfileComplete(completeProfile *user.CompleteUserProfile) error {
	var missingFields []string

	if strings.TrimSpace(completeProfile.Profile.FirstName) == "" {
		missingFields = append(missingFields, "first_name")
	}
	if strings.TrimSpace(completeProfile.Profile.LastName) == "" {
		missingFields = append(missingFields, "last_name")
	}
	if strings.TrimSpace(completeProfile.Profile.Phone) == "" {
		missingFields = append(missingFields, "phone")
	}
	if strings.TrimSpace(completeProfile.Profile.AvatarURL) == "" {
		missingFields = append(missingFields, "avatar_url")
	}

	if len(missingFields) > 0 {
		return fmt.Errorf("please complete your basic profile information first. The following fields are required: %s",
			strings.Join(missingFields, ", "))
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
	}

	if err := s.repo.Create(ctx, artisan); err != nil {
		s.logger.LogError(ctx, "create_profile", err)
		return nil, internal.ErrDatabaseError
	}

	s.logger.LogArtisanAction(ctx, "profile_created", artisan.ID, userID)
	return artisan, nil
}

func (s *ProfileService) updateExisting(ctx context.Context, existing *ArtisanProfile, input *CreateProfileInput, userID string) (*ArtisanProfile, error) {
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

	if err := s.repo.Update(ctx, existing.ID, updates); err != nil {
		s.logger.LogError(ctx, "update_existing_profile", err)
		return nil, internal.ErrDatabaseError
	}

	updated, err := s.repo.GetByID(ctx, existing.ID)
	if err != nil {
		s.logger.LogError(ctx, "reload_updated_profile", err)
		return nil, internal.ErrDatabaseError
	}

	s.logger.LogArtisanAction(ctx, "profile_updated", existing.ID, userID)
	return updated, nil
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
