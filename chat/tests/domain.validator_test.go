package tests

import (
	"context"
	"testing"

	"encore.app/chat/domain"
	"github.com/stretchr/testify/assert"
)

func TestValidator_ValidateSendMessage(t *testing.T) {
	validator := domain.NewChatValidator()
	ctx := context.Background()

	tests := []struct {
		name      string
		input     *domain.SendMessageInput
		wantError bool
		errorCode string
	}{
		{
			name: "valid message",
			input: &domain.SendMessageInput{
				ThreadID:       "thread-123",
				SenderID:       "user-123",
				Content:        "Hello",
				MessageType:    domain.MessageTypeUser,
				IdempotencyKey: "key-123",
			},
			wantError: false,
		},
		{
			name: "empty content",
			input: &domain.SendMessageInput{
				ThreadID:       "thread-123",
				SenderID:       "user-123",
				Content:        "   ",
				MessageType:    domain.MessageTypeUser,
				IdempotencyKey: "key-123",
			},
			wantError: true,
			errorCode: "empty_content",
		},
		{
			name: "missing idempotency key",
			input: &domain.SendMessageInput{
				ThreadID:    "thread-123",
				SenderID:    "user-123",
				Content:     "Hello",
				MessageType: domain.MessageTypeUser,
			},
			wantError: true,
			errorCode: "invalid_input",
		},
		{
			name: "invalid message type",
			input: &domain.SendMessageInput{
				ThreadID:       "thread-123",
				SenderID:       "user-123",
				Content:        "Hello",
				MessageType:    "custom",
				IdempotencyKey: "key-123",
			},
			wantError: true,
			errorCode: "invalid_input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateSendMessage(ctx, tt.input)
			if tt.wantError {
				assert.Error(t, err)
				if chatErr, ok := err.(*domain.ChatError); ok {
					assert.Equal(t, tt.errorCode, chatErr.Code)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
