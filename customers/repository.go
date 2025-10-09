package customers

import (
	"context"

	"encore.app/core"
	"gorm.io/gorm"
)

// Repository interfaces for data access abstraction

type AddressRepository interface {
	GetByID(ctx context.Context, id string, userID string) (*CustomerAddress, error)
	GetByUserID(ctx context.Context, userID string) ([]CustomerAddress, error)
	Create(ctx context.Context, address *CustomerAddress) error
	Update(ctx context.Context, id string, userID string, updates map[string]interface{}) error
	Delete(ctx context.Context, id string, userID string) error
	UnsetDefaultAddresses(ctx context.Context, userID string, excludeID string) error
}

// Repository implementation

type addressRepository struct {
	db *gorm.DB
}

func NewAddressRepository(db *gorm.DB) AddressRepository {
	return &addressRepository{db: db}
}

func (r *addressRepository) GetByID(ctx context.Context, id string, userID string) (*CustomerAddress, error) {
	var address CustomerAddress
	if err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&address).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrAddressNotFound
		}
		return nil, ErrDatabaseError
	}
	return &address, nil
}

func (r *addressRepository) GetByUserID(ctx context.Context, userID string) ([]CustomerAddress, error) {
	var addresses []CustomerAddress
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("is_default DESC, created_at DESC").Find(&addresses).Error; err != nil {
		return nil, ErrDatabaseError
	}
	return addresses, nil
}

func (r *addressRepository) Create(ctx context.Context, address *CustomerAddress) error {
	return r.db.WithContext(ctx).Create(address).Error
}

func (r *addressRepository) Update(ctx context.Context, id string, userID string, updates map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(&CustomerAddress{}).Where("id = ? AND user_id = ?", id, userID).Updates(updates).Error
}

func (r *addressRepository) Delete(ctx context.Context, id string, userID string) error {
	result := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).Delete(&CustomerAddress{})
	if result.Error != nil {
		return ErrDatabaseError
	}
	if result.RowsAffected == 0 {
		return ErrAddressNotFound
	}
	return nil
}

func (r *addressRepository) UnsetDefaultAddresses(ctx context.Context, userID string, excludeID string) error {
	query := r.db.WithContext(ctx).Model(&CustomerAddress{}).Where("user_id = ? AND is_default = ?", userID, true)
	if excludeID != "" {
		query = query.Where("id != ?", excludeID)
	}
	return query.Update("is_default", false).Error
}

// WithTransaction executes a function within a database transaction
func (r *addressRepository) WithTransaction(ctx context.Context, fn func(AddressRepository) error) error {
	return core.WithTransaction(ctx, r.db, func(db *gorm.DB) AddressRepository {
		return &addressRepository{db: db}
	}, fn)
}

// WithReadTransaction executes a function within a read-only transaction for consistency
func (r *addressRepository) WithReadTransaction(ctx context.Context, fn func(AddressRepository) error) error {
	return core.WithReadTransaction(ctx, r.db, func(db *gorm.DB) AddressRepository {
		return &addressRepository{db: db}
	}, fn)
}

// GetDB returns the underlying database connection for complex queries
func (r *addressRepository) GetDB() *gorm.DB {
	return r.db
}
