package repository

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"encore.app/booking/domain"
	"gorm.io/gorm"
)

// RepositoryHelper provides generic repository operations to reduce code duplication
type RepositoryHelper struct {
	db *gorm.DB
}

// NewRepositoryHelper creates a new repository helper instance
func NewRepositoryHelper(db *gorm.DB) *RepositoryHelper {
	return &RepositoryHelper{db: db}
}

// GetByField retrieves a single entity by a field value
func (h *RepositoryHelper) GetByField(ctx context.Context, tableName, fieldName string, fieldValue any, dest any, notFoundErr error) error {
	query := h.db.WithContext(ctx).Where(fmt.Sprintf("%s = ?", fieldName), fieldValue).First(dest)

	if query.Error != nil {
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			return notFoundErr
		}
		return query.Error
	}

	return nil
}

// ListByField retrieves multiple entities by a field value with ordering
func (h *RepositoryHelper) ListByField(ctx context.Context, tableName, fieldName string, fieldValue any, orderBy string, destSlice any, notFoundErr error) error {
	query := h.db.WithContext(ctx).Where(fmt.Sprintf("%s = ?", fieldName), fieldValue)

	if orderBy != "" {
		query = query.Order(orderBy)
	}

	if err := query.Find(destSlice).Error; err != nil {
		return err
	}

	return nil
}

// ConvertSlice converts a slice of database models to domain models
func (h *RepositoryHelper) ConvertSlice(ctx context.Context, dbModelsSlice any, converter func(any) any) (any, error) {
	dbSliceValue := reflect.ValueOf(dbModelsSlice)
	if dbSliceValue.Kind() != reflect.Slice {
		return nil, fmt.Errorf("dbModelsSlice must be a slice")
	}

	// Create result slice of same length
	resultSlice := reflect.MakeSlice(dbSliceValue.Type(), dbSliceValue.Len(), dbSliceValue.Len())

	// Convert each element
	for i := 0; i < dbSliceValue.Len(); i++ {
		dbModel := dbSliceValue.Index(i).Interface()
		domainModel := converter(dbModel)
		resultSlice.Index(i).Set(reflect.ValueOf(domainModel))
	}

	return resultSlice.Interface(), nil
}

// UpdateWithOptimisticLock performs an optimistic lock update with version checking
func (h *RepositoryHelper) UpdateWithOptimisticLock(ctx context.Context, tableName string, id string, updates map[string]any, versionField string, currentVersion int64, notFoundErr error) error {
	// Build update query with version check
	query := h.db.WithContext(ctx).Model(h.getTableModel(tableName)).
		Where("id = ? AND "+versionField+" = ?", id, currentVersion)

	// Add version increment for optimistic locking
	if versionField != "" {
		updates[versionField] = gorm.Expr(versionField + " + 1")
	}

	result := query.Updates(updates)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		// Check if record exists (version conflict or not found)
		var count int64
		if err := h.db.WithContext(ctx).Model(h.getTableModel(tableName)).Where("id = ?", id).Count(&count).Error; err != nil {
			return err
		}

		if count == 0 {
			return notFoundErr
		}

		// Version conflict - return specific error
		return &domain.ErrOptimisticLockFailure{
			BookingID: id,
		}
	}

	return nil
}

// getTableModel returns the appropriate model for a table name (simplified)
func (h *RepositoryHelper) getTableModel(tableName string) any {
	switch tableName {
	case "bookings":
		return &bookingDBModel{}
	case "booking_offers":
		return &offerDBModel{}
	default:
		return &bookingDBModel{} // fallback
	}
}
