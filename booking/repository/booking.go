package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"encore.app/booking/domain"
	"gorm.io/gorm"
)

// bookingRepository implements domain.BookingRepository
type bookingRepository struct {
	db     *gorm.DB
	coreDB *gorm.DB
}

// NewBookingRepository creates a new booking repository
func NewBookingRepository(db *gorm.DB, coreDB *gorm.DB) domain.BookingRepository {
	return &bookingRepository{
		db:     db,
		coreDB: coreDB,
	}
}

// Create inserts a new booking
func (r *bookingRepository) Create(ctx context.Context, booking *domain.Booking) error {
	dbModel := toDBModel(booking)

	if err := r.db.WithContext(ctx).Create(&dbModel).Error; err != nil {
		return err
	}

	// Update the original domain model with the generated ID and timestamps
	booking.ID = dbModel.ID
	booking.CreatedAt = dbModel.CreatedAt
	booking.UpdatedAt = dbModel.UpdatedAt
	booking.Version = dbModel.Version // Ensure version is synced

	return nil
}

// GetByID retrieves a booking by ID
func (r *bookingRepository) GetByID(ctx context.Context, id string) (*domain.Booking, error) {
	var dbModel bookingDBModel
	if err := r.db.WithContext(ctx).First(&dbModel, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrBookingNotFound
		}
		return nil, err
	}
	return toDomainModel(&dbModel), nil
}

// GetByCustomerID retrieves bookings for a customer
func (r *bookingRepository) GetByCustomerID(ctx context.Context, customerID string) ([]*domain.Booking, error) {
	var dbModels []bookingDBModel
	if err := r.db.WithContext(ctx).Where("customer_id = ?", customerID).Order("created_at DESC").Find(&dbModels).Error; err != nil {
		return nil, err
	}

	result := make([]*domain.Booking, 0, len(dbModels))
	for i := range dbModels {
		result = append(result, toDomainModel(&dbModels[i]))
	}
	return result, nil
}

// GetByArtisanID retrieves bookings for an artisan
func (r *bookingRepository) GetByArtisanID(ctx context.Context, artisanID string) ([]*domain.Booking, error) {
	var dbModels []bookingDBModel
	if err := r.db.WithContext(ctx).Where("artisan_id = ?", artisanID).Order("created_at DESC").Find(&dbModels).Error; err != nil {
		return nil, err
	}

	result := make([]*domain.Booking, 0, len(dbModels))
	for i := range dbModels {
		result = append(result, toDomainModel(&dbModels[i]))
	}
	return result, nil
}

// Update saves a booking with optimistic locking.
// This method should be called within a transaction controlled by the service layer.
func (r *bookingRepository) Update(ctx context.Context, booking *domain.Booking) error {
	dbModel := toDBModel(booking)

	result := r.db.WithContext(ctx).Model(&bookingDBModel{}).
		Where("id = ? AND version = ?", dbModel.ID, dbModel.Version).
		Updates(map[string]any{
			"artisan_id":          dbModel.ArtisanID,
			"status":              dbModel.Status,
			"title":               dbModel.Title,
			"description":         dbModel.Description,
			"priority":            dbModel.Priority,
			"scheduled_at":        dbModel.ScheduledAt,
			"metadata":            dbModel.Metadata,
			"is_specific_artisan": dbModel.IsSpecificArtisan,
			"offers_count":        dbModel.OffersCount,
			"updated_at":          time.Now(),
			"version":             gorm.Expr("version + 1"),
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		var count int64
		r.db.WithContext(ctx).Model(&bookingDBModel{}).Where("id = ?", dbModel.ID).Count(&count)
		if count == 0 {
			return domain.ErrBookingNotFound
		}
		return &domain.ErrOptimisticLockFailure{
			BookingID: booking.ID,
		}
	}

	booking.Version++
	return nil
}

// SoftDelete marks a booking as deleted
func (r *bookingRepository) SoftDelete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&bookingDBModel{}).
		Where("id = ?", id).
		Update("deleted_at", time.Now()).
		Error
}

// CheckIdempotencyKey checks if an idempotency key exists and is valid
func (r *bookingRepository) CheckIdempotencyKey(ctx context.Context, key string, userID string, requestHash string) (*domain.IdempotencyRecord, error) {
	var record IdempotencyRecord
	err := r.db.WithContext(ctx).
		Where("idempotency_key = ? AND user_id = ? AND expires_at > ?", key, userID, time.Now()).
		First(&record).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil // No existing request
	}
	if err != nil {
		return nil, err
	}

	// Verify request hash matches to prevent key reuse with different data
	if record.RequestHash != requestHash {
		return nil, errors.New("idempotency key reused with different request data")
	}

	// Convert to domain model
	domainRecord := &domain.IdempotencyRecord{
		IdempotencyKey: record.IdempotencyKey,
		UserID:         record.UserID,
		RequestHash:    record.RequestHash,
		BookingID:      record.BookingID,
		ResponseBody:   record.ResponseBody,
		Status:         record.Status,
		CreatedAt:      record.CreatedAt,
		CompletedAt:    record.CompletedAt,
		ExpiresAt:      record.ExpiresAt,
	}

	return domainRecord, nil
}

// StoreIdempotencyKey stores a new idempotency key
func (r *bookingRepository) StoreIdempotencyKey(ctx context.Context, key string, userID string, requestHash string, expiresAt time.Time) error {
	record := IdempotencyRecord{
		IdempotencyKey: key,
		UserID:         userID,
		RequestHash:    requestHash,
		Status:         "processing",
		CreatedAt:      time.Now(),
		ExpiresAt:      expiresAt,
	}

	return r.db.WithContext(ctx).Create(&record).Error
}

// CompleteIdempotencyKey marks an idempotency key as completed with response
func (r *bookingRepository) CompleteIdempotencyKey(ctx context.Context, key string, bookingID string, response interface{}) error {
	responseJSON, err := json.Marshal(response)
	if err != nil {
		return err
	}

	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&IdempotencyRecord{}).
		Where("idempotency_key = ?", key).
		Updates(map[string]interface{}{
			"booking_id":    bookingID,
			"response_body": responseJSON,
			"status":        "completed",
			"completed_at":  now,
		}).Error
}
