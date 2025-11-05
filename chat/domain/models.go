package domain

import (
	"time"

	"gorm.io/datatypes"
)

// Thread represents a chat thread between customer and artisan
type Thread struct {
	// Identity
	ID        string `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	BookingID string `json:"booking_id" gorm:"type:uuid;not null;uniqueIndex;index"`

	// Participants
	CustomerID string `json:"customer_id" gorm:"type:uuid;not null;index"`
	ArtisanID  string `json:"artisan_id" gorm:"type:uuid;not null;index"`

	// Metadata
	LastMessageAt *time.Time     `json:"last_message_at,omitempty"`
	LastMessageID *string        `json:"last_message_id,omitempty" gorm:"type:uuid"`
	Metadata      datatypes.JSON `json:"metadata,omitempty" gorm:"type:jsonb"`

	// Timestamps
	CreatedAt time.Time `json:"created_at" gorm:"not null"`
	UpdatedAt time.Time `json:"updated_at" gorm:"not null"`
}

// Message represents a single message in a thread
type Message struct {
	// Identity
	ID             string `json:"id" gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	ThreadID       string `json:"thread_id" gorm:"type:uuid;not null;index"`
	IdempotencyKey string `json:"idempotency_key" gorm:"type:varchar(255);not null;uniqueIndex"` // For idempotent message writes

	// Content
	SenderID    string         `json:"sender_id" gorm:"type:uuid;not null;index"`
	Content     string         `json:"content" gorm:"type:text;not null"`
	MessageType MessageType    `json:"message_type" gorm:"type:text;not null;default:'user'"`
	Metadata    datatypes.JSON `json:"metadata,omitempty" gorm:"type:jsonb"`

	// Status tracking
	Status      MessageStatus `json:"status" gorm:"type:text;not null;default:'sending'"`
	SentAt      *time.Time    `json:"sent_at,omitempty"`
	DeliveredAt *time.Time    `json:"delivered_at,omitempty"`
	ReadAt      *time.Time    `json:"read_at,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at" gorm:"not null"`
	UpdatedAt time.Time `json:"updated_at" gorm:"not null"`

	// Optimistic locking
	DBVersion int64 `json:"db_version" gorm:"column:db_version;not null;default:1"`
}

// CanTransitionTo checks if message can transition to target status
func (m *Message) CanTransitionTo(target MessageStatus) bool {
	return CanTransition(m.Status, target)
}

// IsAutomated checks if message is system-generated
func (m *Message) IsAutomated() bool {
	return m.MessageType == MessageTypeSystem || m.MessageType == MessageTypeStatusUpdate
}
