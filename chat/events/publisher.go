package events

import (
	"context"
	"log"

	"encore.app/chat/domain"
	topics_chat "encore.app/core/events/topics/chat"
)

// chatEventPublisher implements the domain.EventPublisher interface.
// It publishes events wrapped in envelopes for observability and tracing.
type chatEventPublisher struct{}

// NewEventPublisher creates a new event publisher.
func NewEventPublisher() domain.EventPublisher {
	return &chatEventPublisher{}
}

// PublishMessageSentEvent publishes when a message is sent.
func (e *chatEventPublisher) PublishMessageSentEvent(ctx context.Context, event *domain.ChatEvent) {
	envelope := CreateEventEnvelope(ctx, "message.sent", *event)
	_, err := topics_chat.MessageSentTopic.Publish(ctx, *envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish message sent event: %v", err)
	}
}

// PublishMessageDeliveredEvent publishes when a message is delivered.
func (e *chatEventPublisher) PublishMessageDeliveredEvent(ctx context.Context, event *domain.ChatEvent) {
	envelope := CreateEventEnvelope(ctx, "message.delivered", *event)
	_, err := topics_chat.MessageDeliveredTopic.Publish(ctx, *envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish message delivered event: %v", err)
	}
}

// PublishThreadCreatedEvent publishes when a thread is created.
func (e *chatEventPublisher) PublishThreadCreatedEvent(ctx context.Context, event *domain.ChatEvent) {
	envelope := CreateEventEnvelope(ctx, "thread.created", *event)
	_, err := topics_chat.ThreadCreatedTopic.Publish(ctx, *envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish thread created event: %v", err)
	}
}
