package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"strings"
	"time"

	"encore.app/artisans/domain"
	"encore.app/core"
	"gorm.io/gorm"
)

// StringArray is a custom type for handling string arrays in PostgreSQL
type StringArray []string

// Value implements driver.Valuer interface for writing to database
func (s StringArray) Value() (driver.Value, error) {
	if len(s) == 0 {
		return "{}", nil
	}

	// Format as PostgreSQL array literal
	elements := make([]string, len(s))
	for i, str := range s {
		elements[i] = `"` + strings.ReplaceAll(str, `"`, `\"`) + `"`
	}
	return "{" + strings.Join(elements, ",") + "}", nil
}

// Scan implements sql.Scanner interface for reading from database
func (s *StringArray) Scan(value interface{}) error {
	if value == nil {
		*s = StringArray{}
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return errors.New("cannot scan non-string value into StringArray")
	}

	// Handle empty array
	if str == "{}" {
		*s = StringArray{}
		return nil
	}

	// Parse PostgreSQL array format: {"str1","str2"}
	if strings.HasPrefix(str, "{") && strings.HasSuffix(str, "}") {
		content := str[1 : len(str)-1]
		if content == "" {
			*s = StringArray{}
			return nil
		}

		// Split by comma and clean quotes
		parts := strings.Split(content, ",")
		result := make([]string, len(parts))
		for i, part := range parts {
			// Remove quotes if present and unescape
			part = strings.Trim(part, `"`)
			part = strings.ReplaceAll(part, `\"`, `"`)
			result[i] = part
		}
		*s = StringArray(result)
		return nil
	}

	return errors.New("invalid StringArray format")
}

type artisanRepository struct {
	db *gorm.DB
}

// NewArtisanRepository creates a new repository implementation
func NewArtisanRepository(db *gorm.DB) domain.ArtisanRepository {
	return &artisanRepository{db: db}
}

// Create inserts a new artisan profile
func (r *artisanRepository) Create(ctx context.Context, artisan *domain.ArtisanProfile) error {
	dbModel := toDBModel(artisan)

	if err := r.db.WithContext(ctx).Create(&dbModel).Error; err != nil {
		return err
	}

	artisan.ID = dbModel.ID
	artisan.CreatedAt = dbModel.CreatedAt
	artisan.UpdatedAt = dbModel.UpdatedAt

	return nil
}

// GetByID retrieves an artisan by ID
func (r *artisanRepository) GetByID(ctx context.Context, id string) (*domain.ArtisanProfile, error) {
	var dbModel ArtisanDBModel

	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&dbModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	return toDomainModel(&dbModel), nil
}

// GetByUserID retrieves an artisan by user ID
func (r *artisanRepository) GetByUserID(ctx context.Context, userID string) (*domain.ArtisanProfile, error) {
	var dbModel ArtisanDBModel

	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&dbModel).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	return toDomainModel(&dbModel), nil
}

