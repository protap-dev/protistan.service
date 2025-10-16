package handlers

import (
	"time"

	"encore.app/booking/domain"
)

type CreateBookingRequest struct {
	ServiceCategoryID     string            `json:"service_category_id"`
	Title                 string            `json:"title"`
	Description           string            `json:"description,omitempty"`
	CustomerAddressID     string            `json:"customer_address_id"`
	Priority              string            `json:"priority,omitempty"`
	ScheduledAt           *time.Time        `json:"scheduled_at,omitempty"`
	EstimatedDurationMins int               `json:"estimated_duration_mins,omitempty"`
	Metadata              map[string]string `json:"metadata,omitempty"`
}

type UpdateStatusRequest struct {
	Status string  `json:"status"`
	Reason *string `json:"reason,omitempty"`
}

type CancelBookingRequest struct {
	Reason *string `json:"reason,omitempty"`
}

type ListBookingsParams struct {
	Status string `json:"status,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

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

type ListBookingsResponse struct {
	Bookings []*BookingResponse `json:"bookings"`
	Total    int                `json:"total"`
	Offset   int                `json:"offset"`
	Limit    int                `json:"limit"`
}
