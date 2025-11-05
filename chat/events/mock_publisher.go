package events

import (
	"context"
	"sync"

	"encore.app/chat/domain"
)

// MockEventPublisher is a mock implementation for testing
type MockEventPublisher struct {
	mu                     sync.RWMutex
	MessageSentEvents      []domain.ChatEvent
	MessageDeliveredEvents []domain.ChatEvent
	ThreadCreatedEvents    []domain.ChatEvent
}

// NewMockEventPublisher creates a new mock publisher
func NewMockEventPublisher() *MockEventPublisher {
	return &MockEventPublisher{
		MessageSentEvents:      make([]domain.ChatEvent, 0),
		MessageDeliveredEvents: make([]domain.ChatEvent, 0),
		ThreadCreatedEvents:    make([]domain.ChatEvent, 0),
	}
}

func (m *MockEventPublisher) PublishMessageSentEvent(ctx context.Context, event *domain.ChatEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.MessageSentEvents = append(m.MessageSentEvents, *event)
}

func (m *MockEventPublisher) PublishMessageDeliveredEvent(ctx context.Context, event *domain.ChatEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.MessageDeliveredEvents = append(m.MessageDeliveredEvents, *event)
}

func (m *MockEventPublisher) PublishThreadCreatedEvent(ctx context.Context, event *domain.ChatEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ThreadCreatedEvents = append(m.ThreadCreatedEvents, *event)
}

// GetMessageSentEvents returns all published message sent events
func (m *MockEventPublisher) GetMessageSentEvents() []domain.ChatEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]domain.ChatEvent{}, m.MessageSentEvents...)
}

// GetThreadCreatedEvents returns all published thread created events
func (m *MockEventPublisher) GetThreadCreatedEvents() []domain.ChatEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]domain.ChatEvent{}, m.ThreadCreatedEvents...)
}

// Reset clears all recorded events
func (m *MockEventPublisher) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.MessageSentEvents = make([]domain.ChatEvent, 0)
	m.MessageDeliveredEvents = make([]domain.ChatEvent, 0)
	m.ThreadCreatedEvents = make([]domain.ChatEvent, 0)
}
