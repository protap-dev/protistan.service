package internal

import (
	"time"

	"encore.app/booking/domain"
)

// ToBookingResponse maps a domain.Booking to a lightweight response struct
type BookingResponse struct {
	ID                    string               `json:"id"`
	CustomerID            string               `json:"customer_id"`
	ArtisanID             *string              `json:"artisan_id,omitempty"`
	ServiceCategoryID     string               `json:"service_category_id"`
	Title                 string               `json:"title"`
	Description           string               `json:"description,omitempty"`
	CustomerAddressID     string               `json:"customer_address_id"`
	Status                domain.BookingStatus `json:"status"`
	Priority              string               `json:"priority,omitempty"`
	ScheduledAt           *time.Time           `json:"scheduled_at,omitempty"`
	EstimatedDurationMins int                  `json:"estimated_duration_mins,omitempty"`
	Metadata              map[string]string    `json:"metadata,omitempty"`
	CreatedAt             time.Time            `json:"created_at"`
	UpdatedAt             time.Time            `json:"updated_at"`
}

func ToBookingResponse(booking *domain.Booking) *BookingResponse {
	var artisanID *string
	if booking.ArtisanID != nil {
		artisanID = booking.ArtisanID
	}
	var scheduledAt *time.Time
	if booking.ScheduledAt != nil {
		scheduledAt = booking.ScheduledAt
	}
	return &BookingResponse{
		ID:                    booking.ID,
		CustomerID:            booking.CustomerID,
		ArtisanID:             artisanID,
		ServiceCategoryID:     booking.ServiceCategoryID,
		Title:                 booking.Title,
		Description:           booking.Description,
		CustomerAddressID:     booking.CustomerAddressID,
		Status:                booking.Status,
		Priority:              booking.Priority,
		ScheduledAt:           scheduledAt,
		EstimatedDurationMins: booking.EstimatedDurationMins,
		Metadata:              booking.Metadata,
		CreatedAt:             booking.CreatedAt,
		UpdatedAt:             booking.UpdatedAt,
	}
}
