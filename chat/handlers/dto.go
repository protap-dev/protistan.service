package handlers

import (
	"encoding/json"
	"sync"
	"time"

	"encore.app/chat/domain"
)

// Request DTOs
type SendMessageRequest struct {
	Content        string `json:"content" validate:"required,max=5000"`
	IdempotencyKey string `json:"idempotency_key" validate:"required"`
}

type MarkThreadAsReadRequest struct {
	ThreadID string `json:"thread_id" validate:"required"`
}

type ListThreadsRequest struct {
	Limit  int `json:"limit" validate:"min=1,max=100"`
	Offset int `json:"offset" validate:"min=0"`
}

type ListMessagesRequest struct {
	ThreadID string `json:"thread_id" validate:"required"`
	Limit    int    `json:"limit" validate:"min=1,max=100"`
	Offset   int    `json:"offset" validate:"min=0"`
}

// Response DTOs
type MessageResponse struct {
	ID             string          `json:"id"`
	ThreadID       string          `json:"thread_id"`
	SenderID       string          `json:"sender_id"`
	Content        string          `json:"content"`
	MessageType    string          `json:"message_type"`
	Status         string          `json:"status"`
	IdempotencyKey string          `json:"idempotency_key"`
	SentAt         *time.Time      `json:"sent_at,omitempty"`
	DeliveredAt    *time.Time      `json:"delivered_at,omitempty"`
	ReadAt         *time.Time      `json:"read_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
}

type ThreadResponse struct {
	ID            string     `json:"id"`
	BookingID     string     `json:"booking_id"`
	CustomerID    string     `json:"customer_id"`
	ArtisanID     string     `json:"artisan_id"`
	LastMessageAt *time.Time `json:"last_message_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type ThreadsResponse struct {
	Threads []*ThreadResponse `json:"threads"`
	Total   int               `json:"total"`
}

type MessagesResponse struct {
	Messages []*MessageResponse `json:"messages"`
	Total    int                `json:"total"`
}

// WSMessage represents a message sent over WebSocket
type WSMessage struct {
	ID             string          `json:"id"`
	ThreadID       string          `json:"thread_id"`
	SenderID       string          `json:"sender_id"`
	Content        string          `json:"content"`
	MessageType    string          `json:"message_type"`
	Status         string          `json:"status"`
	IdempotencyKey string          `json:"idempotency_key"`
	SentAt         time.Time       `json:"sent_at"`
	DeliveredAt    *time.Time      `json:"delivered_at,omitempty"`
	ReadAt         *time.Time      `json:"read_at,omitempty"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	Type           string          `json:"type,omitempty"`
}

// WebSocketManager manages active WebSocket connections per thread.
// It supports multiple connections per user by storing a set of clients for each thread.
type WebSocketManager struct {
	mu        sync.RWMutex
	clients   map[string]map[*WSClient]bool // threadID -> set of clients
	broadcast chan *WSMessage
}

// WSClient represents a connected WebSocket client
type WSClient struct {
	threadID string
	userID   string
	send     chan *WSMessage
	close    chan struct{}
}

type WSMessagePayload struct {
	Content        string `json:"content"`
	IdempotencyKey string `json:"idempotency_key"`
}

type WSStatusUpdate struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"` // sent, delivered, read, failed
	Timestamp int64  `json:"timestamp"`
}

// Mappers
func MessageToResponse(msg *domain.Message) *MessageResponse {
	return &MessageResponse{
		ID:             msg.ID,
		ThreadID:       msg.ThreadID,
		SenderID:       msg.SenderID,
		Content:        msg.Content,
		MessageType:    string(msg.MessageType),
		Status:         string(msg.Status),
		IdempotencyKey: msg.IdempotencyKey,
		SentAt:         msg.SentAt,
		DeliveredAt:    msg.DeliveredAt,
		ReadAt:         msg.ReadAt,
		CreatedAt:      msg.CreatedAt,
		UpdatedAt:      msg.UpdatedAt,
		Metadata:       json.RawMessage(msg.Metadata),
	}
}

func ThreadToResponse(thread *domain.Thread) *ThreadResponse {
	return &ThreadResponse{
		ID:            thread.ID,
		BookingID:     thread.BookingID,
		CustomerID:    thread.CustomerID,
		ArtisanID:     thread.ArtisanID,
		LastMessageAt: thread.LastMessageAt,
		CreatedAt:     thread.CreatedAt,
		UpdatedAt:     thread.UpdatedAt,
	}
}
