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
	SpecificArtisanID     *string           `json:"specific_artisan_id,omitempty"` // Request specific artisan
}

type UpdateStatusRequest struct {
	Status string  `json:"status"`
	Reason *string `json:"reason,omitempty"`
}

type RematchBookingRequest struct {
	Reason *string `json:"reason,omitempty"` // Optional reason for rematch request
}

type CancelBookingRequest struct {
	Reason *string `json:"reason,omitempty"`
}

type ListBookingsParams struct {
	Status string `query:"status,omitempty"`
	Limit  int    `query:"limit,omitempty"`
	Offset int    `query:"offset,omitempty"`
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

// Offer-related DTOs

// OfferBookingRequest is the request to offer a booking to an artisan
type OfferBookingRequest struct {
	ArtisanID string `json:"artisan_id"`         // Artisan to offer the booking to
	ExpiresIn int    `json:"expires_in,omitempty"` // Hours until offer expires (default: 24)
}

// AcceptOfferRequest is the request to accept an offer (empty body)
type AcceptOfferRequest struct{}

// RejectOfferRequest is the request to reject an offer with a reason
type RejectOfferRequest struct {
	Reason string `json:"reason"` // Required: reason for rejection
}

// ListOffersParams contains query parameters for listing offers
type ListOffersParams struct {
	Status string `query:"status"` // Filter by offer status
	Limit  int    `query:"limit"`  // Max number of results
	Offset int    `query:"offset"` // Pagination offset
}

// OfferResponse represents an offer in API responses
type OfferResponse struct {
	ID           string    `json:"id"`
	BookingID    string    `json:"booking_id"`
	ArtisanID    string    `json:"artisan_id"`
	Status       string    `json:"status"`
	OfferedBy    string    `json:"offered_by"`
	OfferedAt    time.Time `json:"offered_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	RespondedAt  *time.Time `json:"responded_at,omitempty"`
	RejectReason *string   `json:"reject_reason,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ListOffersResponse contains a list of offers
type ListOffersResponse struct {
	Offers []*OfferResponse `json:"offers"`
	Total    int              `json:"total"`
}
