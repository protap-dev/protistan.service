package domain

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"
)

// StringArray is a custom type for scanning string arrays from the database.
type StringArray []string

// Scan implements the sql.Scanner interface for StringArray.
// It scans a PostgreSQL array string like "{item1,item2}" into a []string.
func (a *StringArray) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}
	sv, err := driver.String.ConvertValue(value)
	if err != nil {
		return fmt.Errorf("failed to scan StringArray: %v", err)
	}
	s, ok := sv.(string)
	if !ok {
		return fmt.Errorf("failed to scan StringArray: expected string, got %T", sv)
	}

	// Trim the curly braces
	s = strings.Trim(s, "{}")

	// Handle empty array
	if s == "" {
		*a = []string{}
		return nil
	}

	// Split the string by comma. This is a simple approach and may not handle
	// all edge cases like quoted strings with commas.
	parts := strings.Split(s, ",")
	*a = StringArray(parts)

	return nil
}

// Value implements the driver.Valuer interface for StringArray.
func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	if len(a) == 0 {
		return "{}", nil
	}
	// Convert to a string representation of a PostgreSQL array.
	// This simple implementation may not handle all special characters correctly.
	return fmt.Sprintf("{%s}", strings.Join(a, ",")), nil
}

// ArtisanProfile represents the core artisan domain model
type ArtisanProfile struct {
	ID                     string
	UserID                 string
	CategoryIDs            StringArray `gorm:"type:uuid[]"`
	Bio                    string
	YearsExperience        int
	Languages              StringArray `gorm:"type:text[]"`
	Rating                 float64
	ReviewsCount           int
	RatesCount             int
	AvailabilityStatus     string
	Verified               bool
	AcceptsGenericRequests bool
	MaxTravelDistanceKm    float64
	AvatarURL              string
	Coordinates            string
	PreferredCity          string
	PreferredState         string
	PreferredCountry       string
	SearchVector           string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// ArtisanService represents a service offered by an artisan.
type ArtisanService struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// UserData represents user information from the user service
type UserData struct {
	ID         string   `json:"id"`
	Email      string   `json:"email"`
	Roles      []string `json:"roles"`
	ActiveRole string   `json:"active_role"`
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
	ArtisanID              string           `json:"artisan_id"`
	CategoryIDs            StringArray      `json:"category_ids"`
	Bio                    string           `json:"bio"`
	YearsExperience        int              `json:"years_experience"`
	Languages              StringArray      `json:"languages"`
	Rating                 float64          `json:"rating"`
	ReviewsCount           int              `json:"reviews_count"`
	Verified               bool             `json:"verified"`
	AcceptsGenericRequests bool             `json:"accepts_generic_requests"`
	MaxTravelDistanceKm    float64          `json:"max_travel_distance_km"`
	AvatarURL              string           `json:"avatar_url"`
	PreferredCity          string           `json:"preferred_city"`
	PreferredState         string           `json:"preferred_state"`
	PreferredCountry       string           `json:"preferred_country"`
	Services               []ArtisanService `json:"services"`
	// User info (public only)
	UserFirstName      string `json:"user_first_name"`
	UserLastName       string `json:"user_last_name"`
	AvailabilityStatus string `json:"availability_status"`
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
	CategoryIDs            []string
	Bio                    string
	YearsExperience        int
	Languages              []string
	AcceptsGenericRequests bool
	MaxTravelDistanceKm    float64
	AvatarURL              string
	PreferredCity          string
	PreferredState         string
	PreferredCountry       string
}

type UpdateProfileInput struct {
	CategoryIDs            *[]string
	Bio                    *string
	YearsExperience        *int
	Languages              *[]string
	AcceptsGenericRequests *bool
	MaxTravelDistanceKm    *float64
	AvatarURL              *string
	Coordinates            *string
	PreferredCity          *string
	PreferredState         *string
	PreferredCountry       *string
}

// ArtisanSearchResult represents a single artisan in search results.
type ArtisanSearchResult struct {
	Artisan  *PublicArtisanProfile `json:"artisan"`
	User     *UserData             `json:"user"`
	Profile  *ProfileData          `json.source:"profile"`
	Distance float64               `json:"distance_km"`
	Rank     float64               `json:"rank"`
	Services []ArtisanService      `json:"services,omitempty"`
}
