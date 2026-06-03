package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	corerepo "encore.app/core/repository"
	"encore.app/quote/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// QuoteRepository implements domain.QuoteRepository using GORM
type QuoteRepository struct {
	db     *gorm.DB
	coreDB *gorm.DB
}

// NewQuoteRepository creates a new quote repository
func NewQuoteRepository(db *gorm.DB, coreDB *gorm.DB) domain.QuoteRepository {
	return &QuoteRepository{
		db:     db,
		coreDB: coreDB,
	}
}

// ============================================================================
// Basic CRUD Operations
// ============================================================================

// Create creates a new quote
func (r *QuoteRepository) Create(ctx context.Context, quote *domain.Quote) error {
	result := r.db.WithContext(ctx).Create(quote)
	if result.Error != nil {
		if isActiveProposedQuoteConstraintError(result.Error) {
			return domain.ErrActiveQuoteExists
		}
		return fmt.Errorf("failed to create quote: %w", result.Error)
	}
	return nil
}

// Update updates an existing quote with optimistic locking
func (r *QuoteRepository) Update(ctx context.Context, quote *domain.Quote) error {
	// Optimistic locking: update only if version matches
	result := r.db.WithContext(ctx).
		Model(&domain.Quote{}).
		Where("id = ? AND db_version = ?", quote.ID, quote.DBVersion).
		Updates(map[string]any{
			"state":                   quote.State,
			"amount_cents":            quote.AmountCents,
			"currency":                quote.Currency,
			"notes":                   quote.Notes,
			"estimated_duration_mins": quote.EstimatedDurationMins,
			"valid_until":             quote.ValidUntil,
			"decision_by":             quote.DecisionBy,
			"decided_at":              quote.DecidedAt,
			"rejection_reason_code":   quote.RejectionReasonCode,
			"rejection_reason_text":   quote.RejectionReasonText,
			"updated_at":              quote.UpdatedAt,
			"db_version":              gorm.Expr("db_version + 1"),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update quote: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return &domain.ErrOptimisticLockFailure{EntityID: quote.ID}
	}

	// Increment version in memory for consistency
	quote.DBVersion++

	return nil
}

// GetByID retrieves a quote by ID
func (r *QuoteRepository) GetByID(ctx context.Context, id string) (*domain.Quote, error) {
	var quote domain.Quote
	result := r.db.WithContext(ctx).Where("id = ?", id).First(&quote)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, domain.ErrQuoteNotFound
		}
		return nil, fmt.Errorf("failed to get quote: %w", result.Error)
	}

	return &quote, nil
}

// GetByBookingID retrieves all quotes for a booking
func (r *QuoteRepository) GetByBookingID(ctx context.Context, bookingID string) ([]*domain.Quote, error) {
	var quotes []*domain.Quote
	result := r.db.WithContext(ctx).
		Where("booking_id = ?", bookingID).
		Order("version DESC, created_at DESC").
		Find(&quotes)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get quotes by booking ID: %w", result.Error)
	}

	return quotes, nil
}

// GetActiveProposedByBookingID retrieves the pending proposed quote for a booking, if any.
func (r *QuoteRepository) GetActiveProposedByBookingID(ctx context.Context, bookingID string) (*domain.Quote, error) {
	var quote domain.Quote
	result := r.db.WithContext(ctx).
		Where("booking_id = ? AND state = ?", bookingID, domain.QuoteProposed).
		Order("version DESC, created_at DESC").
		First(&quote)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, domain.ErrQuoteNotFound
		}
		return nil, fmt.Errorf("failed to get active proposed quote by booking ID: %w", result.Error)
	}

	return &quote, nil
}

// GetLatestVersionByBookingID retrieves only the latest version number for a booking
func (r *QuoteRepository) GetLatestVersionByBookingID(ctx context.Context, bookingID string) (int, error) {
	var maxVersion int
	result := r.db.WithContext(ctx).
		Model(&domain.Quote{}).
		Where("booking_id = ?", bookingID).
		Select("COALESCE(MAX(version), 0)").
		Scan(&maxVersion)

	if result.Error != nil {
		return 0, fmt.Errorf("failed to get latest version by booking ID: %w", result.Error)
	}

	return maxVersion, nil
}

// GetByIDForUpdate retrieves a quote with row-level lock (FOR UPDATE)
func (r *QuoteRepository) GetByIDForUpdate(ctx context.Context, id string) (*domain.Quote, error) {
	var quote domain.Quote
	result := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", id).
		First(&quote)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, domain.ErrQuoteNotFound
		}
		return nil, fmt.Errorf("failed to get quote for update: %w", result.Error)
	}

	return &quote, nil
}

// ============================================================================
// Query Operations
// ============================================================================

// FindExpiredQuotes finds all quotes that have expired but are still in proposed state
func (r *QuoteRepository) FindExpiredQuotes(ctx context.Context) ([]*domain.Quote, error) {
	var quotes []*domain.Quote

	result := r.db.WithContext(ctx).
		Where("state = ? AND valid_until < ?", domain.QuoteProposed, time.Now()).
		Find(&quotes)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to find expired quotes: %w", result.Error)
	}

	return quotes, nil
}

// ============================================================================
// Transaction Support
// ============================================================================

// WithTransaction executes a function within a database transaction
func (r *QuoteRepository) WithTransaction(ctx context.Context, fn func(txRepo domain.QuoteRepository) error) error {
	// COORDINATED TRANSACTIONS: Since Outbox and Quote data live in different databases,
	// we nest their transactions. This ensures that if the business logic or the quote
	// commit fails, the outbox event is also rolled back.
	return r.coreDB.WithContext(ctx).Transaction(func(coreTx *gorm.DB) error {
		// Check if we're already in a transaction on the quote DB
		if tx := r.db.WithContext(ctx); tx.Statement.DB != nil {
			txRepo := &QuoteRepository{
				db:     tx,
				coreDB: coreTx,
			}
			return fn(txRepo)
		}

		// Start new transaction on quote DB
		return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			txRepo := &QuoteRepository{
				db:     tx,
				coreDB: coreTx,
			}
			return fn(txRepo)
		})
	})
}

// ============================================================================
// Outbox Operations
// ============================================================================

// CreateEventInOutbox creates an event in the outbox table
func (r *QuoteRepository) CreateEventInOutbox(ctx context.Context, event *domain.QuoteEvent) error { // Serialize event data
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Determine topic based on event state
	topic := getTopicForState(event.State)

	// Create outbox entry
	outboxEvent := &corerepo.OutboxEvent{
		Topic:      topic,
		Data:       eventData,
		InsertedAt: time.Now(),
	}

	result := r.coreDB.WithContext(ctx).Create(outboxEvent)
	if result.Error != nil {
		return fmt.Errorf("failed to create outbox event: %w", result.Error)
	}

	return nil
}

// ============================================================================
// Helper Functions
// ============================================================================

// getTopicForState returns the appropriate topic name for a quote state
func getTopicForState(state string) string {
	switch state {
	case "proposed":
		return "quote-v1-proposed"
	case "accepted":
		return "quote-v1-accepted"
	case "rejected":
		return "quote-v1-rejected"
	case "expired":
		return "quote-v1-expired"
	default:
		return "quote-v1-state-changed"
	}
}

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate key") || strings.Contains(message, "unique constraint")
}

func isActiveProposedQuoteConstraintError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	if strings.Contains(errStr, "idx_quotes_one_active_proposed_per_booking") {
		return true
	}
	for err != nil {
		err = errors.Unwrap(err)
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "idx_quotes_one_active_proposed_per_booking") {
			return true
		}
	}
	return false
}
