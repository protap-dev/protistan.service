package domain

import (
	"context"
	"strings"
)

// Validator validates chat business rules
type Validator struct{}

// NewChatValidator creates a new validator
func NewChatValidator() *Validator {
	return &Validator{}
}

// ValidateCreateThread validates thread creation
func (v *Validator) ValidateCreateThread(ctx context.Context, input *CreateThreadInput) error {
	if input.BookingID == "" {
		return &ChatError{Code: "invalid_input", Message: "booking_id is required"}
	}
	if input.CustomerID == "" {
		return &ChatError{Code: "invalid_input", Message: "customer_id is required"}
	}
	if input.ArtisanID == "" {
		return &ChatError{Code: "invalid_input", Message: "artisan_id is required"}
	}
	return nil
}

// ValidateSendMessage validates message sending
func (v *Validator) ValidateSendMessage(ctx context.Context, input *SendMessageInput) error {
	if input.ThreadID == "" {
		return &ChatError{Code: "invalid_input", Message: "thread_id is required"}
	}
	if input.SenderID == "" {
		return &ChatError{Code: "invalid_input", Message: "sender_id is required"}
	}
	if err := v.ValidateMessageType(input.MessageType); err != nil {
		return err
	}
	if strings.TrimSpace(input.Content) == "" {
		return ErrEmptyContent
	}
	if input.IdempotencyKey == "" {
		return &ChatError{Code: "invalid_input", Message: "idempotency_key is required"}
	}
	return nil
}

// ValidateMessageType ensures provided message type is supported
func (v *Validator) ValidateMessageType(messageType MessageType) error {
	if !messageType.IsValid() {
		return &ChatError{Code: "invalid_input", Message: "message_type is invalid"}
	}
	return nil
}

// ValidateParticipant ensures sender is part of the thread
func (v *Validator) ValidateParticipant(thread *Thread, senderID string) error {
	if thread.CustomerID != senderID && thread.ArtisanID != senderID {
		return ErrInvalidParticipant
	}
	return nil
}

// Input types
type CreateThreadInput struct {
	BookingID     string
	CustomerID    string
	ArtisanID     string
	ArtisanUserID string
}

type SendMessageInput struct {
	ThreadID       string
	SenderID       string
	Content        string
	MessageType    MessageType
	IdempotencyKey string
}
