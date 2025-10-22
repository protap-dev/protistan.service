package handlers

import (
	"context"
	"errors"
	"strings"

	"encore.app/artisans/domain"
	"encore.app/artisans/internal"
)

// Request/Response types (API contracts)
type CreateProfileRequest struct {
	CategoryIDs         []string `json:"category_ids"`
	Bio                 string   `json:"bio"`
	YearsExperience     int      `json:"years_experience"`
	Languages           []string `json:"languages"`
	MaxTravelDistanceKm float64  `json:"max_travel_distance_km"`
	AvatarURL           string   `json:"avatar_url"`
	PreferredCity       string   `json:"preferred_city"`
	PreferredState      string   `json:"preferred_state"`
	PreferredCountry    string   `json:"preferred_country"`
}

type UpdateProfileRequest struct {
	CategoryIDs         *[]string `json:"category_ids,omitempty"`
	Bio                 *string   `json:"bio,omitempty"`
	YearsExperience     *int      `json:"years_experience,omitempty"`
	Languages           *[]string `json:"languages,omitempty"`
	MaxTravelDistanceKm *float64  `json:"max_travel_distance_km,omitempty"`
	AvatarURL           *string   `json:"avatar_url,omitempty"`
	Coordinates         *string   `json:"coordinates,omitempty"`
	PreferredCity       *string   `json:"preferred_city,omitempty"`
	PreferredState      *string   `json:"preferred_state,omitempty"`
	PreferredCountry    *string   `json:"preferred_country,omitempty"`
}

type SearchArtisansRequest struct {
	Query         *string   `json:"query,omitempty"`           // Full-text search query
	CategoryIDs   *[]string `json:"category_ids,omitempty"`    // Filter by service categories
	Location      *string   `json:"location,omitempty"`        // Location-based search (city, state, country)
	MaxDistanceKm *float64  `json:"max_distance_km,omitempty"` // Max distance from search location
	MinRating     *float64  `json:"min_rating,omitempty"`      // Minimum rating filter
	MinReviews    *int      `json:"min_reviews,omitempty"`     // Minimum reviews count
	Languages     *[]string `json:"languages,omitempty"`       // Filter by languages spoken
	Limit         *int      `json:"limit,omitempty"`           // Max results (default 20)
	Offset        *int      `json:"offset,omitempty"`          // Pagination offset (default 0)
}

type ArtisanSearchResult struct {
	Artisan    domain.ArtisanProfile `json:"artisan"`
	User       domain.UserData       `json:"user"`                  // User data from user service
	Profile    domain.ProfileData    `json:"profile"`               // Profile data from user service
	DistanceKm *float64              `json:"distance_km,omitempty"` // Distance from search location
	Rank       float64               `json:"rank"`                  // Search relevance rank
}

type SearchFilters struct {
	CategoryIDs   []string `json:"category_ids,omitempty"`
	Location      string   `json:"location,omitempty"`
	MinRating     float64  `json:"min_rating,omitempty"`
	MinReviews    int      `json:"min_reviews,omitempty"`
	Languages     []string `json:"languages,omitempty"`
	MaxDistanceKm float64  `json:"max_distance_km,omitempty"`
	Limit         int      `json:"limit,omitempty"`
	Offset        int      `json:"offset,omitempty"`
}

type SearchArtisansResponse struct {
	Results []ArtisanSearchResult `json:"results"`
	Total   int                   `json:"total"`   // Total matching results
	Query   string                `json:"query"`   // Original search query
	Filters SearchFilters         `json:"filters"` // Applied filters summary
}

type ProfileResponse struct {
	ArtisanID           string   `json:"artisan_id"`
	UserID              string   `json:"user_id"`
	CategoryIDs         []string `json:"category_ids"`
	Bio                 string   `json:"bio"`
	YearsExperience     int      `json:"years_experience"`
	Languages           []string `json:"languages"`
	Rating              float64  `json:"rating"`
	ReviewsCount        int      `json:"reviews_count"`
	Verified            bool     `json:"verified"`
	MaxTravelDistanceKm float64  `json:"max_travel_distance_km"`
	AvatarURL           string   `json:"avatar_url"`
	PreferredCity       string   `json:"preferred_city"`
	PreferredState      string   `json:"preferred_state"`
	PreferredCountry    string   `json:"preferred_country"`
}

// GetArtisanIDByUserIDRequest for internal service calls
type GetArtisanIDByUserIDRequest struct {
	UserID string `json:"user_id"`
}

// GetArtisanIDByUserIDResponse returns just the artisan ID
type GetArtisanIDByUserIDResponse struct {
	UserID    string `json:"user_id"`
	ArtisanID string `json:"artisan_id"`
	Found     bool   `json:"found"`
}

// CompleteProfileResponse represents the complete profile response
type CompleteProfileResponse struct {
	User     domain.UserData     `json:"user"`
	Profile  domain.ProfileData  `json:"profile"`
	Settings domain.SettingsData `json:"settings"`
	Artisan  ProfileResponse     `json:"artisan"`
}

type ProfileHandler struct {
	service    *domain.ProfileService
	authHelper *internal.AuthHelper
	logger     internal.Logger
}

func NewProfileHandler(service *domain.ProfileService, authHelper *internal.AuthHelper, logger internal.Logger) *ProfileHandler {
	return &ProfileHandler{
		service:    service,
		authHelper: authHelper,
		logger:     logger,
	}
}

