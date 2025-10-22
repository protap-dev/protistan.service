package booking

import (
	"context"

	"encore.app/booking/domain"
	"encore.app/booking/events"
	"encore.app/booking/handlers"
	binternal "encore.app/booking/internal"
	"encore.app/booking/relay"
	"encore.app/booking/repository"
	"encore.app/core"
	"encore.app/core/cache"
	eventscommon "encore.app/core/events"
	"encore.app/payment"
	"encore.app/quote"
	"encore.dev/cron"
	"encore.dev/pubsub"
	"encore.dev/storage/sqldb"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

//encore:service
type Service struct {
	bookingsHandler *handlers.BookingsHandler
	relay           *relay.OutboxRelay // For graceful shutdown
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
	offerValidator := domain.NewOfferValidator()
	repo := repository.NewBookingRepository(coreSvc.DB())
	cache := cache.NewInMemoryCache()
	publisher := events.NewEventPublisher()

	// Initialize handlers layer
	bookingsHandler := handlers.NewBookingsHandler(repo, validator, offerValidator, logger, cache, coreSvc, authHelper, publisher)

	// Initialize and start outbox relay with production-ready configuration
	relayConfig := relay.Config{
		PollingInterval: binternal.DefaultOutboxRelayConfig().PollingInterval,
		BatchSize:       binternal.DefaultOutboxRelayConfig().BatchSize,
		MaxRetries:      binternal.DefaultOutboxRelayConfig().MaxRetries,
		RetryBaseDelay:  binternal.DefaultOutboxRelayConfig().RetryBaseDelay,
		RetryMaxDelay:   binternal.DefaultOutboxRelayConfig().RetryMaxDelay,
		AuditRetention:  binternal.DefaultOutboxRelayConfig().AuditRetention,
	}
	relayInstance := relay.NewOutboxRelay(coreSvc.DB(), publisher, relayConfig)
	go relayInstance.Start(context.Background())

	svc := &Service{
		bookingsHandler: bookingsHandler,
		relay:           relayInstance, // Store reference for graceful shutdown
	}

	return svc, nil
}

//encore:api auth method=POST path=/v0/bookings
func (s *Service) CreateBooking(ctx context.Context, req *handlers.CreateBookingRequest) (*handlers.BookingResponse, error) {
	return s.bookingsHandler.CreateBooking(ctx, req)
}

//encore:api auth method=PUT path=/v0/bookings/:id/status
func (s *Service) UpdateBookingStatus(ctx context.Context, id string, req *handlers.UpdateStatusRequest) (*handlers.BookingResponse, error) {
	return s.bookingsHandler.UpdateBookingStatus(ctx, id, req)
}

//encore:api auth method=GET path=/v0/bookings/:id
func (s *Service) GetBooking(ctx context.Context, id string) (*handlers.BookingResponse, error) {
	return s.bookingsHandler.GetBooking(ctx, id)
}

//encore:api auth method=GET path=/v0/bookings
func (s *Service) ListBookings(ctx context.Context, params *handlers.ListBookingsParams) (*handlers.ListBookingsResponse, error) {
	return s.bookingsHandler.ListBookings(ctx, params)
}

//encore:api auth method=POST path=/v0/bookings/:id/rematch
func (s *Service) RematchBooking(ctx context.Context, id string, req *handlers.RematchBookingRequest) (*handlers.BookingResponse, error) {
	return s.bookingsHandler.RematchBooking(ctx, id, req)
}

//encore:api auth method=POST path=/v0/bookings/:id/cancel
func (s *Service) CancelBooking(ctx context.Context, id string, req *handlers.CancelBookingRequest) (*handlers.BookingResponse, error) {
	return s.bookingsHandler.CancelBooking(ctx, id, req)
}

// =============================
// Offer Management Endpoints
// =============================

//encore:api auth method=POST path=/v0/bookings/:id/offers
func (s *Service) OfferBooking(ctx context.Context, id string, req *handlers.OfferBookingRequest) (*handlers.OfferResponse, error) {
	return s.bookingsHandler.OfferBooking(ctx, id, req)
}

//encore:api auth method=POST path=/v0/offers/:id/accept
func (s *Service) AcceptOffer(ctx context.Context, id string, req *handlers.AcceptOfferRequest) (*handlers.BookingResponse, error) {
	return s.bookingsHandler.AcceptOffer(ctx, id, req)
}

//encore:api auth method=POST path=/v0/offers/:id/reject
func (s *Service) RejectOffer(ctx context.Context, id string, req *handlers.RejectOfferRequest) (*handlers.OfferResponse, error) {
	return s.bookingsHandler.RejectOffer(ctx, id, req)
}

//encore:api auth method=GET path=/v0/bookings/:id/offers
func (s *Service) ListBookingOffers(ctx context.Context, id string) (*handlers.ListOffersResponse, error) {
	return s.bookingsHandler.ListBookingOffers(ctx, id)
}

//encore:api auth method=GET path=/v0/artisan/offers
func (s *Service) ListArtisanOffers(ctx context.Context, params *handlers.ListOffersParams) (*handlers.ListOffersResponse, error) {
	return s.bookingsHandler.ListArtisanOffers(ctx, params)
}

// =============================
// Event Subscribers
// =============================

var _ = pubsub.NewSubscription(
	quote.QuoteAcceptedTopic, "handle-quote-accepted",
	pubsub.SubscriptionConfig[*eventscommon.EventEnvelope[eventscommon.BookingEvent]]{
		Handler: func(ctx context.Context, envelope *eventscommon.EventEnvelope[eventscommon.BookingEvent]) error {
			// Convert to domain event
			domainEvent := convertToDomainEvent(&envelope.Data)

			ctx = events.WithEventMetadata(ctx, &events.EventMetadata{
				CorrelationID: envelope.CorrelationID,
				CausationID:   envelope.EventID,
				UserID:        envelope.Data.UserID,
			})

			s, err := initService()
			if err != nil {
				return err
			}
			return s.OnQuoteAccepted(ctx, domainEvent)
		},
	},
)

var _ = pubsub.NewSubscription(
	quote.QuoteRejectedTopic, "handle-quote-rejected",
	pubsub.SubscriptionConfig[*eventscommon.EventEnvelope[eventscommon.BookingEvent]]{
		Handler: func(ctx context.Context, envelope *eventscommon.EventEnvelope[eventscommon.BookingEvent]) error {
			domainEvent := convertToDomainEvent(&envelope.Data)

			ctx = events.WithEventMetadata(ctx, &events.EventMetadata{
				CorrelationID: envelope.CorrelationID,
				CausationID:   envelope.EventID,
				UserID:        envelope.Data.UserID,
			})

			s, err := initService()
			if err != nil {
				return err
			}
			return s.OnQuoteRejected(ctx, domainEvent)
		},
	},
)

var _ = pubsub.NewSubscription(
	payment.PaymentConfirmedTopic, "handle-payment-confirmed",
	pubsub.SubscriptionConfig[*eventscommon.EventEnvelope[eventscommon.BookingEvent]]{
		Handler: func(ctx context.Context, envelope *eventscommon.EventEnvelope[eventscommon.BookingEvent]) error {
			domainEvent := convertToDomainEvent(&envelope.Data)

			ctx = events.WithEventMetadata(ctx, &events.EventMetadata{
				CorrelationID: envelope.CorrelationID,
				CausationID:   envelope.EventID,
				UserID:        envelope.Data.UserID,
			})

			s, err := initService()
			if err != nil {
				return err
			}
			return s.OnPaymentConfirmed(ctx, domainEvent)
		},
	},
)

var _ = pubsub.NewSubscription(
	payment.PaymentFailedTopic, "handle-payment-failed",
	pubsub.SubscriptionConfig[*eventscommon.EventEnvelope[eventscommon.BookingEvent]]{
		Handler: func(ctx context.Context, envelope *eventscommon.EventEnvelope[eventscommon.BookingEvent]) error {
			domainEvent := convertToDomainEvent(&envelope.Data)

			ctx = events.WithEventMetadata(ctx, &events.EventMetadata{
				CorrelationID: envelope.CorrelationID,
				CausationID:   envelope.EventID,
				UserID:        envelope.Data.UserID,
			})

			s, err := initService()
			if err != nil {
				return err
			}
			return s.OnPaymentFailed(ctx, domainEvent)
		},
	},
)

// TODO: Add subscription for quote.proposed when quotes service is implemented
// var _ = pubsub.NewSubscription(
//     events.QuoteProposedTopic, "handle-quote-proposed",
//     pubsub.SubscriptionConfig[*domain.BookingEvent]{
//         Handler: pubsub.MethodHandler((*Service).OnQuoteProposed),
//     },
// )

// Helper function to convert common event to domain event
func convertToDomainEvent(commonEvent *eventscommon.BookingEvent) *domain.BookingEvent {
	return &domain.BookingEvent{
		BookingID:      commonEvent.BookingID,
		Status:         domain.BookingStatus(commonEvent.Status),
		PreviousStatus: domain.BookingStatus(commonEvent.PreviousStatus),
		Timestamp:      commonEvent.Timestamp,
		UserID:         commonEvent.UserID,
		ArtisanID:      commonEvent.ArtisanID,
		Reason:         commonEvent.Reason,
	}
}

func (s *Service) OnQuoteProposed(ctx context.Context, event *domain.BookingEvent) error {
	current, err := s.bookingsHandler.GetRepository().GetByID(ctx, event.BookingID)
	if err != nil {
		return err
	}
	return s.bookingsHandler.UpdateBookingStatusInternal(ctx, event.BookingID, domain.BookingQuoteProposed, event.UserID, nil, current)
}

func (s *Service) OnQuoteAccepted(ctx context.Context, event *domain.BookingEvent) error {
	current, err := s.bookingsHandler.GetRepository().GetByID(ctx, event.BookingID)
	if err != nil {
		return err
	}
	return s.bookingsHandler.UpdateBookingStatusInternal(ctx, event.BookingID, domain.BookingQuoteAccepted, event.UserID, nil, current)
}

func (s *Service) OnQuoteRejected(ctx context.Context, event *domain.BookingEvent) error {
	current, err := s.bookingsHandler.GetRepository().GetByID(ctx, event.BookingID)
	if err != nil {
		return err
	}
	// If quote is rejected, transition back to Assigned so artisan can propose a new quote or customer can re-offer
	return s.bookingsHandler.UpdateBookingStatusInternal(ctx, event.BookingID, domain.BookingAssigned, event.UserID, event.Reason, current)
}

func (s *Service) OnPaymentConfirmed(ctx context.Context, event *domain.BookingEvent) error {
	current, err := s.bookingsHandler.GetRepository().GetByID(ctx, event.BookingID)
	if err != nil {
		return err
	}
	return s.bookingsHandler.UpdateBookingStatusInternal(ctx, event.BookingID, domain.BookingConfirmed, event.UserID, nil, current)
}

func (s *Service) OnPaymentFailed(ctx context.Context, event *domain.BookingEvent) error {
	current, err := s.bookingsHandler.GetRepository().GetByID(ctx, event.BookingID)
	if err != nil {
		return err
	}

	// Only handle payment failures for bookings in payment pending state
	if current.Status != domain.BookingPaymentPending {
		// Idempotent - if booking is not in payment pending, we've already handled this failure
		return nil
	}

	// Check if this is actually a payment failure event (indicated by reason)
	if event.Reason == nil || *event.Reason != "payment_failed" {
		// Not a payment failure event, ignore
		return nil
	}

	// Transition back to quote accepted state
	return s.bookingsHandler.UpdateBookingStatusInternal(ctx, event.BookingID, domain.BookingQuoteAccepted, event.UserID, event.Reason, current)
}

// ExpireOffers is a cron job that expires pending offers
var _ = cron.NewJob("offer-expiry", cron.JobConfig{
	Title:    "Expire Pending Offers",
	Every:    10 * cron.Minute,
	Endpoint: ExpireOffers,
})

// ExpireOffers scans for expired pending offers and transitions them
//
//encore:api private method=POST path=/internal/expire-offers
func ExpireOffers(ctx context.Context) error {
	svc, err := initService()
	if err != nil {
		return err
	}

	return svc.bookingsHandler.ExpireOffers(ctx)
}
