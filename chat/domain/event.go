package domain

import (
	"context"
	"time"
)

type ChatEvent struct {
	MessageID string `json:"message_id,omitempty"`
	ThreadID  string `json:"thread_id"`
	BookingID string `json:"booking_id"`

	EventType string `json:"event_type"` // e.g., "message.sent", "message.delivered", "thread.created"

	SenderID   string `json:"sender_id,omitempty"`
	ReceiverID string `json:"receiver_id,omitempty"`
	CustomerID string `json:"customer_id"`
	ArtisanID  string `json:"artisan_id"`

	Content        *string `json:"content,omitempty"`
	MessageType    string  `json:"message_type,omitempty"`   // "user", "system", "status_update"
	MessageStatus  string  `json:"message_status,omitempty"` // "sending", "sent", "delivered", "read"
	PreviousStatus *string `json:"previous_status,omitempty"`

	IdempotencyKey string `json:"idempotency_key,omitempty"`

	Timestamp   time.Time  `json:"timestamp"`
	SentAt      *time.Time `json:"sent_at,omitempty"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
	ReadAt      *time.Time `json:"read_at,omitempty"`

	UserID   string            `json:"user_id"` // The user who triggered this event
	Metadata map[string]string `json:"metadata,omitempty"`
}

// EventPublisher defines the interface for publishing chat events
type EventPublisher interface {
	PublishMessageSentEvent(ctx context.Context, event *ChatEvent)
	PublishMessageDeliveredEvent(ctx context.Context, event *ChatEvent)
	PublishThreadCreatedEvent(ctx context.Context, event *ChatEvent)
}
