package repository

import (
	"context"
	"errors"
	"time"

	"encore.app/core"
	"encore.app/payment/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type transactionRepository struct {
	db     *gorm.DB
	coreDB *gorm.DB
}

func NewTransactionRepository(db *gorm.DB, coreDB *gorm.DB) domain.TransactionRepository {
	return &transactionRepository{db: db, coreDB: coreDB}
}

func (r *transactionRepository) Create(ctx context.Context, txn *domain.Transaction) error {
	model := toDBModel(txn)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	// Sync DB-generated fields back to the domain model.
	txn.ID = model.ID
	txn.CreatedAt = model.CreatedAt
	txn.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *transactionRepository) Update(ctx context.Context, txn *domain.Transaction) error {
	return r.db.WithContext(ctx).Save(toDBModel(txn)).Error
}

func (r *transactionRepository) GetByID(ctx context.Context, id string) (*domain.Transaction, error) {
	var model transactionDBModel
	if err := r.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return toDomainModel(&model), nil
}

func (r *transactionRepository) GetByInternalRef(ctx context.Context, ref string) (*domain.Transaction, error) {
	var model transactionDBModel
	if err := r.db.WithContext(ctx).First(&model, "internal_ref = ?", ref).Error; err != nil {
		return nil, err
	}
	return toDomainModel(&model), nil
}

func (r *transactionRepository) GetPendingByBookingAndQuote(ctx context.Context, bookingID, quoteID string) (*domain.Transaction, error) {
	var model transactionDBModel
	result := r.db.WithContext(ctx).
		Where("booking_id = ? AND quote_id = ? AND status = ?", bookingID, quoteID, string(domain.TxPending)).
		Order("created_at DESC").
		Limit(1).
		Find(&model)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return toDomainModel(&model), nil
}

func (r *transactionRepository) CreateAttempt(ctx context.Context, attempt *domain.PaymentAttempt) error {
	model := toAttemptDBModel(attempt)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	attempt.ID = model.ID
	attempt.CreatedAt = model.CreatedAt
	attempt.UpdatedAt = model.UpdatedAt
	return nil
}

func (r *transactionRepository) UpdateAttempt(ctx context.Context, attempt *domain.PaymentAttempt) error {
	return r.db.WithContext(ctx).Save(toAttemptDBModel(attempt)).Error
}

func (r *transactionRepository) GetAttemptByInternalRef(ctx context.Context, ref string) (*domain.PaymentAttempt, error) {
	var model paymentAttemptDBModel
	if err := r.db.WithContext(ctx).First(&model, "internal_ref = ?", ref).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toDomainAttempt(&model), nil
}

func (r *transactionRepository) GetCurrentAttemptByTransactionID(ctx context.Context, transactionID string) (*domain.PaymentAttempt, error) {
	var txn transactionDBModel
	if err := r.db.WithContext(ctx).Select("current_attempt_id").First(&txn, "id = ?", transactionID).Error; err != nil {
		return nil, err
	}
	if txn.CurrentAttemptID == nil || *txn.CurrentAttemptID == "" {
		return nil, nil
	}

	var model paymentAttemptDBModel
	if err := r.db.WithContext(ctx).First(&model, "id = ?", *txn.CurrentAttemptID).Error; err != nil {
		return nil, err
	}
	return toDomainAttempt(&model), nil
}

func (r *transactionRepository) CountAttemptsByTransactionID(ctx context.Context, transactionID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&paymentAttemptDBModel{}).Where("transaction_id = ?", transactionID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *transactionRepository) HasWebhookEvent(ctx context.Context, provider, requestID string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&webhookEventDBModel{}).
		Where("provider = ? AND request_id = ?", provider, requestID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *transactionRepository) RecordWebhookEvent(ctx context.Context, provider, requestID string, receivedAt time.Time) (bool, error) {
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "provider"}, {Name: "request_id"}}, DoNothing: true}).
		Create(&webhookEventDBModel{
			Provider:   provider,
			RequestID:  requestID,
			ReceivedAt: receivedAt,
			CreatedAt:  time.Now(),
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *transactionRepository) DeleteWebhookEventsBefore(ctx context.Context, cutoff time.Time) error {
	return r.db.WithContext(ctx).
		Where("created_at < ?", cutoff).
		Delete(&webhookEventDBModel{}).
		Error
}

func (r *transactionRepository) ListTransactionsMissingPaymentEvent(ctx context.Context, cutoff time.Time, limit int) ([]*domain.Transaction, error) {
	if limit <= 0 {
		limit = 100
	}

	var models []transactionDBModel
	if err := r.db.WithContext(ctx).
		Where("status IN ?", []string{string(domain.TxSuccessful), string(domain.TxFailed)}).
		Where("updated_at < ?", cutoff).
		Where("COALESCE(metadata->>'payment_status_event_enqueued_for','') <> status").
		Order("updated_at ASC").
		Limit(limit).
		Find(&models).Error; err != nil {
		return nil, err
	}

	txns := make([]*domain.Transaction, 0, len(models))
	for i := range models {
		txns = append(txns, toDomainModel(&models[i]))
	}
	return txns, nil
}

func (r *transactionRepository) CountPendingTransactionsOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&transactionDBModel{}).
		Where("status = ?", string(domain.TxPending)).
		Where("updated_at < ?", cutoff).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *transactionRepository) CountTransactionsMissingPaymentEvent(ctx context.Context, cutoff time.Time) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&transactionDBModel{}).
		Where("status IN ?", []string{string(domain.TxSuccessful), string(domain.TxFailed)}).
		Where("updated_at < ?", cutoff).
		Where("COALESCE(metadata->>'payment_status_event_enqueued_for','') <> status").
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *transactionRepository) CountWebhookEventsSince(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&webhookEventDBModel{}).
		Where("created_at >= ?", since).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// WithTransaction executes fn within coordinated payment and core database
// transactions, matching the existing booking/quote outbox pattern.
func (r *transactionRepository) WithTransaction(ctx context.Context, fn func(domain.TransactionRepository) error) error {
	return r.coreDB.WithContext(ctx).Transaction(func(coreTx *gorm.DB) error {
		return core.WithTransaction(ctx, r.db, func(dbTx *gorm.DB) domain.TransactionRepository {
			return &transactionRepository{db: dbTx, coreDB: coreTx}
		}, fn)
	})
}