// Create handles profile creation
func (h *ProfileHandler) Create(ctx context.Context, req *CreateProfileRequest) (*CompleteProfileResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "create_artisan_profile")
	if err != nil {
		return nil, err
	}

	domainReq := &domain.CreateProfileInput{
		CategoryIDs:         req.CategoryIDs,
		Bio:                 strings.TrimSpace(req.Bio),
		YearsExperience:     req.YearsExperience,
		Languages:           req.Languages,
		MaxTravelDistanceKm: req.MaxTravelDistanceKm,
		AvatarURL:           strings.TrimSpace(req.AvatarURL),
		PreferredCity:       strings.TrimSpace(req.PreferredCity),
		PreferredState:      strings.TrimSpace(req.PreferredState),
		PreferredCountry:    strings.TrimSpace(req.PreferredCountry),
	}

	result, err := h.service.CreateProfile(ctx, userCtx, domainReq)
	if err != nil {
		return nil, err
	}

	return &CompleteProfileResponse{
		User:     result.User,
		Profile:  result.Profile,
		Settings: result.Settings,
		Artisan:  toProfileResponse(&result.Artisan),
	}, nil
}

// Update handles profile updates
func (h *ProfileHandler) Update(ctx context.Context, req *UpdateProfileRequest) (*ProfileResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "update_artisan_profile")
	if err != nil {
		return nil, err
	}

	domainReq := &domain.UpdateProfileInput{ // nil values will be set below if provided
		CategoryIDs:         req.CategoryIDs,
		Bio:                 nil,
		YearsExperience:     req.YearsExperience,
		Languages:           req.Languages,
		MaxTravelDistanceKm: req.MaxTravelDistanceKm,
		AvatarURL:           nil,
		Coordinates:         nil,
		PreferredCity:       nil,
		PreferredState:      nil,
		PreferredCountry:    nil,
	}

	// Set optional fields with sanitization if provided
	if req.Bio != nil {
		trimmedBio := strings.TrimSpace(*req.Bio)
		domainReq.Bio = &trimmedBio
	}
	if req.AvatarURL != nil {
		trimmedAvatarURL := strings.TrimSpace(*req.AvatarURL)
		domainReq.AvatarURL = &trimmedAvatarURL
	}
	if req.Coordinates != nil {
		trimmedCoordinates := strings.TrimSpace(*req.Coordinates)
		domainReq.Coordinates = &trimmedCoordinates
	}
	if req.PreferredCity != nil {
		trimmedCity := strings.TrimSpace(*req.PreferredCity)
		domainReq.PreferredCity = &trimmedCity
	}
	if req.PreferredState != nil {
		trimmedState := strings.TrimSpace(*req.PreferredState)
		domainReq.PreferredState = &trimmedState
	}
	if req.PreferredCountry != nil {
		trimmedCountry := strings.TrimSpace(*req.PreferredCountry)
		domainReq.PreferredCountry = &trimmedCountry
	}

	result, err := h.service.UpdateProfile(ctx, userCtx, domainReq)
	if err != nil {
		return nil, err
	}

	// Fixed: return address of result
	response := toProfileResponse(result)
	return &response, nil
}

// Get retrieves the authenticated user's profile
func (h *ProfileHandler) Get(ctx context.Context) (*CompleteProfileResponse, error) {
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "get_artisan_profile")
	if err != nil {
		return nil, err
	}

	result, err := h.service.GetProfile(ctx, userCtx)
	if err != nil {
		return nil, err
	}

	return &CompleteProfileResponse{
		User:     result.User,
		Profile:  result.Profile,
		Settings: result.Settings,
		Artisan:  toProfileResponse(&result.Artisan),
	}, nil
}

// GetArtisanProfile retrieves a public artisan profile by ID (no auth required)
func (h *ProfileHandler) GetArtisanProfile(ctx context.Context, id string) (*domain.PublicArtisanProfile, error) {
	result, err := h.service.GetPublicArtisanProfile(ctx, id)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Helper to convert domain model to response (returns value, not pointer)
func toProfileResponse(profile *domain.ArtisanProfile) ProfileResponse {
	return ProfileResponse{
		ArtisanID:           profile.ID,
		UserID:              profile.UserID,
		CategoryIDs:         profile.CategoryIDs,
		Bio:                 profile.Bio,
		YearsExperience:     profile.YearsExperience,
		Languages:           profile.Languages,
		Rating:              profile.Rating,
		ReviewsCount:        profile.ReviewsCount,
		Verified:            profile.Verified,
		MaxTravelDistanceKm: profile.MaxTravelDistanceKm,
		AvatarURL:           profile.AvatarURL,
		PreferredCity:       profile.PreferredCity,
		PreferredState:      profile.PreferredState,
		PreferredCountry:    profile.PreferredCountry,
	}
}

func (h *ProfileHandler) GetArtisanIDByUserID(ctx context.Context, userID string) (*GetArtisanIDByUserIDResponse, error) {
	h.logger.LogUserAction(ctx, "get_artisan_id_by_user_id", userID)

	result, err := h.service.GetArtisanIDByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// User is not an artisan - not an error, just not found
			return &GetArtisanIDByUserIDResponse{
				UserID: userID,
				Found:  false,
			}, nil
		}
		return nil, err
	}

	return &GetArtisanIDByUserIDResponse{
		UserID:    userID,
		ArtisanID: result,
		Found:     true,
	}, nil
}
