package payment

import (
	"context"
	"net/http"
	"sync"
	"time"

	"encore.app/core/cache"
	coredb "encore.app/core/db"
	corerelay "encore.app/core/relay"
	pdomain "encore.app/payment/domain"
	phandlers "encore.app/payment/handlers"
	pproviders "encore.app/payment/providers"
	prelay "encore.app/payment/relay"
	prepos "encore.app/payment/repository"
	"encore.dev/cron"
	"encore.dev/storage/sqldb"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const webhookEventRetention = 90 * 24 * time.Hour

var PaymentDB = sqldb.NewDatabase("payment", sqldb.DatabaseConfig{
	Migrations: "./migrations",
})

// secrets is populated by the Encore framework at runtime.
var secrets struct {
	NombaClientID     string
	NombaClientSecret string
	NombaSignatureKey string
	NombaAccountID    string
	NombaApi          string
	PublicBaseURL     string
	AppReturnURL      string
	ReturnContextKey  string
}

//encore:service
type Service struct {
	paymentsHandler *phandlers.PaymentsHandler
	relay           *prelay.OutboxRelay
}

var serviceInstance *Service
var serviceOnce sync.Once

func initService() (*Service, error) {
	var initErr error

	serviceOnce.Do(func() {
		handlerConfig, err := phandlers.NewPaymentConfig(secrets.PublicBaseURL, secrets.AppReturnURL, secrets.ReturnContextKey)
		if err != nil {
			initErr = err
			return
		}

		paymentGormDB, err := gorm.Open(postgres.New(postgres.Config{
			Conn: PaymentDB.Stdlib(),
		}), &gorm.Config{})
		if err != nil {
			initErr = err
			return
		}

		coreGormDB, err := gorm.Open(postgres.New(postgres.Config{
			Conn: coredb.ProtisanDB.Stdlib(),
		}), &gorm.Config{})
		if err != nil {
			initErr = err
			return
		}

		repo := prepos.NewTransactionRepository(paymentGormDB, coreGormDB)

		nombaProvider, err := pproviders.NewNombaProvider(
			cache.NewInMemoryCache(),
			secrets.NombaApi,
			secrets.NombaClientID,
			secrets.NombaClientSecret,
			secrets.NombaSignatureKey,
			secrets.NombaAccountID,
		)
		if err != nil {
			initErr = err
			return
		}

		providers := map[string]pdomain.PaymentProvider{
			nombaProvider.Identifier(): nombaProvider,
		}

		relayInstance := prelay.NewOutboxRelay(coreGormDB, corerelay.DefaultConfig())
		relayInstance.Start(context.Background())

		serviceInstance = &Service{
			paymentsHandler: phandlers.NewPaymentsHandler(repo, providers, phandlers.WithPaymentConfig(handlerConfig)),
			relay:           relayInstance,
		}
	})

	return serviceInstance, initErr
}

//encore:api auth method=POST path=/v1/payments/initialize
func (s *Service) InitializePayment(ctx context.Context, req *phandlers.InitializePaymentRequest) (*phandlers.InitializePaymentResponse, error) {
	return s.paymentsHandler.InitializePayment(ctx, req)
}

//encore:api auth method=GET path=/v1/payments/transactions/:transactionId
func (s *Service) GetPayment(ctx context.Context, transactionId string) (*phandlers.PaymentStatusResponse, error) {
	return s.paymentsHandler.GetPayment(ctx, transactionId)
}

//encore:api public raw path=/v1/payments/webhook/nomba
func (s *Service) NombaWebhook(w http.ResponseWriter, req *http.Request) {
	s.paymentsHandler.NombaWebhook(w, req)
}

func (s *Service) ProviderWebhook(providerID string, w http.ResponseWriter, req *http.Request) {
	s.paymentsHandler.ProviderWebhook(providerID, w, req)
}

var _ = cron.NewJob("payment-outbox-repair", cron.JobConfig{
	Title:    "Repair Missing Payment Outbox Events",
	Every:    5 * cron.Minute,
	Endpoint: RepairPaymentOutboxEvents,
})

//encore:api private method=POST path=/internal/payments/repair-outbox-events
func RepairPaymentOutboxEvents(ctx context.Context) error {
	svc, err := initService()
	if err != nil {
		return err
	}
	_, err = svc.paymentsHandler.RepairPaymentOutboxEvents(ctx)
	return err
}

var _ = cron.NewJob("payment-webhook-event-cleanup", cron.JobConfig{
	Title:    "Cleanup Processed Payment Webhook Events",
	Every:    24 * 60 * cron.Minute,
	Endpoint: CleanupWebhookEvents,
})

//encore:api private method=POST path=/internal/payments/cleanup-webhook-events
func CleanupWebhookEvents(ctx context.Context) error {
	svc, err := initService()
	if err != nil {
		return err
	}
	return svc.paymentsHandler.CleanupWebhookEvents(ctx, time.Now().Add(-webhookEventRetention))
}

//encore:api private method=GET path=/internal/payments/diagnostics
func PaymentDiagnostics(ctx context.Context) (*phandlers.PaymentDiagnosticsResponse, error) {
	svc, err := initService()
	if err != nil {
		return nil, err
	}
	return svc.paymentsHandler.Diagnostics(ctx)
}

//encore:api public raw path=/checkout/complete
func (s *Service) CheckoutComplete(w http.ResponseWriter, req *http.Request) {
	s.paymentsHandler.HostedCheckoutCallback(w, req, "complete")
}

//encore:api public raw path=/checkout/cancel
func (s *Service) CheckoutCancel(w http.ResponseWriter, req *http.Request) {
	s.paymentsHandler.HostedCheckoutCallback(w, req, "cancel")
}
