package domain

import (
	"context"
	"strings"
	"time"

	"encore.app/artisans/internal"
	"encore.app/user"
	"gorm.io/gorm"
)

// SearchService handles artisan search operations
type SearchService struct {
	repo   ArtisanRepository
	logger internal.Logger
}

// NewSearchService creates a new search service
func NewSearchService(repo ArtisanRepository, logger internal.Logger) *SearchService {
	return &SearchService{
		repo:   repo,
		logger: logger,
	}
}

// SearchInput represents search parameters
type SearchInput struct {
	Query         *string   // Full-text search query
	CategoryIDs   *[]string // Filter by service categories
	Location      *string   // Location-based search
	MaxDistanceKm *float64  // Max distance from search location
	MinRating     *float64  // Minimum rating filter
	MinReviews    *int      // Minimum reviews count
	Languages     *[]string // Filter by languages spoken
	Limit         *int      // Max results (default 20)
	Offset        *int      // Pagination offset (default 0)
}

// SearchResult represents a single search result
type SearchResult struct {
	Artisan    ArtisanProfile
	User       user.User
	Profile    user.UserProfile
	DistanceKm *float64 // Distance from search location
	Rank       float64  // Search relevance rank
}

// SearchResults contains all search results and metadata
type SearchResults struct {
	Results []SearchResult
	Total   int
	Query   string
	Filters SearchFilters
}

// SearchFilters represents applied search filters
type SearchFilters struct {
	CategoryIDs   []string
	Location      string
	MinRating     float64
	MinReviews    int
	Languages     []string
	MaxDistanceKm float64
	Limit         int
	Offset        int
}

// Search performs full-text search with advanced filtering using read-only transactions
func (s *SearchService) Search(ctx context.Context, input *SearchInput) (*SearchResults, error) {
	var result *SearchResults

	// Execute search within a read-only transaction for consistency
	err := s.repo.WithReadTransaction(ctx, func(txRepo ArtisanRepository) error {
		// Execute search query within transaction for consistency
		searchResults, total, err := s.executeSearchQuery(ctx, input)
		if err != nil {
			s.logger.LogError(ctx, "execute_search_query", err)
			return err
		}

		// Build response metadata within transaction
		query := s.extractQueryString(input)
		filters := s.buildFiltersSummary(input)

		result = &SearchResults{
			Results: searchResults,
			Total:   total,
			Query:   query,
			Filters: filters,
		}

		return nil // Transaction will commit
	})

	if err != nil {
		s.logger.LogError(ctx, "search_transaction_failed", err)
		return nil, err
	}

	return result, nil
}

// validateSearchInput validates search parameters
func (s *SearchService) validateSearchInput(input *SearchInput) error {
	// At least one search criteria must be provided
	if input.Query == nil && input.CategoryIDs == nil && input.Location == nil {
		return internal.ErrValidationFailed
	}

	// Validate query if provided
	if input.Query != nil && len(strings.TrimSpace(*input.Query)) < 2 {
		return internal.ErrValidationFailed
	}

	// Validate category IDs if provided
	if input.CategoryIDs != nil {
		for _, categoryID := range *input.CategoryIDs {
			if categoryID == "" {
				return internal.ErrValidationFailed
			}
			if len(categoryID) != 36 {
				return internal.ErrValidationFailed
			}
		}
	}

	// Validate rating range if provided
	if input.MinRating != nil {
		if *input.MinRating < 0 || *input.MinRating > 5 {
			return internal.ErrValidationFailed
		}
	}

	// Validate pagination parameters
	if input.Limit != nil && (*input.Limit < 1 || *input.Limit > 100) {
		return internal.ErrValidationFailed
	}
	if input.Offset != nil && *input.Offset < 0 {
		return internal.ErrValidationFailed
	}

	return nil
}

