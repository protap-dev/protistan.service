package domain

import (
	"context"
	"time"

	eventscommon "encore.app/core/events"
)

// TransactionRepository defines the data layer boundaries for handling Payment Transactions
type TransactionRepository interface {
	Create(ctx context.Context, txn *Transaction) error
	Update(ctx context.Context, txn *Transaction) error
	GetByID(ctx context.Context, id string) (*Transaction, error)
	GetByInternalRef(ctx context.Context, ref string) (*Transaction, error)
	GetPendingByBookingAndQuote(ctx context.Context, bookingID, quoteID string) (*Transaction, error)
	CreateAttempt(ctx context.Context, attempt *PaymentAttempt) error
	UpdateAttempt(ctx context.Context, attempt *PaymentAttempt) error
	GetAttemptByInternalRef(ctx context.Context, ref string) (*PaymentAttempt, error)
	GetCurrentAttemptByTransactionID(ctx context.Context, transactionID string) (*PaymentAttempt, error)
	CountAttemptsByTransactionID(ctx context.Context, transactionID string) (int64, error)
	HasWebhookEvent(ctx context.Context, provider, requestID string) (bool, error)
	RecordWebhookEvent(ctx context.Context, provider, requestID string, receivedAt time.Time) (bool, error)
	DeleteWebhookEventsBefore(ctx context.Context, cutoff time.Time) error
	ListTransactionsMissingPaymentEvent(ctx context.Context, cutoff time.Time, limit int) ([]*Transaction, error)
	CountPendingTransactionsOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
	CountTransactionsMissingPaymentEvent(ctx context.Context, cutoff time.Time) (int64, error)
	CountWebhookEventsSince(ctx context.Context, since time.Time) (int64, error)
	WithTransaction(ctx context.Context, fn func(TransactionRepository) error) error

	// CreatePaymentEventInOutbox securely writes payment events crossing service boundaries safely.
	CreatePaymentEventInOutbox(ctx context.Context, topic string, event *eventscommon.EventEnvelope[PaymentEvent]) error
}
