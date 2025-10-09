package domain

import "time"

// ArtisanProfile represents the core artisan domain model
type ArtisanProfile struct {
	ID                  string
	UserID              string
	CategoryIDs         []string
	Bio                 string
	YearsExperience     int
	Languages           []string
	Rating              float64
	ReviewsCount        int
	Verified            bool
	MaxTravelDistanceKm float64
	AvatarURL           string
	Coordinates         string
	PreferredCity       string
	PreferredState      string
	PreferredCountry    string
	SearchVector        string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// UserData represents user information from the user service
type UserData struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	UserType string `json:"user_type"`
}

// ProfileData represents profile information from the user service
type ProfileData struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Phone     string `json:"phone"`
	AvatarURL string `json:"avatar_url"`
}

// SettingsData represents user settings from the user service
type SettingsData struct {
	// Add settings fields as needed
}

// PublicArtisanProfile represents publicly viewable artisan information for customers
type PublicArtisanProfile struct {
	ArtisanID           string   `json:"artisan_id"`
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
	// User info (public only)
	UserFirstName string `json:"user_first_name"`
	UserLastName  string `json:"user_last_name"`
}

// CompleteProfile aggregates user and artisan data
type CompleteProfile struct {
	User     UserData       `json:"user"`
	Profile  ProfileData    `json:"profile"`
	Settings SettingsData   `json:"settings"`
	Artisan  ArtisanProfile `json:"artisan"`
}

// Input DTOs for service layer
type CreateProfileInput struct {
	CategoryIDs         []string
	Bio                 string
	YearsExperience     int
	Languages           []string
	MaxTravelDistanceKm float64
	AvatarURL           string
	PreferredCity       string
	PreferredState      string
	PreferredCountry    string
}

type UpdateProfileInput struct {
	CategoryIDs         *[]string
	Bio                 *string
	YearsExperience     *int
	Languages           *[]string
	MaxTravelDistanceKm *float64
	AvatarURL           *string
	Coordinates         *string
	PreferredCity       *string
	PreferredState      *string
	PreferredCountry    *string
}
