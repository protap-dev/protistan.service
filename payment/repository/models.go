package repository

import (
	"encoding/json"
	"time"

	"encore.app/payment/domain"
	"gorm.io/datatypes"
)

type transactionDBModel struct {
	ID               string         `gorm:"column:id;primaryKey;default:generate_uuid()"`
	CustomerID       string         `gorm:"column:customer_id;not null"`
	BookingID        string         `gorm:"column:booking_id;not null"`
	QuoteID          string         `gorm:"column:quote_id"`
	Amount           float64        `gorm:"column:amount;not null;type:decimal(15,2)"`
	Currency         string         `gorm:"column:currency;not null"`
	Provider         string         `gorm:"column:provider;not null"`
	Method           string         `gorm:"column:method;not null"`
	Status           string         `gorm:"column:status;not null"`
	InternalRef      string         `gorm:"column:internal_ref;not null;unique"`
	ProviderRef      string         `gorm:"column:provider_ref"`
	CurrentAttemptID *string        `gorm:"column:current_attempt_id"`
	Metadata         datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt        time.Time      `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt        time.Time      `gorm:"column:updated_at;not null;default:now()"`
}

func (transactionDBModel) TableName() string { return "transactions" }

type paymentAttemptDBModel struct {
	ID                     string     `gorm:"column:id;primaryKey;default:generate_uuid()"`
	TransactionID          string     `gorm:"column:transaction_id;not null"`
	AttemptNo              int        `gorm:"column:attempt_no;not null"`
	Provider               string     `gorm:"column:provider;not null"`
	Status                 string     `gorm:"column:status;not null"`
	InternalRef            string     `gorm:"column:internal_ref;not null;unique"`
	ProviderRef            string     `gorm:"column:provider_ref"`
	CheckoutURL            string     `gorm:"column:checkout_url"`
	ReturnURL              string     `gorm:"column:return_url"`
	ReturnContext          string     `gorm:"column:return_context"`
	ReturnContextExpiresAt *time.Time `gorm:"column:return_context_expires_at"`
	CancelRequestedAt      *time.Time `gorm:"column:cancel_requested_at"`
	CancelledAt            *time.Time `gorm:"column:cancelled_at"`
	SupersededByAttemptID  *string    `gorm:"column:superseded_by_attempt_id"`
	CreatedByUserID        string     `gorm:"column:created_by_user_id;not null"`
	CreatedAt              time.Time  `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt              time.Time  `gorm:"column:updated_at;not null;default:now()"`
}

func (paymentAttemptDBModel) TableName() string { return "payment_attempts" }

type webhookEventDBModel struct {
	ID         string    `gorm:"column:id;primaryKey;default:generate_uuid()"`
	Provider   string    `gorm:"column:provider;not null"`
	RequestID  string    `gorm:"column:request_id;not null"`
	ReceivedAt time.Time `gorm:"column:received_at;not null"`
	CreatedAt  time.Time `gorm:"column:created_at;not null;default:now()"`
}

func (webhookEventDBModel) TableName() string { return "webhook_events" }

func toDBModel(txn *domain.Transaction) *transactionDBModel {
	meta, err := json.Marshal(txn.Metadata)
	if err != nil {
		meta = []byte("{}")
	}
	return &transactionDBModel{
		ID:               txn.ID,
		CustomerID:       txn.CustomerID,
		BookingID:        txn.BookingID,
		QuoteID:          txn.QuoteID,
		Amount:           txn.Amount,
		Currency:         txn.Currency,
		Provider:         txn.Provider,
		Method:           txn.Method,
		Status:           string(txn.Status),
		InternalRef:      txn.InternalRef,
		ProviderRef:      txn.ProviderRef,
		CurrentAttemptID: txn.CurrentAttemptID,
		Metadata:         datatypes.JSON(meta),
		CreatedAt:        txn.CreatedAt,
		UpdatedAt:        txn.UpdatedAt,
	}
}

func toDomainModel(m *transactionDBModel) *domain.Transaction {
	metadata := make(map[string]string)
	var raw map[string]any
	if err := json.Unmarshal(m.Metadata, &raw); err == nil {
		for k, v := range raw {
			if s, ok := v.(string); ok {
				metadata[k] = s
			}
		}
	}
	return &domain.Transaction{
		ID:               m.ID,
		CustomerID:       m.CustomerID,
		BookingID:        m.BookingID,
		QuoteID:          m.QuoteID,
		Amount:           m.Amount,
		Currency:         m.Currency,
		Provider:         m.Provider,
		Method:           m.Method,
		Status:           domain.TransactionStatus(m.Status),
		InternalRef:      m.InternalRef,
		ProviderRef:      m.ProviderRef,
		CurrentAttemptID: m.CurrentAttemptID,
		Metadata:         metadata,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
	}
}

func toAttemptDBModel(attempt *domain.PaymentAttempt) *paymentAttemptDBModel {
	return &paymentAttemptDBModel{
		ID:                     attempt.ID,
		TransactionID:          attempt.TransactionID,
		AttemptNo:              attempt.AttemptNo,
		Provider:               attempt.Provider,
		Status:                 string(attempt.Status),
		InternalRef:            attempt.InternalRef,
		ProviderRef:            attempt.ProviderRef,
		CheckoutURL:            attempt.CheckoutURL,
		ReturnURL:              attempt.ReturnURL,
		ReturnContext:          attempt.ReturnContext,
		ReturnContextExpiresAt: attempt.ReturnContextExpiresAt,
		CancelRequestedAt:      attempt.CancelRequestedAt,
		CancelledAt:            attempt.CancelledAt,
		SupersededByAttemptID:  attempt.SupersededByAttemptID,
		CreatedByUserID:        attempt.CreatedByUserID,
		CreatedAt:              attempt.CreatedAt,
		UpdatedAt:              attempt.UpdatedAt,
	}
}

func toDomainAttempt(m *paymentAttemptDBModel) *domain.PaymentAttempt {
	return &domain.PaymentAttempt{
		ID:                     m.ID,
		TransactionID:          m.TransactionID,
		AttemptNo:              m.AttemptNo,
		Provider:               m.Provider,
		Status:                 domain.PaymentAttemptStatus(m.Status),
		InternalRef:            m.InternalRef,
		ProviderRef:            m.ProviderRef,
		CheckoutURL:            m.CheckoutURL,
		ReturnURL:              m.ReturnURL,
		ReturnContext:          m.ReturnContext,
		ReturnContextExpiresAt: m.ReturnContextExpiresAt,
		CancelRequestedAt:      m.CancelRequestedAt,
		CancelledAt:            m.CancelledAt,
		SupersededByAttemptID:  m.SupersededByAttemptID,
		CreatedByUserID:        m.CreatedByUserID,
		CreatedAt:              m.CreatedAt,
		UpdatedAt:              m.UpdatedAt,
	}
}
