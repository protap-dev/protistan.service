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

// GetAllServicesGroupedByCategory retrieves all services grouped by their categories
// Optimized version using a single JOIN query to avoid N+1 problem
// Optional categoryID parameter filters results to specific category
// includeEmpty parameter determines whether to include categories with no services
func (r *ratesRepository) GetAllServicesGroupedByCategory(ctx context.Context, includeEmpty bool, categoryID ...string) ([]domain.ServiceCategoryWithServices, error) {
	type CategoryServiceRow struct {
		// Category fields
		CategoryID          string    `gorm:"column:category_id"`
		CategoryName        string    `gorm:"column:category_name"`
		CategoryDescription string    `gorm:"column:category_description"`
		CategoryCreatedAt   time.Time `gorm:"column:category_created_at"`

		// Service fields (nullable for categories with no services)
		ServiceID          *string    `gorm:"column:service_id"`
		ServiceName        *string    `gorm:"column:service_name"`
		ServiceDescription *string    `gorm:"column:service_description"`
		ServiceBasePrice   *int64     `gorm:"column:service_base_price"`
		ServiceCurrency    *string    `gorm:"column:service_currency"`
		ServiceCreatedAt   *time.Time `gorm:"column:service_created_at"`
	}

	var rows []CategoryServiceRow
	query := r.db.WithContext(ctx).
		Table("service_categories sc").
		Select(`
			sc.id as category_id,
			sc.name as category_name,
			sc.description as category_description,
			sc.created_at as category_created_at,
			s.id as service_id,
			s.name as service_name,
			s.description as service_description,
			s.base_price_cents as service_base_price,
			s.currency as service_currency,
			s.created_at as service_created_at
		`)

	// Use INNER JOIN when we don't want empty categories, LEFT JOIN otherwise
	if includeEmpty {
		query = query.Joins("LEFT JOIN services s ON sc.id = s.category_id")
	} else {
		query = query.Joins("INNER JOIN services s ON sc.id = s.category_id")
	}

	// Apply category filter if provided
	if len(categoryID) > 0 {
		// Filter out empty strings and check if we have valid category IDs
		validCategoryIDs := make([]string, 0, len(categoryID))
		for _, id := range categoryID {
			if id != "" {
				validCategoryIDs = append(validCategoryIDs, id)
			}
		}

		if len(validCategoryIDs) > 0 {
			query = query.Where("sc.id IN ?", validCategoryIDs)
		}
	}

	err := query.Order("sc.name ASC, s.name ASC").Find(&rows).Error

	if err != nil {
		return nil, err
	}

	// Group results by category efficiently
	categoryMap := make(map[string]*domain.ServiceCategoryWithServices)
	var categoryOrder []string // Track insertion order for deterministic results

	for _, row := range rows {
		// Get or initialize category structure for grouping
		category, exists := categoryMap[row.CategoryID]
		if !exists {
			newCategory := &domain.ServiceCategoryWithServices{
				ServiceCategory: domain.ServiceCategory{
					ID:          row.CategoryID,
					Name:        row.CategoryName,
					Description: row.CategoryDescription,
					CreatedAt:   row.CategoryCreatedAt,
				},
				Services: make([]domain.Service, 0), // Pre-allocate empty slice
			}
			categoryMap[row.CategoryID] = newCategory
			categoryOrder = append(categoryOrder, row.CategoryID)
			category = newCategory
		}

		// Add service if it exists (LEFT JOIN might return null services)
		if row.ServiceID != nil {
			service := domain.Service{
				ID:          *row.ServiceID,
				CategoryID:  row.CategoryID,
				Name:        *row.ServiceName,
				Description: *row.ServiceDescription,
				Currency:    *row.ServiceCurrency,
				CreatedAt:   *row.ServiceCreatedAt,
			}
			if row.ServiceBasePrice != nil {
				service.BasePrice = row.ServiceBasePrice
			}

			// Append to the category services
			category.Services = append(category.Services, service)
		}
	}

	// Build result slice in deterministic order
	result := make([]domain.ServiceCategoryWithServices, 0, len(categoryOrder))
	for _, categoryID := range categoryOrder {
		result = append(result, *categoryMap[categoryID])
	}

	return result, nil
}
