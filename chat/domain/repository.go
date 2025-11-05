package domain

import "context"

// ThreadRepository defines thread repository operations
type ThreadRepository interface {
	// Thread CRUD
	Create(ctx context.Context, thread *Thread) error
	GetByID(ctx context.Context, id string) (*Thread, error)
	GetByBookingID(ctx context.Context, bookingID string) (*Thread, error)
	GetByParticipant(ctx context.Context, userID string, limit, offset int) ([]*Thread, error)
	Update(ctx context.Context, thread *Thread) error

	// Transaction support
	WithTransaction(ctx context.Context, fn func(txRepo ThreadRepository) error) error
}

// MessageRepository defines message repository operations
type MessageRepository interface {
	// Message CRUD with idempotency
	Create(ctx context.Context, message *Message) error
	GetByID(ctx context.Context, id string) (*Message, error)
	GetByIDForUpdate(ctx context.Context, id string) (*Message, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*Message, error)
	GetByThreadID(ctx context.Context, threadID string, limit, offset int) ([]*Message, error)
	Update(ctx context.Context, message *Message) error

	// Bulk operations
	MarkThreadAsRead(ctx context.Context, threadID string, userID string) error

	// Transaction support
	WithTransaction(ctx context.Context, fn func(txRepo MessageRepository) error) error

	// Outbox
	CreateEventInOutbox(ctx context.Context, event *ChatEvent) error
}
