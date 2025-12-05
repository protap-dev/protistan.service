package domain

import (
	"context"
	"errors"
	"slices"
	"strings"

	"encore.app/artisans/internal"
)

// RatesService handles artisan rates operations
type RatesService struct {
	artisanRepo ArtisanRepository
	ratesRepo   RatesRepository
	logger      internal.Logger
}

// NewRatesService creates a new rates service
func NewRatesService(artisanRepo ArtisanRepository, ratesRepo RatesRepository, logger internal.Logger) *RatesService {
	return &RatesService{
		artisanRepo: artisanRepo,
		ratesRepo:   ratesRepo,
		logger:      logger,
	}
}

// GetArtisanRatesByArtisanID retrieves all rates for a specific artisan by artisan ID (public endpoint)
func (s *RatesService) GetArtisanRatesByArtisanID(ctx context.Context, artisanID string) (*ArtisanRatesResponse, error) {
	// Verify artisan exists
	_, err := s.artisanRepo.GetByArtisanID(ctx, artisanID)
	if err != nil {
		s.logger.LogError(ctx, "get_artisan_rates_verify_artisan", err)
		return nil, err
	}

	// Get all rates for the artisan
	rates, err := s.ratesRepo.GetByArtisanID(ctx, artisanID)
	if err != nil {
		s.logger.LogError(ctx, "get_artisan_rates", err)
		return nil, err
	}

	// Convert to response format with service information
	ratesWithServices := make([]ArtisanRateWithService, len(rates))
	defaultCurrency := "NGN"

	for i, rate := range rates {
		// Get service information
		service, err := s.ratesRepo.GetServiceByID(ctx, rate.ServiceID)
		if err != nil {
			s.logger.LogError(ctx, "get_service_for_rate", err)
			continue // Skip this rate if service not found
		}

		// Get category information
		category, err := s.ratesRepo.GetCategoryByID(ctx, service.CategoryID)
		if err != nil {
			s.logger.LogError(ctx, "get_category_for_rate", err)
			continue // Skip this rate if category not found
		}

		ratesWithServices[i] = ArtisanRateWithService{
			ArtisanRate:        rate,
			ServiceName:        service.Name,
			ServiceDescription: service.Description,
			CategoryID:         category.ID,
		}

		// Use the first rate's currency as default
		if i == 0 && rate.Currency != "" {
			defaultCurrency = rate.Currency
		}
	}

	return &ArtisanRatesResponse{
		ArtisanID: artisanID,
		Rates:     ratesWithServices,
		Currency:  defaultCurrency,
	}, nil
}

// UpdateArtisanRates updates rates for the authenticated artisan
func (s *RatesService) UpdateArtisanRates(ctx context.Context, userCtx *internal.UserContext, input *UpdateArtisanRatesInput) (*ArtisanRatesResponse, error) {
	// Verify the user is an artisan
	artisan, err := s.artisanRepo.GetByID(ctx, userCtx.ID)
	if err != nil {
		s.logger.LogError(ctx, "update_rates_verify_artisan", err)
		return nil, err
	}

	// Execute all rate updates in a single transaction for atomicity
	err = s.ratesRepo.WithTransaction(ctx, func(txRepo RatesRepository) error {
		for _, rateInput := range input.Rates {
			// Validate that the service exists and is in artisan's allowed categories
			service, err := txRepo.GetServiceByID(ctx, rateInput.ServiceID)
			if err != nil {
				s.logger.LogError(ctx, "service_not_found", err)
				return errors.New("service not found: " + rateInput.ServiceID)
			}

			// Check if artisan is authorized for this service's category
			isAuthorized := slices.Contains(artisan.CategoryIDs, service.CategoryID)

			if !isAuthorized {
				s.logger.LogError(ctx, "unauthorized_service_category", errors.New("artisan not authorized for service category: "+service.CategoryID))
				return errors.New("unauthorized: you can only set rates for services in your approved categories")
			}

			// Check if rate already exists
			existingRate, err := txRepo.GetByArtisanAndServiceID(ctx, artisan.ID, rateInput.ServiceID)

			if err == nil {
				// Update existing rate within transaction
				updates := make(map[string]any)
				if rateInput.HourlyRateCents != nil {
					updates["hourly_rate_cents"] = *rateInput.HourlyRateCents
				}
				if rateInput.MinimumChargeCents != nil {
					updates["minimum_charge_cents"] = *rateInput.MinimumChargeCents
				}
				if rateInput.Currency != "" {
					updates["currency"] = rateInput.Currency
				}

				err = txRepo.Update(ctx, existingRate.ID, updates)
				if err != nil {
					s.logger.LogError(ctx, "update_existing_rate", err)
					return err
				}
			} else {
				// Validate and set currency (use default if empty)
				currency := rateInput.Currency
				if strings.TrimSpace(currency) == "" {
					currency = "NGN" // Default currency
				}

				// Validate currency is allowed
				validCurrencies := []string{"NGN", "USD", "GBP", "EUR"}
				isValidCurrency := slices.Contains(validCurrencies, currency)

				if !isValidCurrency {
					s.logger.LogError(ctx, "invalid_currency", errors.New("invalid currency: "+currency))
					return errors.New("invalid currency. Allowed values: NGN, USD, GBP, EUR")
				}

				// Create new rate within transaction
				newRate := &ArtisanRate{
					ArtisanID:          artisan.ID,
					ServiceID:          rateInput.ServiceID,
					HourlyRateCents:    rateInput.HourlyRateCents,
					MinimumChargeCents: rateInput.MinimumChargeCents,
					Currency:           currency,
				}

				err = txRepo.Create(ctx, newRate)
				if err != nil {
					s.logger.LogError(ctx, "create_new_rate", err)
					return err
				}
			}
		}
		return nil // Transaction will commit
	})

	if err != nil {
		s.logger.LogError(ctx, "update_rates_transaction_failed", err)
		return nil, err // Transaction rolled back automatically
	}

	// Return updated rates using read-only transaction for consistency
	var result *ArtisanRatesResponse
	err = s.ratesRepo.WithReadTransaction(ctx, func(txRepo RatesRepository) error {
		rates, err := txRepo.GetByArtisanID(ctx, artisan.ID)
		if err != nil {
			return err
		}

		// Convert to response format with service information
		ratesWithServices := make([]ArtisanRateWithService, len(rates))
		defaultCurrency := "NGN"

		for i, rate := range rates {
			// Get service information
			service, err := txRepo.GetServiceByID(ctx, rate.ServiceID)
			if err != nil {
				s.logger.LogError(ctx, "get_service_for_rate", err)
				continue // Skip this rate if service not found
			}

			// Get category information
			category, err := txRepo.GetCategoryByID(ctx, service.CategoryID)
			if err != nil {
				s.logger.LogError(ctx, "get_category_for_rate", err)
				continue // Skip this rate if category not found
			}

			ratesWithServices[i] = ArtisanRateWithService{
				ArtisanRate:        rate,
				ServiceName:        service.Name,
				ServiceDescription: service.Description,
				CategoryID:         category.ID,
			}

			// Use the first rate's currency as default
			if i == 0 && rate.Currency != "" {
				defaultCurrency = rate.Currency
			}
		}

		result = &ArtisanRatesResponse{
			ArtisanID: artisan.ID,
			Rates:     ratesWithServices,
			Currency:  defaultCurrency,
		}
		return nil
	})

	if err != nil {
		s.logger.LogError(ctx, "get_rates_read_transaction_failed", err)
		return nil, err
	}

	s.logger.LogUserAction(ctx, "artisan_rates_updated", artisan.ID)
	return result, nil
}
