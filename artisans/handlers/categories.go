package handlers

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"encore.app/artisans/domain"
	"encore.app/artisans/internal"
	"encore.app/core/cache"
)

// CategoriesResponse represents the response for getting all categories
type CategoriesResponse struct {
	Categories []domain.ServiceCategory `json:"categories"`
	Count      int                      `json:"count"`
	CachedAt   *time.Time               `json:"cached_at,omitempty"`
}

// GetServicesRequest represents the request for getting services with optional filters
type GetServicesRequest struct {
	CategoryID   []string `json:"category_id,omitempty" query:"category_id"` // Support multiple category IDs
	ExcludeEmpty bool     `json:"exclude_empty" query:"exclude_empty"`       // Exclude categories with no services (default: false)
}

// ServicesResponseEnhanced represents an enhanced response with metadata
type ServicesResponseEnhanced struct {
	Categories []domain.ServiceCategoryWithServices `json:"categories"`
	Metadata   ResponseMetadata                     `json:"metadata"`
}

// ResponseMetadata provides additional response information
type ResponseMetadata struct {
	TotalCategories int        `json:"total_categories"`
	TotalServices   int        `json:"total_services"`
	CachedAt        *time.Time `json:"cached_at,omitempty"`
	GeneratedAt     time.Time  `json:"generated_at"`
}

// CachedServicesData wraps services data with cache metadata
type CachedServicesData struct {
	Categories []domain.ServiceCategoryWithServices `json:"categories"`
	CachedAt   time.Time                            `json:"cached_at"`
}

// CategoriesHandler handles service category operations
type CategoriesHandler struct {
	ratesRepo domain.RatesRepository
	logger    internal.Logger
	cache     cache.CacheManager
}

// NewCategoriesHandler creates a new categories handler
func NewCategoriesHandler(ratesRepo domain.RatesRepository, logger internal.Logger, cache cache.CacheManager) *CategoriesHandler {
	return &CategoriesHandler{
		ratesRepo: ratesRepo,
		logger:    logger,
		cache:     cache,
	}
}

// GetAllCategories handles GET /artisans/categories
func (h *CategoriesHandler) GetAllCategories(ctx context.Context) (*CategoriesResponse, error) {
	h.logger.LogUserAction(ctx, "get_all_categories", "public_request")

	categories, err := h.ratesRepo.GetAllCategories(ctx)
	if err != nil {
		h.logger.LogError(ctx, "get_categories_failed", err)
		return nil, internal.ErrDatabaseError
	}

	h.logger.LogUserAction(ctx, "get_categories_success", "success")
	return &CategoriesResponse{
		Categories: categories,
		Count:      len(categories),
	}, nil
}

// GetAllServices handles GET /artisans/services with caching for scalability
func (h *CategoriesHandler) GetAllServices(ctx context.Context, req *GetServicesRequest) (*ServicesResponseEnhanced, error) {
	h.logger.LogUserAction(ctx, "get_all_services", "authenticated")

	// Validate request
	var categoryIDs []string
	includeEmpty := true // Default behavior: show empty categories

	if req != nil {
		categoryIDs = req.CategoryID
		if req.ExcludeEmpty {
			includeEmpty = false
		}
	}

	// Build cache key based on filters
	cacheKey := "services:all_grouped"
	if len(categoryIDs) > 0 {
		// Create a deterministic cache key for multiple category IDs
		sort.Strings(categoryIDs)
		cacheKey = "services:categories:" + strings.Join(categoryIDs, ",")
	}
	if !includeEmpty {
		cacheKey += ":non_empty"
	}

	const cacheTTL = 15 * time.Minute

	// Try to get from cache first
	cachedData, err := h.cache.GetOrLoad(ctx, cacheKey, cacheTTL, func() (any, error) {
		h.logger.LogUserAction(ctx, "get_services_cache_miss", "loading_from_db")

		// Pass category filter to repository for efficient SQL filtering
		var data []domain.ServiceCategoryWithServices
		var err error
		data, err = h.ratesRepo.GetAllServicesGroupedByCategory(ctx, includeEmpty, categoryIDs...)

		if err != nil {
			h.logger.LogError(ctx, "get_services_db_failed", err)
			return nil, err
		}

		// Wrap with cache metadata
		cachedServicesData := CachedServicesData{
			Categories: data,
			CachedAt:   time.Now(),
		}

		h.logger.LogUserAction(ctx, "get_services_db_success", "cached")
		return cachedServicesData, nil
	})

	if err != nil {
		h.logger.LogError(ctx, "get_services_failed", err)
		return nil, err
	}

	// Type assertion with better error handling
	servicesData, ok := cachedData.(CachedServicesData)
	if !ok {
		h.logger.LogError(ctx, "get_services_type_assertion_failed", fmt.Errorf("expected CachedServicesData, got %T", cachedData))
		return nil, internal.ErrCacheError
	}

	// Calculate metadata
	totalServices := 0
	for _, category := range servicesData.Categories {
		totalServices += len(category.Services)
	}

	response := &ServicesResponseEnhanced{
		Categories: servicesData.Categories,
		Metadata: ResponseMetadata{
			TotalCategories: len(servicesData.Categories),
			TotalServices:   totalServices,
			CachedAt:        &servicesData.CachedAt,
			GeneratedAt:     time.Now(),
		},
	}

	h.logger.LogUserAction(ctx, "get_services_success", "success")

	return response, nil
}

// InvalidateServicesCache invalidates all services cache entries when data changes
func (h *CategoriesHandler) InvalidateServicesCache(ctx context.Context) error {
	const cachePrefix = "services:"

	h.logger.LogUserAction(ctx, "invalidate_services_cache", "cache_invalidation")
	return h.cache.DeleteByPrefix(ctx, cachePrefix)
}
