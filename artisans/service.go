package artisans

import (
	"context"

	"encore.app/artisans/domain"
	"encore.app/artisans/handlers"
	"encore.app/artisans/internal"
	"encore.app/artisans/repository"
	"encore.app/core"
	"encore.app/core/db"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

//encore:service
type Service struct {
	profileHandler    *handlers.ProfileHandler
	searchHandler     *handlers.SearchHandler
	ratesHandler      *handlers.RatesHandler
	categoriesHandler *handlers.CategoriesHandler
}

func initService() (*Service, error) {
	// Initialize GORM connection using core database
	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: db.ProtisanDB.Stdlib(),
	}), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Initialize core service for shared infrastructure
	coreSvc := core.NewCoreService(gormDB)

	// Initialize layers (bottom-up)
	logger := internal.NewLogger()
	authHelper := internal.NewAuthHelper(logger)
	artisanRepo := repository.NewArtisanRepository(coreSvc.DB())
	validator := domain.NewValidator()
	profileService := domain.NewProfileService(artisanRepo, validator, logger, coreSvc)
	profileHandler := handlers.NewProfileHandler(profileService, authHelper, logger)

	searchService := domain.NewSearchService(artisanRepo, logger)
	searchHandler := handlers.NewSearchHandler(searchService, authHelper, logger)

	// Initialize rates layer
	ratesRepo := repository.NewRatesRepository(coreSvc.DB())
	ratesService := domain.NewRatesService(artisanRepo, ratesRepo, logger)
	ratesHandler := handlers.NewRatesHandler(ratesService, authHelper, logger)

	// Initialize categories handler
	categoriesHandler := handlers.NewCategoriesHandler(ratesRepo, logger)

	return &Service{
		profileHandler:    profileHandler,
		searchHandler:     searchHandler,
		ratesHandler:      ratesHandler,
		categoriesHandler: categoriesHandler,
	}, nil
}

// API endpoints
//
//encore:api auth method=POST path=/v0/artisans/profile
func (s *Service) CreateProfile(ctx context.Context, req *handlers.CreateProfileRequest) (*handlers.CompleteProfileResponse, error) {
	return s.profileHandler.Create(ctx, req)
}

//encore:api auth method=PUT path=/v0/artisans/profile
func (s *Service) UpdateProfile(ctx context.Context, req *handlers.UpdateProfileRequest) (*handlers.ProfileResponse, error) {
	return s.profileHandler.Update(ctx, req)
}

//encore:api auth method=PUT path=/v0/artisans/availability
func (s *Service) UpdateAvailability(ctx context.Context, req *handlers.UpdateAvailabilityRequest) (*handlers.ProfileResponse, error) {
	return s.profileHandler.UpdateAvailability(ctx, req)
}

//encore:api auth method=GET path=/v0/artisans/profile
func (s *Service) GetProfile(ctx context.Context) (*handlers.CompleteProfileResponse, error) {
	return s.profileHandler.Get(ctx)
}

//encore:api public method=GET path=/v0/artisans/profile/:id
func (s *Service) GetArtisanProfile(ctx context.Context, id string) (*domain.PublicArtisanProfile, error) {
	return s.profileHandler.GetArtisanProfile(ctx, id)
}

//encore:api public method=GET path=/v0/artisans/rates/:id
func (s *Service) GetArtisanRates(ctx context.Context, id string) (*handlers.GetArtisanRatesResponse, error) {
	return s.ratesHandler.GetArtisanRatesByArtisanID(ctx, id)
}

//encore:api auth method=PUT path=/v0/artisans/rates
func (s *Service) UpdateRates(ctx context.Context, req *handlers.UpdateArtisanRatesRequest) (*handlers.UpdateArtisanRatesResponse, error) {
	return s.ratesHandler.UpdateRates(ctx, req)
}

//encore:api public method=GET path=/v0/artisans/categories
func (s *Service) GetServiceCategories(ctx context.Context) (*handlers.CategoriesResponse, error) {
	return s.categoriesHandler.GetAllCategories(ctx)
}

//encore:api auth method=GET path=/v0/artisans/services
func (s *Service) GetServices(ctx context.Context, req *handlers.GetServicesRequest) (*handlers.ServicesResponseEnhanced, error) {
	return s.categoriesHandler.GetAllServices(ctx, req)
}

//encore:api public method=POST path=/v0/artisans/search
func (s *Service) SearchArtisans(ctx context.Context, req *handlers.SearchArtisansRequest) (*handlers.SearchArtisansResponse, error) {
	return s.searchHandler.Search(ctx, req)
}
