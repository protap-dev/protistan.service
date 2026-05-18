package quote

import (
	"context"
	"sync"

	"encore.app/core"
	"encore.app/core/cache"
	coredb "encore.app/core/db"
	corerelay "encore.app/core/relay"

	"encore.app/quote/domain"
	"encore.app/quote/events"
	"encore.app/quote/handlers"
	qinternal "encore.app/quote/internal"
	"encore.app/quote/relay"
	"encore.app/quote/repository"
	"encore.dev/cron"
	"encore.dev/storage/sqldb"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

//encore:service
type Service struct {
	QuotesHandler *handlers.QuotesHandler
	relay         *relay.OutboxRelay // For graceful shutdown
}

// QuoteDB initializes the quote service database
var QuoteDB = sqldb.NewDatabase("quote", sqldb.DatabaseConfig{
	Migrations: "./migrations",
})

type PaymentQuoteResponse = handlers.PaymentQuoteResponse

var serviceInstance *Service
var serviceOnce sync.Once

// initService creates or returns the singleton service instance
func initService() (*Service, error) {
	var initErr error

	serviceOnce.Do(func() {
		// Initialize GORM connection
		QuotegormDB, err := gorm.Open(postgres.New(postgres.Config{
			Conn: QuoteDB.Stdlib(),
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
		coreSvc := core.NewCoreService(QuotegormDB)

		// Initialize dependencies (SHARED across all requests)
		logger := qinternal.NewServiceLogger("quote")
		authHelper := qinternal.NewAuthHelper(logger)
		validator := domain.NewQuoteValidator()
		repo := repository.NewQuoteRepository(QuotegormDB, coreGormDB)
		quoteSvc := domain.NewQuoteService(repo, validator)
		cache := cache.NewInMemoryCache()
		publisher := events.NewEventPublisher()

		// Initialize handlers layer
		QuotesHandler := handlers.NewQuotesHandler(repo, quoteSvc, validator, logger, cache, coreSvc, authHelper, publisher)

		// Initialize outbox relay
		relayConfig := corerelay.Config{
			PollingInterval: qinternal.DefaultOutboxRelayConfig().PollingInterval,
			BatchSize:       qinternal.DefaultOutboxRelayConfig().BatchSize,
			MaxRetries:      qinternal.DefaultOutboxRelayConfig().MaxRetries,
			RetryBaseDelay:  qinternal.DefaultOutboxRelayConfig().RetryBaseDelay,
			RetryMaxDelay:   qinternal.DefaultOutboxRelayConfig().RetryMaxDelay,
			AuditRetention:  qinternal.DefaultOutboxRelayConfig().AuditRetention,
		}
		relayInstance := relay.NewOutboxRelay(coreGormDB, relayConfig)
		go relayInstance.Start(context.Background())

		serviceInstance = &Service{
			QuotesHandler: QuotesHandler,
			relay:         relayInstance,
		}
	})

	return serviceInstance, initErr
}

// ProposeQuote creates and proposes a quote for a booking
//
//encore:api auth method=POST path=/v0/quote
func ProposeQuote(ctx context.Context, req *handlers.ProposeQuoteRequest) (*handlers.QuoteResponse, error) {
	svc, err := initService()
	if err != nil {
		return nil, err
	}
	return svc.QuotesHandler.ProposeQuote(ctx, req)
}

// GetQuote retrieves a quote by its ID
//
//encore:api auth method=GET path=/v0/quote/item/:id
func GetQuote(ctx context.Context, id string) (*handlers.QuoteResponse, error) {
	svc, err := initService()
	if err != nil {
		return nil, err
	}
	return svc.QuotesHandler.GetQuote(ctx, id)
}

// GetQuoteForPayment retrieves the minimal quote data needed for payment initialization.
//
//encore:api private method=GET path=/internal/quote/:id/payment
func GetQuoteForPayment(ctx context.Context, id string) (*handlers.PaymentQuoteResponse, error) {
	svc, err := initService()
	if err != nil {
		return nil, err
	}
	return svc.QuotesHandler.GetQuoteForPayment(ctx, id)
}

// AcceptQuote accepts a proposed quote
//
//encore:api auth method=POST path=/v0/quote/:id/accept
func AcceptQuote(ctx context.Context, id string, req *handlers.AcceptQuoteRequest) (*handlers.QuoteResponse, error) {
	svc, err := initService()
	if err != nil {
		return nil, err
	}
	return svc.QuotesHandler.AcceptQuote(ctx, id, req)
}

// RejectQuote rejects a proposed quote
//
//encore:api auth method=POST path=/v0/quote/:id/reject
func RejectQuote(ctx context.Context, id string, req *handlers.RejectQuoteRequest) (*handlers.QuoteResponse, error) {
	svc, err := initService()
	if err != nil {
		return nil, err
	}
	return svc.QuotesHandler.RejectQuote(ctx, id, req)
}

// ListQuotesByBooking lists all quotes for a booking
//
//encore:api auth method=GET path=/v0/quote/for-booking/:bookingID
func ListQuotesByBooking(ctx context.Context, bookingID string) (*handlers.ListQuotesResponse, error) {
	svc, err := initService()
	if err != nil {
		return nil, err
	}
	return svc.QuotesHandler.ListQuotesByBooking(ctx, bookingID)
}

// Cron job for expiring quotes
var _ = cron.NewJob("quote-expiry", cron.JobConfig{
	Title:    "Expire Pending Quotes",
	Every:    5 * cron.Minute,
	Endpoint: ExpireQuotes,
})

//encore:api private method=POST path=/internal/quote/expire
func ExpireQuotes(ctx context.Context) error {
	s, err := initService()
	if err != nil {
		return err
	}

	return s.QuotesHandler.ExpireQuotes(ctx)
}
