package relay

import (
	"context"

	eventscommon "encore.app/core/events"
	topics_payment "encore.app/core/events/topics/payment"
	"encore.app/core/repository"
	pevents "encore.app/payment/events"
)

// PaymentEvent is the outbox payload emitted by the payment service for booking updates.
type PaymentEvent struct {
	pevents.EventEnvelope[eventscommon.PaymentEvent]
}

// EventType satisfies the core relay EventData interface.
func (e PaymentEvent) EventType() string {
	return e.EventEnvelope.EventType
}

// PaymentPublisher routes payment outbox events to the matching PubSub topic.
type PaymentPublisher struct{}

func (p *PaymentPublisher) PublishToTopic(ctx context.Context, outboxEvent *repository.OutboxEvent, event PaymentEvent) error {
	envelope := &event.EventEnvelope

	switch outboxEvent.Topic {
	case "payment-v1-confirmed":
		_, err := topics_payment.PaymentConfirmedTopic.Publish(ctx, envelope)
		return err
	case "payment-v1-failed":
		_, err := topics_payment.PaymentFailedTopic.Publish(ctx, envelope)
		return err
	default:
		return nil
	}
}