// executeSearchQuery builds and executes the search query with all filters
func (s *SearchService) executeSearchQuery(ctx context.Context, input *SearchInput) ([]SearchResult, int, error) {
	// Get database connection from repository
	db := s.repo.GetDB()

	// Build base query with joins
	query := db.WithContext(ctx).
		Table("artisans a")

	selectFields := `
		a.*,
		u.email, u.user_type, u.email_verified, u.profile_complete, u.created_at as user_created_at, u.updated_at as user_updated_at,
		up.first_name, up.last_name, up.phone, up.avatar_url as user_avatar_url
	`

	// Apply full-text search if query provided
	if input.Query != nil && strings.TrimSpace(*input.Query) != "" {
		searchTerm := strings.TrimSpace(*input.Query)
		query = query.Select(selectFields+", ts_rank(a.search_vector, plainto_tsquery('english', ?)) as rank", searchTerm)
		query = query.Where("a.search_vector @@ plainto_tsquery('english', ?)", searchTerm)
		query = query.Order("rank DESC")
	} else {
		query = query.Select(selectFields + ", 0 as rank")
	}

	query = query.Joins("JOIN users u ON a.user_id = u.id").
		Joins("LEFT JOIN user_profiles up ON u.id = up.user_id")

	// Apply category filter
	if input.CategoryIDs != nil && len(*input.CategoryIDs) > 0 {
		categoryList := *input.CategoryIDs
		query = query.Where("a.category_ids && ?", gorm.Expr("ARRAY[?]::uuid[]", categoryList))
	}

	// Apply location filter (simple text matching for now)
	if input.Location != nil && *input.Location != "" {
		location := strings.TrimSpace(*input.Location)
		query = query.Where("a.preferred_city ILIKE ? OR a.preferred_state ILIKE ? OR a.preferred_country ILIKE ?",
			"%"+location+"%", "%"+location+"%", "%"+location+"%")
	}

	// Apply rating filter
	if input.MinRating != nil {
		query = query.Where("a.rating >= ?", *input.MinRating)
	}

	// Apply reviews count filter
	if input.MinReviews != nil {
		query = query.Where("a.reviews_count >= ?", *input.MinReviews)
	}

	// Apply language filter, ignoring empty strings
	if input.Languages != nil && len(*input.Languages) > 0 {
		var validLangs []string
		for _, lang := range *input.Languages {
			if strings.TrimSpace(lang) != "" {
				validLangs = append(validLangs, lang)
			}
		}
		if len(validLangs) > 0 {
			query = query.Where("a.languages && ARRAY[?]", validLangs)
		}
	}

	// Get total count
	var total int64
	countQuery := query.Session(&gorm.Session{})
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, internal.ErrDatabaseError
	}

	// Apply pagination
	limit := 20 // default
	if input.Limit != nil {
		limit = *input.Limit
	}
	offset := 0
	if input.Offset != nil {
		offset = *input.Offset
	}

	query = query.Limit(limit).Offset(offset)

	// Execute query
	var results []struct {
		ArtisanProfile
		UserEmail           string    `gorm:"column:email"`
		UserType            string    `gorm:"column:user_type"`
		UserEmailVerified   bool      `gorm:"column:email_verified"`
		UserProfileComplete bool      `gorm:"column:profile_complete"`
		UserCreatedAt       time.Time `gorm:"column:user_created_at"`
		UserUpdatedAt       time.Time `gorm:"column:user_updated_at"`
		UserFirstName       string    `gorm:"column:first_name"`
		UserLastName        string    `gorm:"column:last_name"`
		UserPhone           string    `gorm:"column:phone"`
		UserAvatarURL       string    `gorm:"column:user_avatar_url"`
		Rank                float64   `gorm:"column:rank"`
	}

	if err := query.Find(&results).Error; err != nil {
		return nil, 0, internal.ErrDatabaseError
	}

	// Convert to response format
	searchResults := make([]SearchResult, len(results))
	for i, result := range results {
		searchResults[i] = SearchResult{
			Artisan: result.ArtisanProfile,
			User: user.User{
				ID:              result.UserID,
				Email:           result.UserEmail,
				UserType:        result.UserType,
				EmailVerified:   result.UserEmailVerified,
				ProfileComplete: result.UserProfileComplete,
				CreatedAt:       result.UserCreatedAt,
				UpdatedAt:       result.UserUpdatedAt,
			},
			Profile: user.UserProfile{
				UserID:    result.UserID,
				FirstName: result.UserFirstName,
				LastName:  result.UserLastName,
				Phone:     result.UserPhone,
				AvatarURL: result.UserAvatarURL,
			},
			Rank: result.Rank,
		}
	}

	return searchResults, int(total), nil
}

// extractQueryString extracts the actual query string from input
func (s *SearchService) extractQueryString(input *SearchInput) string {
	if input.Query != nil {
		return *input.Query
	}
	return ""
}

// buildFiltersSummary builds a summary of applied filters
func (s *SearchService) buildFiltersSummary(input *SearchInput) SearchFilters {
	filters := SearchFilters{}

	if input.CategoryIDs != nil {
		filters.CategoryIDs = *input.CategoryIDs
	}
	if input.Location != nil {
		filters.Location = *input.Location
	}
	if input.MinRating != nil {
		filters.MinRating = *input.MinRating
	}
	if input.MinReviews != nil {
		filters.MinReviews = *input.MinReviews
	}
	if input.Languages != nil {
		filters.Languages = *input.Languages
	}
	if input.MaxDistanceKm != nil {
		filters.MaxDistanceKm = *input.MaxDistanceKm
	}
	if input.Limit != nil {
		filters.Limit = *input.Limit
	}
	if input.Offset != nil {
		filters.Offset = *input.Offset
	}

	return filters
}
