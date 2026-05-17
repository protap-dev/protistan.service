package events

import (
	"context"
	"sync"

	"encore.app/payment/domain"
)

// MockEventPublisher records payment events for tests.
type MockEventPublisher struct {
	mu                     sync.RWMutex
	PaymentConfirmedEvents []domain.PaymentEvent
	PaymentFailedEvents    []domain.PaymentEvent
}

func NewMockEventPublisher() *MockEventPublisher {
	return &MockEventPublisher{
		PaymentConfirmedEvents: make([]domain.PaymentEvent, 0),
		PaymentFailedEvents:    make([]domain.PaymentEvent, 0),
	}
}

func (m *MockEventPublisher) PublishPaymentConfirmedEvent(ctx context.Context, event *domain.PaymentEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PaymentConfirmedEvents = append(m.PaymentConfirmedEvents, *event)
}

func (m *MockEventPublisher) PublishPaymentFailedEvent(ctx context.Context, event *domain.PaymentEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PaymentFailedEvents = append(m.PaymentFailedEvents, *event)
}

func (m *MockEventPublisher) GetPaymentConfirmedEvents() []domain.PaymentEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]domain.PaymentEvent{}, m.PaymentConfirmedEvents...)
}

func (m *MockEventPublisher) GetPaymentFailedEvents() []domain.PaymentEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]domain.PaymentEvent{}, m.PaymentFailedEvents...)
}

func (m *MockEventPublisher) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PaymentConfirmedEvents = make([]domain.PaymentEvent, 0)
	m.PaymentFailedEvents = make([]domain.PaymentEvent, 0)
}
