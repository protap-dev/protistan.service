package user

import (
	"context"
	"time"

	"encore.app/core/cache"
)

// UserProfileCache defines the interface for caching user profiles
type UserProfileCache interface {
	GetProfile(ctx context.Context, userID string) (*UserProfile, bool)
	SetProfile(ctx context.Context, userID string, profile *UserProfile, ttl time.Duration) error
}

// userProfileCacheAdapter adapts the core cache manager for user profile specific operations
type userProfileCacheAdapter struct {
	coreCache cache.CacheManager
}

// NewUserProfileCache creates a user profile cache using the core cache manager
func NewUserProfileCache(cacheManager cache.CacheManager) UserProfileCache {
	return &userProfileCacheAdapter{coreCache: cacheManager}
}

// GetProfile retrieves a cached user profile by user ID
func (u *userProfileCacheAdapter) GetProfile(ctx context.Context, userID string) (*UserProfile, bool) {
	data, found := u.coreCache.Get(ctx, userID)
	if !found {
		return nil, false
	}

	profile, ok := data.(*UserProfile)
	if !ok {
		return nil, false
	}

	return profile, true
}

// SetProfile stores a user profile in cache with TTL
func (u *userProfileCacheAdapter) SetProfile(ctx context.Context, userID string, profile *UserProfile, ttl time.Duration) error {
	return u.coreCache.Set(ctx, userID, profile, ttl)
}
