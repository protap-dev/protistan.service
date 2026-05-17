package domain

import (
	"context"

	eventscommon "encore.app/core/events"
)

// PaymentEvent is the payment service domain alias for the shared event contract.
type PaymentEvent = eventscommon.PaymentEvent

// EventPublisher defines the interface for publishing payment-related events.
type EventPublisher interface {
	PublishPaymentConfirmedEvent(ctx context.Context, event *PaymentEvent)
	PublishPaymentFailedEvent(ctx context.Context, event *PaymentEvent)
}