// Update modifies an existing artisan profile
func (r *artisanRepository) Update(ctx context.Context, id string, updates map[string]any) error {
	// Handle array conversions if present
	if categoryIDs, ok := updates["category_ids"].([]string); ok {
		updates["category_ids"] = StringArray(categoryIDs)
	}
	if languages, ok := updates["languages"].([]string); ok {
		updates["languages"] = StringArray(languages)
	}
	// Handle coordinates pointer conversion if present
	if coordinates, ok := updates["coordinates"].(string); ok {
		if coordinates == "" {
			updates["coordinates"] = nil // Set to NULL for empty coordinates
		} else {
			updates["coordinates"] = &coordinates // Set as pointer for non-empty coordinates
		}
	}

	result := r.db.WithContext(ctx).Model(&ArtisanDBModel{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// Delete removes an artisan profile
func (r *artisanRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&ArtisanDBModel{}).Error
}

// WithTransaction executes a function within a database transaction
func (r *artisanRepository) WithTransaction(ctx context.Context, fn func(domain.ArtisanRepository) error) error {
	return core.WithTransaction(ctx, r.db, func(db *gorm.DB) domain.ArtisanRepository {
		return &artisanRepository{db: db}
	}, fn)
}

// WithReadTransaction executes a function within a read-only transaction for consistency
func (r *artisanRepository) WithReadTransaction(ctx context.Context, fn func(domain.ArtisanRepository) error) error {
	return core.WithReadTransaction(ctx, r.db, func(db *gorm.DB) domain.ArtisanRepository {
		return &artisanRepository{db: db}
	}, fn)
}

// GetDB returns the underlying database connection for complex queries
func (r *artisanRepository) GetDB() *gorm.DB {
	return r.db
}

// DB model (maps to database table)
type ArtisanDBModel struct {
	ID                  string      `gorm:"primarykey;type:uuid;default:generate_uuid()"`
	UserID              string      `gorm:"column:user_id"`
	CategoryIDs         StringArray `gorm:"type:uuid[];column:category_ids"`
	Bio                 string      `gorm:"column:bio"`
	YearsExperience     int         `gorm:"column:years_experience"`
	Languages           StringArray `gorm:"type:text[];column:languages"`
	Rating              float64     `gorm:"column:rating"`
	ReviewsCount        int         `gorm:"column:reviews_count"`
	Verified            bool        `gorm:"column:verified"`
	MaxTravelDistanceKm float64     `gorm:"column:max_travel_distance_km"`
	AvatarURL           string      `gorm:"column:avatar_url"`
	Coordinates         *string     `gorm:"type:point;column:coordinates"`
	PreferredCity       string      `gorm:"column:preferred_city"`
	PreferredState      string      `gorm:"column:preferred_state"`
	PreferredCountry    string      `gorm:"column:preferred_country"`
	SearchVector        string      `gorm:"type:tsvector;column:search_vector"`
	CreatedAt           time.Time   `gorm:"column:created_at"`
	UpdatedAt           time.Time   `gorm:"column:updated_at"`
}

func (ArtisanDBModel) TableName() string {
	return "artisans"
}

// Mapping functions
func toDomainModel(db *ArtisanDBModel) *domain.ArtisanProfile {
	coordinates := ""
	if db.Coordinates != nil {
		coordinates = *db.Coordinates
	}

	return &domain.ArtisanProfile{
		ID:                  db.ID,
		UserID:              db.UserID,
		CategoryIDs:         []string(db.CategoryIDs),
		Bio:                 db.Bio,
		YearsExperience:     db.YearsExperience,
		Languages:           []string(db.Languages),
		Rating:              db.Rating,
		ReviewsCount:        db.ReviewsCount,
		Verified:            db.Verified,
		MaxTravelDistanceKm: db.MaxTravelDistanceKm,
		AvatarURL:           db.AvatarURL,
		Coordinates:         coordinates,
		PreferredCity:       db.PreferredCity,
		PreferredState:      db.PreferredState,
		PreferredCountry:    db.PreferredCountry,
		SearchVector:        db.SearchVector,
		CreatedAt:           db.CreatedAt,
		UpdatedAt:           db.UpdatedAt,
	}
}

func toDBModel(d *domain.ArtisanProfile) *ArtisanDBModel {
	var coordinates *string
	if d.Coordinates != "" {
		coordinates = &d.Coordinates
	}

	return &ArtisanDBModel{
		ID:                  d.ID,
		UserID:              d.UserID,
		CategoryIDs:         StringArray(d.CategoryIDs),
		Bio:                 d.Bio,
		YearsExperience:     d.YearsExperience,
		Languages:           StringArray(d.Languages),
		Rating:              d.Rating,
		ReviewsCount:        d.ReviewsCount,
		Verified:            d.Verified,
		MaxTravelDistanceKm: d.MaxTravelDistanceKm,
		AvatarURL:           d.AvatarURL,
		Coordinates:         coordinates,
		PreferredCity:       d.PreferredCity,
		PreferredState:      d.PreferredState,
		PreferredCountry:    d.PreferredCountry,
		SearchVector:        d.SearchVector,
		CreatedAt:           d.CreatedAt,
		UpdatedAt:           d.UpdatedAt,
	}
}
