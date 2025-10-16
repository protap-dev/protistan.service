package booking

import (
	"context"

	"encore.app/booking/domain"
	"encore.app/booking/handlers"
	"encore.app/booking/events"
	binternal "encore.app/booking/internal"
	"encore.app/booking/repository"
	"encore.app/core"
	"encore.app/core/cache"
	"encore.dev/storage/sqldb"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

//encore:service
type Service struct {
	bookingsHandler *handlers.BookingsHandler
}

// BookingDB initializes the booking service database
var BookingDB = sqldb.NewDatabase("booking", sqldb.DatabaseConfig{
	Migrations: "./migrations",
})

// NewService creates a new booking service
func initService() (*Service, error) {
	// Initialize GORM connection using booking service's own database
	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: BookingDB.Stdlib(),
	}), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Initialize core service for shared infrastructure
	coreSvc := core.NewCoreService(gormDB)

	// Initialize dependencies
	logger := binternal.NewServiceLogger("booking")
	authHelper := binternal.NewAuthHelper(logger)
	validator := domain.NewBookingValidator()
	repo := repository.NewBookingRepository(coreSvc.DB())
	cache := cache.NewInMemoryCache()
	publisher := events.NewEventPublisher()

	// Initialize handlers layer
	bookingsHandler := handlers.NewBookingsHandler(repo, validator, logger, cache, coreSvc, authHelper, publisher)

	return &Service{
		bookingsHandler: bookingsHandler,
	}, nil
}

//encore:api auth method=POST path=/bookings
func (s *Service) CreateBooking(ctx context.Context, req *handlers.CreateBookingRequest) (*handlers.BookingResponse, error) {
	return s.bookingsHandler.CreateBooking(ctx, req)
}

//encore:api auth method=PUT path=/bookings/:id/status
func (s *Service) UpdateBookingStatus(ctx context.Context, id string, req *handlers.UpdateStatusRequest) (*handlers.BookingResponse, error) {
	return s.bookingsHandler.UpdateBookingStatus(ctx, id, req)
}

//encore:api auth method=GET path=/bookings/:id
func (s *Service) GetBooking(ctx context.Context, id string) (*handlers.BookingResponse, error) {
	return s.bookingsHandler.GetBooking(ctx, id)
}

//encore:api auth method=GET path=/bookings
func (s *Service) ListBookings(ctx context.Context, params *handlers.ListBookingsParams) (*handlers.ListBookingsResponse, error) {
	return s.bookingsHandler.ListBookings(ctx, params)
}

//encore:api auth method=POST path=/bookings/:id/cancel
func (s *Service) CancelBooking(ctx context.Context, id string, req *handlers.CancelBookingRequest) (*handlers.BookingResponse, error) {
	return s.bookingsHandler.CancelBooking(ctx, id, req)
}
