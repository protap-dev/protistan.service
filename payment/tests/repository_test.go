package tests

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"encore.app/payment/domain"
	"encore.app/payment/repository"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupPaymentMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	return gormDB, mock, sqlDB
}

func TestGetPendingByBookingAndQuoteReturnsNilWithoutRecordNotFound(t *testing.T) {
	db, mock, sqlDB := setupPaymentMockDB(t)
	defer sqlDB.Close()

	repo := repository.NewTransactionRepository(db, db)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "transactions" WHERE booking_id = $1 AND quote_id = $2 AND status = $3 ORDER BY created_at DESC LIMIT $4 FOR UPDATE`)).
		WithArgs("booking-1", "quote-1", string(domain.TxPending), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "customer_id", "booking_id", "quote_id", "amount_cents", "currency",
			"provider", "method", "status", "internal_ref", "provider_ref",
			"metadata", "created_at", "updated_at",
		}))

	txn, err := repo.GetPendingByBookingAndQuote(context.Background(), "booking-1", "quote-1")

	require.NoError(t, err)
	require.Nil(t, txn)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetAttemptByInternalRefReturnsNilWithoutRecordNotFound(t *testing.T) {
	db, mock, sqlDB := setupPaymentMockDB(t)
	defer sqlDB.Close()

	repo := repository.NewTransactionRepository(db, db)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "payment_attempts" WHERE internal_ref = $1 ORDER BY "payment_attempts"."id" LIMIT $2`)).
		WithArgs("attempt-missing", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "transaction_id", "attempt_no", "provider", "status", "internal_ref",
			"provider_ref", "checkout_url", "return_url", "return_context",
			"return_context_expires_at", "cancel_requested_at", "cancelled_at",
			"superseded_by_attempt_id", "created_by_user_id", "created_at", "updated_at",
		}))

	attempt, err := repo.GetAttemptByInternalRef(context.Background(), "attempt-missing")

	require.NoError(t, err)
	require.Nil(t, attempt)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetByIDConvertsNonStringMetadataValues(t *testing.T) {
	db, mock, sqlDB := setupPaymentMockDB(t)
	defer sqlDB.Close()

	repo := repository.NewTransactionRepository(db, db)
	now := time.Now().UTC()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "transactions" WHERE id = $1 ORDER BY "transactions"."id" LIMIT $2`)).
		WithArgs("txn-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "customer_id", "booking_id", "quote_id", "amount_cents", "currency",
			"provider", "method", "status", "internal_ref", "provider_ref",
			"current_attempt_id", "metadata", "created_at", "updated_at",
		}).AddRow(
			"txn-1", "customer-1", "booking-1", "quote-1", int64(1050000), "NGN",
			"nomba", "checkout", string(domain.TxPending), "TXN-1", "",
			nil, []byte(`{"string_value":"ok","number_value":123,"bool_value":true,"object_value":{"nested":"value"},"null_value":null}`), now, now,
		))

	txn, err := repo.GetByID(context.Background(), "txn-1")

	require.NoError(t, err)
	require.NotNil(t, txn)
	require.Equal(t, int64(1050000), txn.AmountCents)
	require.Equal(t, "ok", txn.Metadata["string_value"])
	require.Equal(t, "123", txn.Metadata["number_value"])
	require.Equal(t, "true", txn.Metadata["bool_value"])
	require.Equal(t, `{"nested":"value"}`, txn.Metadata["object_value"])
	require.NotContains(t, txn.Metadata, "null_value")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteWebhookEventsBeforeDeletesOldRows(t *testing.T) {
	db, mock, sqlDB := setupPaymentMockDB(t)
	defer sqlDB.Close()

	repo := repository.NewTransactionRepository(db, db)
	cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "webhook_events" WHERE created_at < $1`)).
		WithArgs(cutoff).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()

	err := repo.DeleteWebhookEventsBefore(context.Background(), cutoff)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListTransactionsMissingPaymentEventFiltersMarkedRows(t *testing.T) {
	db, mock, sqlDB := setupPaymentMockDB(t)
	defer sqlDB.Close()

	repo := repository.NewTransactionRepository(db, db)
	cutoff := time.Now().UTC().Add(-time.Minute)
	createdAt := cutoff.Add(-time.Hour)
	updatedAt := cutoff.Add(-time.Minute)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "transactions" WHERE status IN ($1,$2) AND updated_at < $3 AND COALESCE(metadata->>'payment_status_event_enqueued_for','') <> status ORDER BY updated_at ASC LIMIT $4`)).
		WithArgs(string(domain.TxSuccessful), string(domain.TxFailed), cutoff, 50).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "customer_id", "booking_id", "quote_id", "amount_cents", "currency",
			"provider", "method", "status", "internal_ref", "provider_ref",
			"current_attempt_id", "metadata", "created_at", "updated_at",
		}).AddRow(
			"txn-1", "customer-1", "booking-1", "quote-1", int64(1050000), "NGN",
			"nomba", "checkout", string(domain.TxSuccessful), "TXN-1", "PAY-1",
			nil, []byte(`{"payment_reservation_key":"reservation-1"}`), createdAt, updatedAt,
		))

	txns, err := repo.ListTransactionsMissingPaymentEvent(context.Background(), cutoff, 50)

	require.NoError(t, err)
	require.Len(t, txns, 1)
	require.Equal(t, "txn-1", txns[0].ID)
	require.Equal(t, domain.TxSuccessful, txns[0].Status)
	require.NoError(t, mock.ExpectationsWereMet())
}
