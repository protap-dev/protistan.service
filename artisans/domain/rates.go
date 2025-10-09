package domain

import "time"

// ArtisanRate represents an artisan's rate for a specific service
type ArtisanRate struct {
	ID                 string    `gorm:"primarykey;type:uuid;default:generate_uuid()" json:"id"`
	ArtisanID          string    `gorm:"column:artisan_id;type:uuid" json:"artisan_id"`
	ServiceID          string    `gorm:"column:service_id;type:uuid" json:"service_id"`
	HourlyRateCents    *int64    `gorm:"column:hourly_rate_cents" json:"hourly_rate_cents,omitempty"`
	MinimumChargeCents *int64    `gorm:"column:minimum_charge_cents" json:"minimum_charge_cents,omitempty"`
	Currency           string    `gorm:"column:currency;default:NGN" json:"currency"`
	CreatedAt          time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt          time.Time `gorm:"column:updated_at" json:"updated_at"`
}

// ArtisanRateWithService represents an artisan rate with associated service information
type ArtisanRateWithService struct {
	ArtisanRate
	ServiceName        string `json:"service_name"`
	ServiceDescription string `json:"service_description"`
	CategoryName       string `json:"category_name"`
}

// ArtisanRatesResponse represents the response for getting artisan rates
type ArtisanRatesResponse struct {
	ArtisanID string                   `json:"artisan_id"`
	Rates     []ArtisanRateWithService `json:"rates"`
	Currency  string                   `json:"currency"`
}

// Input DTOs for rates operations

// UpdateArtisanRatesInput represents input for updating artisan rates
type UpdateArtisanRatesInput struct {
	Rates []ArtisanRateUpdateInput `json:"rates"`
}

// ArtisanRateUpdateInput represents input for updating a single rate
type ArtisanRateUpdateInput struct {
	ServiceID          string `json:"service_id"`
	HourlyRateCents    *int64 `json:"hourly_rate_cents,omitempty"`
	MinimumChargeCents *int64 `json:"minimum_charge_cents,omitempty"`
	Currency           string `json:"currency"`
}

// CreateArtisanRateInput represents input for creating a new rate
type CreateArtisanRateInput struct {
	ServiceID          string `json:"service_id"`
	HourlyRateCents    *int64 `json:"hourly_rate_cents,omitempty"`
	MinimumChargeCents *int64 `json:"minimum_charge_cents,omitempty"`
	Currency           string `json:"currency"`
}

// ServiceRate represents a service with its rate information for an artisan
type ServiceRate struct {
	ServiceID          string  `json:"service_id"`
	ServiceName        string  `json:"service_name"`
	ServiceDescription string  `json:"service_description"`
	CategoryName       string  `json:"category_name"`
	HourlyRateCents    *int64  `json:"hourly_rate_cents,omitempty"`
	MinimumChargeCents *int64  `json:"minimum_charge_cents,omitempty"`
	Currency           string  `json:"currency"`
	IsSet              bool    `json:"is_set"` // Whether the artisan has set a rate for this service
}

// Service represents a service that artisans can offer
type Service struct {
	ID          string `json:"id"`
	CategoryID  string `json:"category_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	BasePrice   *int64 `json:"base_price_cents,omitempty"`
	Currency    string `json:"currency"`
	CreatedAt   time.Time `json:"created_at"`
}

// ServiceCategory represents a category of services
type ServiceCategory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// ServiceWithCategory represents a service with its category information
type ServiceWithCategory struct {
	Service
	Category ServiceCategory `json:"category"`
}

// ServicesResponse represents the response for getting available services
type ServicesResponse struct {
	Categories []ServiceCategoryWithServices `json:"categories"`
}

// ServiceCategoryWithServices represents a category with its services
type ServiceCategoryWithServices struct {
	ServiceCategory
	Services []Service `json:"services"`
}
