package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"encore.app/booking/domain"
	"encore.app/core"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// bookingRepository implements domain.BookingRepository
type bookingRepository struct {
	db *gorm.DB
}

// NewBookingRepository creates a new booking repository
func NewBookingRepository(db *gorm.DB) domain.BookingRepository {
	return &bookingRepository{db: db}
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
			"artisan_id":              dbModel.ArtisanID,
			"status":                  dbModel.Status,
			"title":                   dbModel.Title,
			"description":             dbModel.Description,
			"priority":                dbModel.Priority,
			"scheduled_at":            dbModel.ScheduledAt,
			"estimated_duration_mins": dbModel.EstimatedDurationMins,
			"metadata":                dbModel.Metadata,
			"updated_at":              time.Now(),
			"version":                 gorm.Expr("version + 1"),
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

// CreateStatusHistory creates an audit trail entry for a booking status change.
func (r *bookingRepository) CreateStatusHistory(ctx context.Context, history *domain.BookingEvent) error {
	dbModel := &statusHistoryDBModel{
		BookingID:      history.BookingID,
		Status:         string(history.Status),
		PreviousStatus: string(history.PreviousStatus),
		ChangedBy:      history.UserID,
		Reason:         history.Reason,
		CreatedAt:      history.Timestamp,
	}
	return r.db.WithContext(ctx).Create(dbModel).Error
}

// SoftDelete marks a booking as deleted
func (r *bookingRepository) SoftDelete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&bookingDBModel{}).
		Where("id = ?", id).
		Update("deleted_at", time.Now()).
		Error
}

// WithTransaction executes a function within a database transaction.
func (r *bookingRepository) WithTransaction(ctx context.Context, fn func(repo domain.BookingRepository) error) error {
	return core.WithTransaction(ctx, r.db, func(db *gorm.DB) domain.BookingRepository {
		return &bookingRepository{db: db}
	}, fn)
}

// WithReadTransaction executes a function within a read-only transaction.
func (r *bookingRepository) WithReadTransaction(ctx context.Context, fn func(repo domain.BookingRepository) error) error {
	return core.WithReadTransaction(ctx, r.db, func(db *gorm.DB) domain.BookingRepository {
		return &bookingRepository{db: db}
	}, fn)
}

// GetDB returns the underlying database connection.
func (r *bookingRepository) GetDB() *gorm.DB {
	return r.db
}

// Repository-specific errors are now defined in the domain package.

// bookingDBModel represents the database model for bookings
type bookingDBModel struct {
	ID                    string         `gorm:"column:id;primaryKey;default:generate_uuid()"`
	CustomerID            string         `gorm:"column:customer_id;not null"`
	ArtisanID             *string        `gorm:"column:artisan_id"`
	ServiceCategoryID     string         `gorm:"column:service_category_id;not null"`
	Title                 string         `gorm:"column:title;not null"`
	Description           string         `gorm:"column:description"`
	CustomerAddressID     string         `gorm:"column:customer_address_id;not null"`
	Status                string         `gorm:"column:status;not null"`
	Priority              string         `gorm:"column:priority"`
	ScheduledAt           *time.Time     `gorm:"column:scheduled_at"`
	EstimatedDurationMins int            `gorm:"column:estimated_duration_mins"`
	Metadata              datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt             time.Time      `gorm:"column:created_at;not null"`
	UpdatedAt             time.Time      `gorm:"column:updated_at;not null"`
	Version               int64          `gorm:"column:version;default:1"`
	DeletedAt             *time.Time     `gorm:"column:deleted_at"`
}

func (bookingDBModel) TableName() string {
	return "bookings"
}

// statusHistoryDBModel represents the booking status history for audit trail
type statusHistoryDBModel struct {
	ID             string    `gorm:"column:id;primaryKey;default:generate_uuid()"`
	BookingID      string    `gorm:"column:booking_id;not null"`
	Status         string    `gorm:"column:status;not null"`
	PreviousStatus string    `gorm:"column:previous_status"`
	ChangedBy      string    `gorm:"column:changed_by"`
	Reason         *string   `gorm:"column:reason"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
}

func (statusHistoryDBModel) TableName() string {
	return "booking_status_history"
}

// toDBModel converts domain model to database model
func toDBModel(domainBooking *domain.Booking) *bookingDBModel {
	metadata := make(map[string]any)
	for k, v := range domainBooking.Metadata {
		metadata[k] = v
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	return &bookingDBModel{
		ID:                    domainBooking.ID,
		CustomerID:            domainBooking.CustomerID,
		ArtisanID:             domainBooking.ArtisanID,
		ServiceCategoryID:     domainBooking.ServiceCategoryID,
		Title:                 domainBooking.Title,
		Description:           domainBooking.Description,
		CustomerAddressID:     domainBooking.CustomerAddressID,
		Status:                string(domainBooking.Status),
		Priority:              domainBooking.Priority,
		ScheduledAt:           domainBooking.ScheduledAt,
		EstimatedDurationMins: domainBooking.EstimatedDurationMins,
		Metadata:              datatypes.JSON(metadataJSON),
		CreatedAt:             domainBooking.CreatedAt,
		UpdatedAt:             domainBooking.UpdatedAt,
		Version:               domainBooking.Version,
	}
}

// toDomainModel converts database model to domain model
func toDomainModel(dbBooking *bookingDBModel) *domain.Booking {
	var metadataMap map[string]any
	if err := json.Unmarshal(dbBooking.Metadata, &metadataMap); err != nil {
		metadataMap = make(map[string]any)
	}

	metadata := make(map[string]string)
	for k, v := range metadataMap {
		if str, ok := v.(string); ok {
			metadata[k] = str
		}
	}

	return &domain.Booking{
		ID:                    dbBooking.ID,
		CustomerID:            dbBooking.CustomerID,
		ArtisanID:             dbBooking.ArtisanID,
		ServiceCategoryID:     dbBooking.ServiceCategoryID,
		Title:                 dbBooking.Title,
		Description:           dbBooking.Description,
		CustomerAddressID:     dbBooking.CustomerAddressID,
		Status:                domain.BookingStatus(dbBooking.Status),
		Priority:              dbBooking.Priority,
		ScheduledAt:           dbBooking.ScheduledAt,
		EstimatedDurationMins: dbBooking.EstimatedDurationMins,
		Metadata:              metadata,
		CreatedAt:             dbBooking.CreatedAt,
		UpdatedAt:             dbBooking.UpdatedAt,
		Version:               dbBooking.Version,
	}
}
