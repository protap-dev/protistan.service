package repository

import (
	"context"
	"time"

	"encore.app/artisans/domain"
	"gorm.io/gorm"
)

// ratesRepository implements domain.RatesRepository
type ratesRepository struct {
	db *gorm.DB
}

// NewRatesRepository creates a new rates repository
func NewRatesRepository(db *gorm.DB) domain.RatesRepository {
	return &ratesRepository{db: db}
}

// Create creates a new artisan rate
func (r *ratesRepository) Create(ctx context.Context, rate *domain.ArtisanRate) error {
	// Note: ID, CreatedAt, UpdatedAt are handled by database defaults
	return r.db.WithContext(ctx).Create(rate).Error
}

// GetByArtisanID retrieves all rates for a specific artisan
func (r *ratesRepository) GetByArtisanID(ctx context.Context, artisanID string) ([]domain.ArtisanRate, error) {
	var rates []domain.ArtisanRate
	err := r.db.WithContext(ctx).
		Where("artisan_id = ?", artisanID).
		Order("created_at DESC").
		Find(&rates).Error

	return rates, err
}

// GetByArtisanAndServiceID retrieves a specific rate for an artisan and service
func (r *ratesRepository) GetByArtisanAndServiceID(ctx context.Context, artisanID, serviceID string) (*domain.ArtisanRate, error) {
	var rate domain.ArtisanRate
	err := r.db.WithContext(ctx).
		Where("artisan_id = ? AND service_id = ?", artisanID, serviceID).
		First(&rate).Error

	if err != nil {
		return nil, err
	}

	return &rate, nil
}

// GetByID retrieves a rate by its ID
func (r *ratesRepository) GetByID(ctx context.Context, id string) (*domain.ArtisanRate, error) {
	var rate domain.ArtisanRate
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&rate).Error
	if err != nil {
		return nil, err
	}

	return &rate, nil
}

// Update updates a rate with the given updates
func (r *ratesRepository) Update(ctx context.Context, id string, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now()
	return r.db.WithContext(ctx).Model(&domain.ArtisanRate{}).Where("id = ?", id).Updates(updates).Error
}

// Delete deletes a rate by ID
func (r *ratesRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&domain.ArtisanRate{}, "id = ?", id).Error
}

// WithTransaction executes a function within a database transaction
func (r *ratesRepository) WithTransaction(ctx context.Context, fn func(domain.RatesRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Create a repository instance that uses the transaction
		txRepo := &ratesRepository{db: tx}
		return fn(txRepo)
	})
}

// WithReadTransaction executes a function within a read-only transaction for consistency
func (r *ratesRepository) WithReadTransaction(ctx context.Context, fn func(domain.RatesRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Create a repository instance that uses the transaction
		txRepo := &ratesRepository{db: tx}
		return fn(txRepo)
	})
}

// GetServiceByID retrieves service information by ID
func (r *ratesRepository) GetServiceByID(ctx context.Context, serviceID string) (*domain.Service, error) {
	var service domain.Service
	err := r.db.WithContext(ctx).
		Table("services").
		Where("id = ?", serviceID).
		First(&service).Error

	if err != nil {
		return nil, err
	}

	return &service, nil
}

// GetCategoryByID retrieves category information by ID
func (r *ratesRepository) GetCategoryByID(ctx context.Context, categoryID string) (*domain.ServiceCategory, error) {
	var category domain.ServiceCategory
	err := r.db.WithContext(ctx).
		Table("service_categories").
		Where("id = ?", categoryID).
		First(&category).Error

	if err != nil {
		return nil, err
	}

	return &category, nil
}

// GetAllCategories retrieves all service categories
func (r *ratesRepository) GetAllCategories(ctx context.Context) ([]domain.ServiceCategory, error) {
	var categories []domain.ServiceCategory
	err := r.db.WithContext(ctx).
		Table("service_categories").
		Order("name ASC").
		Find(&categories).Error

	if err != nil {
		return nil, err
	}

	return categories, nil
}
