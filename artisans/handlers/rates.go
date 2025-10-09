package handlers

import (
	"context"

	"encore.app/artisans/domain"
	"encore.app/artisans/internal"
)

// Request/Response types for rates API

// GetArtisanRatesRequest represents request for getting artisan rates
type GetArtisanRatesRequest struct {
	ArtisanID string `json:"artisan_id"` // From URL path parameter
}

// UpdateArtisanRatesRequest represents request for updating artisan rates
type UpdateArtisanRatesRequest struct {
	Rates []RateUpdateRequest `json:"rates"`
}

// RateUpdateRequest represents a single rate update
type RateUpdateRequest struct {
	ServiceID          string `json:"service_id"`
	HourlyRateCents    *int64 `json:"hourly_rate_cents,omitempty"`
	MinimumChargeCents *int64 `json:"minimum_charge_cents,omitempty"`
	Currency           string `json:"currency"`
}

// GetArtisanRatesResponse represents response for getting artisan rates
type GetArtisanRatesResponse struct {
	ArtisanID string                `json:"artisan_id"`
	Rates     []RateWithServiceInfo `json:"rates"`
	Currency  string                `json:"currency"`
}

// RateWithServiceInfo represents a rate with associated service information
type RateWithServiceInfo struct {
	ServiceID          string `json:"service_id"`
	ServiceName        string `json:"service_name"`
	ServiceDescription string `json:"service_description"`
	CategoryName       string `json:"category_name"`
	HourlyRateCents    *int64 `json:"hourly_rate_cents,omitempty"`
	MinimumChargeCents *int64 `json:"minimum_charge_cents,omitempty"`
	Currency           string `json:"currency"`
}

// UpdateArtisanRatesResponse represents response for updating artisan rates
type UpdateArtisanRatesResponse struct {
	ArtisanID string                `json:"artisan_id"`
	Rates     []RateWithServiceInfo `json:"rates"`
	Currency  string                `json:"currency"`
}

// RatesHandler handles artisan rates operations
type RatesHandler struct {
	service    *domain.RatesService
	authHelper *internal.AuthHelper
	logger     internal.Logger
}

// NewRatesHandler creates a new rates handler
func NewRatesHandler(service *domain.RatesService, authHelper *internal.AuthHelper, logger internal.Logger) *RatesHandler {
	return &RatesHandler{
		service:    service,
		authHelper: authHelper,
		logger:     logger,
	}
}

// GetArtisanRates handles GET /artisans/rates/:id
func (h *RatesHandler) GetArtisanRates(ctx context.Context, artisanID string) (*GetArtisanRatesResponse, error) {
	result, err := h.service.GetArtisanRates(ctx, artisanID)
	if err != nil {
		return nil, err
	}

	// Convert domain response to handler response
	rates := make([]RateWithServiceInfo, len(result.Rates))
	for i, rate := range result.Rates {
		rates[i] = RateWithServiceInfo{
			ServiceID:          rate.ServiceID,
			ServiceName:        rate.ServiceName,
			ServiceDescription: rate.ServiceDescription,
			CategoryName:       rate.CategoryName,
			HourlyRateCents:    rate.HourlyRateCents,
			MinimumChargeCents: rate.MinimumChargeCents,
			Currency:           rate.Currency,
		}
	}

	return &GetArtisanRatesResponse{
		ArtisanID: result.ArtisanID,
		Rates:     rates,
		Currency:  result.Currency,
	}, nil
}

// UpdateRates handles PUT /artisans/rates
func (h *RatesHandler) UpdateRates(ctx context.Context, req *UpdateArtisanRatesRequest) (*UpdateArtisanRatesResponse, error) {
	// Extract user context to verify artisan
	userCtx, err := h.authHelper.ExtractUserContext(ctx, "update_artisan_rates")
	if err != nil {
		return nil, err
	}

	// Convert handler request to domain input
	rates := make([]domain.ArtisanRateUpdateInput, len(req.Rates))
	for i, rate := range req.Rates {
		rates[i] = domain.ArtisanRateUpdateInput{
			ServiceID:          rate.ServiceID,
			HourlyRateCents:    rate.HourlyRateCents,
			MinimumChargeCents: rate.MinimumChargeCents,
			Currency:           rate.Currency,
		}
	}

	domainReq := &domain.UpdateArtisanRatesInput{
		Rates: rates,
	}

	result, err := h.service.UpdateArtisanRates(ctx, userCtx, domainReq)
	if err != nil {
		return nil, err
	}

	// Convert domain response to handler response
	handlerRates := make([]RateWithServiceInfo, len(result.Rates))
	for i, rate := range result.Rates {
		handlerRates[i] = RateWithServiceInfo{
			ServiceID:          rate.ServiceID,
			ServiceName:        rate.ServiceName,
			ServiceDescription: rate.ServiceDescription,
			CategoryName:       rate.CategoryName,
			HourlyRateCents:    rate.HourlyRateCents,
			MinimumChargeCents: rate.MinimumChargeCents,
			Currency:           rate.Currency,
		}
	}

	return &UpdateArtisanRatesResponse{
		ArtisanID: result.ArtisanID,
		Rates:     handlerRates,
		Currency:  result.Currency,
	}, nil
}
