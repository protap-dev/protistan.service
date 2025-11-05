package tests

import (
	"testing"

	"encore.app/chat/domain"
	"github.com/stretchr/testify/assert"
)

func TestMessage_CanTransitionTo(t *testing.T) {
	tests := []struct {
		name     string
		current  domain.MessageStatus
		target   domain.MessageStatus
		expected bool
	}{
		{"sending to sent", domain.MessageSending, domain.MessageSent, true},
		{"sending to delivered", domain.MessageSending, domain.MessageDelivered, false},
		{"sent to delivered", domain.MessageSent, domain.MessageDelivered, true},
		{"delivered to read", domain.MessageDelivered, domain.MessageRead, true},
		{"read to delivered", domain.MessageRead, domain.MessageDelivered, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &domain.Message{Status: tt.current}
			result := msg.CanTransitionTo(tt.target)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMessage_IsAutomated(t *testing.T) {
	tests := []struct {
		name        string
		messageType domain.MessageType
		expected    bool
	}{
		{"user message", domain.MessageTypeUser, false},
		{"system message", domain.MessageTypeSystem, true},
		{"status update", domain.MessageTypeStatusUpdate, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &domain.Message{MessageType: tt.messageType}
			assert.Equal(t, tt.expected, msg.IsAutomated())
		})
	}
}

func TestMessageStatusHelpers(t *testing.T) {
	assert.True(t, domain.MessageDelivered.IsDeliveredOrRead())
	assert.True(t, domain.MessageRead.IsDeliveredOrRead())
	assert.False(t, domain.MessageSent.IsDeliveredOrRead())

	assert.True(t, domain.MessageSending.IsPendingDelivery())
	assert.True(t, domain.MessageSent.IsPendingDelivery())
	assert.False(t, domain.MessageDelivered.IsPendingDelivery())

	assert.True(t, domain.MessageFailed.IsFailure())
	assert.False(t, domain.MessageSent.IsFailure())
}

func TestMessageTypeIsValid(t *testing.T) {
	tests := []struct {
		msgType  domain.MessageType
		expected bool
	}{
		{domain.MessageTypeUser, true},
		{domain.MessageTypeSystem, true},
		{domain.MessageTypeStatusUpdate, true},
		{"", false},
		{"custom", false},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.msgType.IsValid())
	}
}
