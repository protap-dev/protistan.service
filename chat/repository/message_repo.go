package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"encore.app/chat/domain"
	corerepo "encore.app/core/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MessageRepositoryImpl implements domain.MessageRepository
type MessageRepositoryImpl struct {
	db     *gorm.DB
	coreDB *gorm.DB
}

// NewMessageRepository creates a new message repository
func NewMessageRepository(db *gorm.DB, coreDB *gorm.DB) domain.MessageRepository {
	return &MessageRepositoryImpl{
		db:     db,
		coreDB: coreDB,
	}
}

// Create creates a new message with idempotency check
func (r *MessageRepositoryImpl) Create(ctx context.Context, message *domain.Message) error {
	result := r.db.WithContext(ctx).Create(message)
	if result.Error != nil {
		// Check for duplicate idempotency key
		if isDuplicateKeyError(result.Error) {
			return domain.ErrDuplicateMessage
		}
		return fmt.Errorf("failed to create message: %w", result.Error)
	}
	return nil
}

// GetByID retrieves a message by ID
func (r *MessageRepositoryImpl) GetByID(ctx context.Context, id string) (*domain.Message, error) {
	var message domain.Message
	result := r.db.WithContext(ctx).Where("id = ?", id).First(&message)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, domain.ErrMessageNotFound
		}
		return nil, fmt.Errorf("failed to get message: %w", result.Error)
	}
	return &message, nil
}

// GetByIDForUpdate retrieves a message with row-level lock (FOR UPDATE)
func (r *MessageRepositoryImpl) GetByIDForUpdate(ctx context.Context, id string) (*domain.Message, error) {
	var message domain.Message
	result := r.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", id).
		First(&message)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, domain.ErrMessageNotFound
		}
		return nil, fmt.Errorf("failed to get message for update: %w", result.Error)
	}
	return &message, nil
}

// GetByIdempotencyKey retrieves a message by idempotency key
func (r *MessageRepositoryImpl) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Message, error) {
	var message domain.Message
	result := r.db.WithContext(ctx).Where("idempotency_key = ?", key).First(&message)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil // Not an error, just not found
		}
		return nil, fmt.Errorf("failed to get message by idempotency key: %w", result.Error)
	}
	return &message, nil
}

// GetByThreadID retrieves messages for a thread with pagination
func (r *MessageRepositoryImpl) GetByThreadID(ctx context.Context, threadID string, limit, offset int) ([]*domain.Message, error) {
	var messages []*domain.Message
	result := r.db.WithContext(ctx).
		Where("thread_id = ?", threadID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&messages)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get messages by thread: %w", result.Error)
	}
	return messages, nil
}

// Update updates a message with optimistic locking
func (r *MessageRepositoryImpl) Update(ctx context.Context, message *domain.Message) error {
	result := r.db.WithContext(ctx).
		Model(&domain.Message{}).
		Where("id = ? AND db_version = ?", message.ID, message.DBVersion).
		Updates(map[string]any{
			"status":       message.Status,
			"sent_at":      message.SentAt,
			"delivered_at": message.DeliveredAt,
			"read_at":      message.ReadAt,
			"updated_at":   message.UpdatedAt,
			"db_version":   gorm.Expr("db_version + 1"),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update message: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return &domain.ErrOptimisticLockFailure{EntityID: message.ID}
	}

	message.DBVersion++
	return nil
}

// MarkThreadAsRead marks all unread messages in a thread as read
func (r *MessageRepositoryImpl) MarkThreadAsRead(ctx context.Context, threadID string, userID string) error {
	now := time.Now()
	result := r.db.WithContext(ctx).
		Model(&domain.Message{}).
		Where("thread_id = ? AND sender_id != ? AND status != ?", threadID, userID, domain.MessageRead).
		Updates(map[string]any{
			"status":     domain.MessageRead,
			"read_at":    now,
			"updated_at": now,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to mark thread as read: %w", result.Error)
	}
	return nil
}

// WithTransaction executes a function within a database transaction
func (r *MessageRepositoryImpl) WithTransaction(ctx context.Context, fn func(txRepo domain.MessageRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := &MessageRepositoryImpl{
			db:     tx,
			coreDB: r.coreDB,
		}
		return fn(txRepo)
	})
}

// CreateEventInOutbox creates an event in the outbox table
func (r *MessageRepositoryImpl) CreateEventInOutbox(ctx context.Context, event *domain.ChatEvent) error {
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	topic := getTopicForEventType(event.EventType)

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

// Helper function to map event types to topics
func getTopicForEventType(eventType string) string {
	switch eventType {
	case "message.sent":
		return "chat-v1-message-sent"
	case "message.delivered":
		return "chat-v1-message-delivered"
	case "thread.created":
		return "chat-v1-thread-created"
	default:
		return "chat-v1-event"
	}
}
