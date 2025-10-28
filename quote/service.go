package quote

import (
	"context"
	"sync"

	"encore.app/core"
	"encore.app/core/cache"
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

var serviceInstance *Service
var serviceOnce sync.Once

// initService creates or returns the singleton service instance
func initService() (*Service, error) {
	var initErr error

	serviceOnce.Do(func() {
		// Initialize GORM connection
		gormDB, err := gorm.Open(postgres.New(postgres.Config{
			Conn: QuoteDB.Stdlib(),
		}), &gorm.Config{})
		if err != nil {
			initErr = err
			return
		}

		// Initialize core service
		coreSvc := core.NewCoreService(gormDB)

		// Initialize dependencies (SHARED across all requests)
		logger := qinternal.NewServiceLogger("quote")
		authHelper := qinternal.NewAuthHelper(logger)
		validator := domain.NewQuoteValidator()
		repo := repository.NewQuoteRepository(coreSvc.DB())
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
		relayInstance := relay.NewOutboxRelay(coreSvc.DB(), relayConfig)
		go relayInstance.Start(context.Background())

		serviceInstance = &Service{
			QuotesHandler: QuotesHandler,
			relay:         relayInstance,
		}
	})

	return serviceInstance, initErr
}

// QuoteRequest represents a quote request
type QuoteRequest struct {
	BookingID             string `json:"booking_id"`
	EstimatedDurationMins int    `json:"estimated_duration_mins"`
	ServiceCategoryID     string `json:"service_category_id"`
}

// AcceptQuoteRequest represents accepting a quote
type AcceptQuoteRequest struct {
	BookingID string `json:"booking_id"`
}

// RejectQuoteRequest represents rejecting a quote
type RejectQuoteRequest struct {
	BookingID string  `json:"booking_id"`
	Reason    *string `json:"reason,omitempty"`
}

// QuoteResponse represents a quote
type QuoteResponse struct {
	ID        string  `json:"id"`
	BookingID string  `json:"booking_id"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
	Status    string  `json:"status"`
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
