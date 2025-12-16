package handlers

import (
	"context"
	"fmt"
	"strings"

	"encore.app/artisans/domain"
	"encore.app/artisans/internal"
)

// SearchHandler handles search operations
type SearchHandler struct {
	service    *domain.SearchService
	authHelper *internal.AuthHelper
	logger     internal.Logger
}

// NewSearchHandler creates a new search handler
func NewSearchHandler(service *domain.SearchService, authHelper *internal.AuthHelper, logger internal.Logger) *SearchHandler {
	return &SearchHandler{
		service:    service,
		authHelper: authHelper,
		logger:     logger,
	}
}

// Search handles artisan search requests
func (h *SearchHandler) Search(ctx context.Context, req *SearchArtisansRequest) (*SearchArtisansResponse, error) {
	h.logger.LogUserAction(ctx, "search_artisans", "authenticated")

	// Validate search request
	if err := h.validateSearchRequest(req); err != nil {
		h.logger.LogError(ctx, "validate_search_request", err)
		return nil, err
	}

	// Convert handler request to domain input
	domainInput := &domain.SearchInput{
		Query:         req.Query,
		CategoryIDs:   req.CategoryIDs,
		Location:      req.Location,
		MaxDistanceKm: req.MaxDistanceKm,
		MinRating:     req.MinRating,
		MinReviews:    req.MinReviews,
		Languages:     req.Languages,
		Limit:         req.Limit,
		Offset:        req.Offset,
	}

	// Execute search
	result, err := h.service.Search(ctx, domainInput)
	if err != nil {
		h.logger.LogError(ctx, "execute_search", err)
		return nil, err
	}

	// Convert domain results to handler response
	response := &SearchArtisansResponse{
		Results: h.convertSearchResults(result.Results),
		Total:   result.Total,
		Query:   result.Query,
		Filters: h.convertFilters(result.Filters),
	}

	h.logger.LogUserAction(ctx, "search_artisans_completed", fmt.Sprintf("found_%d_results", result.Total))
	return response, nil
}

// validateSearchRequest validates search parameters
func (h *SearchHandler) validateSearchRequest(req *SearchArtisansRequest) error {
	// At least one search criteria must be provided
	if (req.Query == nil || strings.TrimSpace(*req.Query) == "") &&
		(req.CategoryIDs == nil || len(*req.CategoryIDs) == 0) &&
		(req.Location == nil || strings.TrimSpace(*req.Location) == "") {
		return internal.ErrInvalidSearchQuery
	}

	// Validate query if provided, ensuring it's not empty and meets length requirements
	if req.Query != nil && strings.TrimSpace(*req.Query) != "" && len(strings.TrimSpace(*req.Query)) < 2 {
		return internal.ErrInvalidSearchQuery
	}

	// Validate category IDs if provided
	if req.CategoryIDs != nil {
		for _, categoryID := range *req.CategoryIDs {
			if categoryID == "" {
				return internal.ErrInvalidCategories
			}
			if len(categoryID) != 36 {
				return internal.ErrInvalidCategories
			}
		}
	}

	// Validate rating range if provided
	if req.MinRating != nil {
		if *req.MinRating < 0 || *req.MinRating > 5 {
			return internal.ErrInvalidInput
		}
	}

	// Validate pagination parameters
	if req.Limit != nil && (*req.Limit < 1 || *req.Limit > 100) {
		return internal.ErrInvalidPagination
	}
	if req.Offset != nil && *req.Offset < 0 {
		return internal.ErrInvalidPagination
	}

	return nil
}

// convertSearchResults converts domain search results to handler response
func (h *SearchHandler) convertSearchResults(results []domain.SearchResult) []ArtisanSearchResult {
	responseResults := make([]ArtisanSearchResult, len(results))

	for i, result := range results {
		responseResults[i] = ArtisanSearchResult{
			Artisan: result.Artisan,
			User: domain.UserData{
				ID:         result.User.ID,
				Email:      result.User.Email,
				Roles:      result.User.Roles,
				ActiveRole: result.User.ActiveRole,
			},
			Profile: domain.ProfileData{
				FirstName: result.Profile.FirstName,
				LastName:  result.Profile.LastName,
				Phone:     result.Profile.Phone,
				AvatarURL: result.Profile.AvatarURL,
			},
			DistanceKm: result.DistanceKm,
			Rank:       result.Rank,
		}
	}

	return responseResults
}

// convertFilters converts domain filters to handler response
func (h *SearchHandler) convertFilters(filters domain.SearchFilters) SearchFilters {
	return SearchFilters{
		CategoryIDs:   filters.CategoryIDs,
		Location:      filters.Location,
		MinRating:     filters.MinRating,
		MinReviews:    filters.MinReviews,
		Languages:     filters.Languages,
		MaxDistanceKm: filters.MaxDistanceKm,
		Limit:         filters.Limit,
		Offset:        filters.Offset,
	}
}
