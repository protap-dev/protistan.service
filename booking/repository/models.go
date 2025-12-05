package repository

import (
	"encoding/json"
	"time"

	"encore.app/booking/domain"
	"gorm.io/datatypes"
)

// bookingDBModel represents the database model for bookings
type bookingDBModel struct {
	ID                string         `gorm:"column:id;primaryKey;default:generate_uuid()"`
	CustomerID        string         `gorm:"column:customer_id;not null"`
	ArtisanID         *string        `gorm:"column:artisan_id"`
	ServiceCategoryID string         `gorm:"column:service_category_id;not null"`
	ServiceID         string         `gorm:"column:service_id"`
	Title             string         `gorm:"column:title;not null"`
	Description       string         `gorm:"column:description"`
	CustomerAddressID string         `gorm:"column:customer_address_id;not null"`
	Status            string         `gorm:"column:status;not null"`
	Priority          string         `gorm:"column:priority"`
	ScheduledAt       *time.Time     `gorm:"column:scheduled_at"`
	Metadata          datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	IsSpecificArtisan bool           `gorm:"column:is_specific_artisan;default:false"`
	OffersCount       int            `gorm:"column:offers_count;default:0"`
	CreatedAt         time.Time      `gorm:"column:created_at;not null"`
	UpdatedAt         time.Time      `gorm:"column:updated_at;not null"`
	Version           int64          `gorm:"column:version;default:1"`
	DeletedAt         *time.Time     `gorm:"column:deleted_at"`
	MediaURLs         []string       `gorm:"type:text[]" json:"media_urls"`
	IsFlexible        bool           `json:"is_flexible"`
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

// offerDBModel represents the database model for booking offers
type offerDBModel struct {
	ID           string     `gorm:"column:id;primaryKey;default:generate_uuid()"`
	BookingID    string     `gorm:"column:booking_id;not null"`
	ArtisanID    string     `gorm:"column:artisan_id;not null"`
	Status       string     `gorm:"column:status;not null"`
	OfferedBy    string     `gorm:"column:offered_by;not null"`
	OfferedAt    time.Time  `gorm:"column:offered_at;not null"`
	ExpiresAt    time.Time  `gorm:"column:expires_at;not null"`
	RespondedAt  *time.Time `gorm:"column:responded_at"`
	RejectReason *string    `gorm:"column:reject_reason"`
	CreatedAt    time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;not null"`
}

func (offerDBModel) TableName() string {
	return "booking_offers"
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

	mediaURLs := domainBooking.MediaURLs
	if len(mediaURLs) == 1 && mediaURLs[0] == "" {
		mediaURLs = []string{} // Use an empty slice instead of a slice with an empty string
	}

	return &bookingDBModel{
		ID:                domainBooking.ID,
		CustomerID:        domainBooking.CustomerID,
		ArtisanID:         domainBooking.ArtisanID,
		ServiceCategoryID: domainBooking.ServiceCategoryID,
		ServiceID:         domainBooking.ServiceID,
		Title:             domainBooking.Title,
		Description:       domainBooking.Description,
		CustomerAddressID: domainBooking.CustomerAddressID,
		Status:            string(domainBooking.Status),
		Priority:          domainBooking.Priority,
		ScheduledAt:       domainBooking.ScheduledAt,
		Metadata:          datatypes.JSON(metadataJSON),
		IsSpecificArtisan: domainBooking.IsSpecificArtisan,
		OffersCount:       domainBooking.OffersCount,
		CreatedAt:         domainBooking.CreatedAt,
		UpdatedAt:         domainBooking.UpdatedAt,
		Version:           domainBooking.Version,
		MediaURLs:         mediaURLs,
		IsFlexible:        domainBooking.IsFlexible,
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
		ID:                dbBooking.ID,
		CustomerID:        dbBooking.CustomerID,
		ArtisanID:         dbBooking.ArtisanID,
		ServiceCategoryID: dbBooking.ServiceCategoryID,
		ServiceID:         dbBooking.ServiceID,
		Title:             dbBooking.Title,
		Description:       dbBooking.Description,
		CustomerAddressID: dbBooking.CustomerAddressID,
		Status:            domain.BookingStatus(dbBooking.Status),
		Priority:          dbBooking.Priority,
		ScheduledAt:       dbBooking.ScheduledAt,
		Metadata:          metadata,
		IsSpecificArtisan: dbBooking.IsSpecificArtisan,
		OffersCount:       dbBooking.OffersCount,
		CreatedAt:         dbBooking.CreatedAt,
		UpdatedAt:         dbBooking.UpdatedAt,
		Version:           dbBooking.Version,
	}
}

// offerToDBModel converts domain offer to database model
func offerToDBModel(domainOffer *domain.BookingOffer) *offerDBModel {
	return &offerDBModel{
		ID:           domainOffer.ID,
		BookingID:    domainOffer.BookingID,
		ArtisanID:    domainOffer.ArtisanID,
		Status:       string(domainOffer.Status),
		OfferedBy:    domainOffer.OfferedBy,
		OfferedAt:    domainOffer.OfferedAt,
		ExpiresAt:    domainOffer.ExpiresAt,
		RespondedAt:  domainOffer.RespondedAt,
		RejectReason: domainOffer.RejectReason,
		CreatedAt:    domainOffer.CreatedAt,
		UpdatedAt:    domainOffer.UpdatedAt,
	}
}

// offerToDomainModel converts database offer to domain model
func offerToDomainModel(dbOffer *offerDBModel) *domain.BookingOffer {
	return &domain.BookingOffer{
		ID:           dbOffer.ID,
		BookingID:    dbOffer.BookingID,
		ArtisanID:    dbOffer.ArtisanID,
		Status:       domain.BookingOfferStatus(dbOffer.Status),
		OfferedBy:    dbOffer.OfferedBy,
		OfferedAt:    dbOffer.OfferedAt,
		ExpiresAt:    dbOffer.ExpiresAt,
		RespondedAt:  dbOffer.RespondedAt,
		RejectReason: dbOffer.RejectReason,
		CreatedAt:    dbOffer.CreatedAt,
		UpdatedAt:    dbOffer.UpdatedAt,
	}
}

// IdempotencyRecord represents the idempotency key storage
type IdempotencyRecord struct {
	IdempotencyKey string     `gorm:"primaryKey;column:idempotency_key;type:varchar(255)"`
	UserID         string     `gorm:"column:user_id;type:uuid;not null;index"`
	RequestHash    string     `gorm:"column:request_hash;type:text;not null"`
	BookingID      *string    `gorm:"column:booking_id;type:uuid"`
	ResponseBody   []byte     `gorm:"column:response_body;type:jsonb"`
	Status         string     `gorm:"column:status;type:varchar(20);not null;default:'processing'"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null;default:now()"`
	CompletedAt    *time.Time `gorm:"column:completed_at"`
	ExpiresAt      time.Time  `gorm:"column:expires_at;not null;index"`
}

func (IdempotencyRecord) TableName() string {
	return "idempotency_keys"
}
