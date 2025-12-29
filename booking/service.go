package booking

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"encore.app/booking/domain"
	"encore.app/booking/events"
	"encore.app/booking/handlers"
	binternal "encore.app/booking/internal"
	"encore.app/booking/relay"
	"encore.app/booking/repository"
	"encore.app/core"
	"encore.app/core/cache"
	coredb "encore.app/core/db"
	eventscommon "encore.app/core/events"
	topics_payment "encore.app/core/events/topics/payment"
	topics_quote "encore.app/core/events/topics/quotes"
	corerelay "encore.app/core/relay"
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

var serviceInstance *Service
var serviceOnce sync.Once

// initService creates or returns the singleton service instance
func initService() (*Service, error) {
	var initErr error

	serviceOnce.Do(func() {
		// Initialize GORM connection
		bookingGormDB, err := gorm.Open(postgres.New(postgres.Config{
			Conn: BookingDB.Stdlib(),
		}), &gorm.Config{})
		if err != nil {
			initErr = err
			return
		}

		coreGormDB, err := gorm.Open(postgres.New(postgres.Config{
			Conn: coredb.ProtisanDB.Stdlib(), // Different connection to protisan core DB
		}), &gorm.Config{})
		if err != nil {
			initErr = err
			return
		}

		// Initialize core service
		coreSvc := core.NewCoreService(bookingGormDB)

		// Initialize dependencies (SHARED across all requests)
		logger := binternal.NewServiceLogger("booking")
		authHelper := binternal.NewAuthHelper(logger)
		validator := domain.NewBookingValidator()
		offerValidator := domain.NewOfferValidator()
		repo := repository.NewBookingRepository(bookingGormDB, coreGormDB)
		cache := cache.NewInMemoryCache() // SINGLE CACHE INSTANCE
		publisher := events.NewEventPublisher()

		// Initialize handlers layer
		bookingsHandler := handlers.NewBookingsHandler(repo, validator, offerValidator, logger, cache, coreSvc, authHelper, publisher)

		// Initialize outbox relay
		relayConfig := corerelay.Config{
			PollingInterval: binternal.DefaultOutboxRelayConfig().PollingInterval,
			BatchSize:       binternal.DefaultOutboxRelayConfig().BatchSize,
			MaxRetries:      binternal.DefaultOutboxRelayConfig().MaxRetries,
			RetryBaseDelay:  binternal.DefaultOutboxRelayConfig().RetryBaseDelay,
			RetryMaxDelay:   binternal.DefaultOutboxRelayConfig().RetryMaxDelay,
			AuditRetention:  binternal.DefaultOutboxRelayConfig().AuditRetention,
		}
		relayInstance := relay.NewOutboxRelay(coreGormDB, relayConfig)
		go relayInstance.Start(context.Background())

		serviceInstance = &Service{
			bookingsHandler: bookingsHandler,
			relay:           relayInstance,
		}
	})

	return serviceInstance, initErr
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
func (s *Service) ListArtisanOffers(ctx context.Context, params *handlers.ListOffersParams) (*handlers.ListArtisanOffersResponse, error) {
	return s.bookingsHandler.ListArtisanOffers(ctx, params)
}

// =============================
// Event Subscribers
// =============================

var _ = pubsub.NewSubscription(
	topics_quote.QuoteAcceptedTopic, "handle-quote-accepted",
	pubsub.SubscriptionConfig[eventscommon.EventEnvelope[eventscommon.QuoteEvent]]{ // Remove pointer
		Handler: func(ctx context.Context, envelope eventscommon.EventEnvelope[eventscommon.QuoteEvent]) error { // Remove pointer
			ctx = eventscommon.WithEventMetadata(ctx, &binternal.EventMetadata{
				CorrelationID: envelope.CorrelationID,
				CausationID:   envelope.EventID,
				UserID:        envelope.Data.UserID,
			})

			s, err := initService()
			if err != nil {
				return err
			}
			return s.OnQuoteAccepted(ctx, &envelope.Data)
		},
	},
)

var _ = pubsub.NewSubscription(
	topics_quote.QuoteRejectedTopic, "handle-quote-rejected",
	pubsub.SubscriptionConfig[eventscommon.EventEnvelope[eventscommon.QuoteEvent]]{ // Remove pointer
		Handler: func(ctx context.Context, envelope eventscommon.EventEnvelope[eventscommon.QuoteEvent]) error { // Remove pointer
			ctx = eventscommon.WithEventMetadata(ctx, &binternal.EventMetadata{
				CorrelationID: envelope.CorrelationID,
				CausationID:   envelope.EventID,
				UserID:        envelope.Data.UserID,
			})

			s, err := initService()
			if err != nil {
				return err
			}
			return s.OnQuoteRejected(ctx, &envelope.Data)
		},
	},
)

var _ = pubsub.NewSubscription(
	topics_payment.PaymentConfirmedTopic, "handle-payment-confirmed",
	pubsub.SubscriptionConfig[*eventscommon.EventEnvelope[domain.BookingEvent]]{
		Handler: func(ctx context.Context, envelope *eventscommon.EventEnvelope[domain.BookingEvent]) error {
			domainEvent := convertToDomainEvent(&envelope.Data)

			ctx = eventscommon.WithEventMetadata(ctx, &binternal.EventMetadata{
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
	topics_payment.PaymentFailedTopic, "handle-payment-failed",
	pubsub.SubscriptionConfig[*eventscommon.EventEnvelope[domain.BookingEvent]]{
		Handler: func(ctx context.Context, envelope *eventscommon.EventEnvelope[domain.BookingEvent]) error {
			domainEvent := convertToDomainEvent(&envelope.Data)

			ctx = eventscommon.WithEventMetadata(ctx, &binternal.EventMetadata{
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

var _ = pubsub.NewSubscription(
	topics_quote.QuoteProposedTopic, "handle-quote-proposed",
	pubsub.SubscriptionConfig[eventscommon.EventEnvelope[eventscommon.QuoteEvent]]{
		Handler: func(ctx context.Context, envelope eventscommon.EventEnvelope[eventscommon.QuoteEvent]) error {

			ctx = eventscommon.WithEventMetadata(ctx, &binternal.EventMetadata{
				CorrelationID: envelope.CorrelationID,
				CausationID:   envelope.EventID,
				UserID:        envelope.Data.UserID,
			})

			s, err := initService()
			if err != nil {
				return err
			}
			result := s.OnQuoteProposed(ctx, &envelope.Data)
			return result
		},
	},
)

// Helper function to convert common event to domain event
func convertToDomainEvent(commonEvent *domain.BookingEvent) *domain.BookingEvent {
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

func (s *Service) OnQuoteProposed(ctx context.Context, quoteEvent *eventscommon.QuoteEvent) error {
	current, err := s.bookingsHandler.GetRepository().GetByID(ctx, quoteEvent.BookingID)
	if err != nil {
		log.Printf("[ERROR] Failed to get booking %s in OnQuoteProposed: %v",
			quoteEvent.BookingID, err)
		return err
	}

	rawData, _ := json.Marshal(quoteEvent)

	err = s.bookingsHandler.UpdateBookingStatusInternal(
		ctx,
		quoteEvent.BookingID,
		domain.BookingQuoteProposed,
		quoteEvent.UserID,
		nil,
		current,
		nil, // No manual metadata needed, details are in RawData
		rawData,
	)

	if err != nil {
		log.Printf("[ERROR] Failed to update booking %s to quote_proposed: %v",
			quoteEvent.BookingID, err)
		return err
	}

	return nil
}

func (s *Service) OnQuoteAccepted(ctx context.Context, quoteEvent *eventscommon.QuoteEvent) error {
	current, err := s.bookingsHandler.GetRepository().GetByID(ctx, quoteEvent.BookingID)
	if err != nil {
		return err
	}
	return s.bookingsHandler.UpdateBookingStatusInternal(
		ctx,
		quoteEvent.BookingID,
		domain.BookingQuoteAccepted,
		quoteEvent.UserID,
		nil,
		current,
		nil,
		nil,
	)
}

func (s *Service) OnQuoteRejected(ctx context.Context, quoteEvent *eventscommon.QuoteEvent) error {
	current, err := s.bookingsHandler.GetRepository().GetByID(ctx, quoteEvent.BookingID)
	if err != nil {
		return err
	}

	// Build reason from rejection code
	var reason *string
	if quoteEvent.RejectionReasonCode != nil {
		reasonText := fmt.Sprintf("quote_rejected: %s", *quoteEvent.RejectionReasonCode)
		reason = &reasonText
	}

	// Transition back to Assigned so artisan can propose a new quote
	return s.bookingsHandler.UpdateBookingStatusInternal(
		ctx,
		quoteEvent.BookingID,
		domain.BookingAssigned,
		quoteEvent.UserID,
		reason,
		current,
		nil,
		nil,
	)
}

func (s *Service) OnPaymentConfirmed(ctx context.Context, event *domain.BookingEvent) error {
	current, err := s.bookingsHandler.GetRepository().GetByID(ctx, event.BookingID)
	if err != nil {
		return err
	}
	return s.bookingsHandler.UpdateBookingStatusInternal(ctx, event.BookingID, domain.BookingConfirmed, event.UserID, nil, current, nil, nil)
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
	return s.bookingsHandler.UpdateBookingStatusInternal(ctx, event.BookingID, domain.BookingQuoteAccepted, event.UserID, event.Reason, current, nil, nil)
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
