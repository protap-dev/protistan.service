package events

import (
	"context"
	"log"

	topics_payment "encore.app/core/events/topics/payment"
	"encore.app/payment/domain"
)

type eventPublisher struct{}

// NewEventPublisher creates a new payment event publisher.
func NewEventPublisher() domain.EventPublisher {
	return &eventPublisher{}
}

func (e *eventPublisher) PublishPaymentConfirmedEvent(ctx context.Context, event *domain.PaymentEvent) {
	envelope := CreateEventEnvelope(ctx, "payment.v1.confirmed", *event)
	_, err := topics_payment.PaymentConfirmedTopic.Publish(ctx, envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish payment confirmed event: %v", err)
	}
}

func (e *eventPublisher) PublishPaymentFailedEvent(ctx context.Context, event *domain.PaymentEvent) {
	envelope := CreateEventEnvelope(ctx, "payment.v1.failed", *event)
	_, err := topics_payment.PaymentFailedTopic.Publish(ctx, envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish payment failed event: %v", err)
	}
}
