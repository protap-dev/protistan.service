package relay

import (
	"context"

	"encore.app/chat/domain"
	chatevents "encore.app/chat/events"
	topics_chat "encore.app/core/events/topics/chat"
	"encore.app/core/repository"
)

// ChatEvent implements the core EventData interface
// This makes ChatEvent compatible with the core relay system
type ChatEvent struct {
	domain.ChatEvent
}

// EventType returns the event type for routing
func (e ChatEvent) EventType() string {
	return e.ChatEvent.EventType
}

// ChatPublisher implements the core relay Publisher interface for chat events
type ChatPublisher struct{}

// PublishToTopic publishes the chat event to the correct topic based on the stored topic name in outbox
func (p *ChatPublisher) PublishToTopic(ctx context.Context, outboxEvent *repository.OutboxEvent, event ChatEvent) error {
	// Use the stored topic name from outbox table for routing
	switch outboxEvent.Topic {
	case "chat-v1-message-sent":
		envelope := chatevents.CreateEventEnvelope(ctx, "message.sent", event.ChatEvent)
		_, err := topics_chat.MessageSentTopic.Publish(ctx, *envelope)
		return err

	case "chat-v1-message-delivered":
		envelope := chatevents.CreateEventEnvelope(ctx, "message.delivered", event.ChatEvent)
		_, err := topics_chat.MessageDeliveredTopic.Publish(ctx, *envelope)
		return err

	case "chat-v1-thread-created":
		envelope := chatevents.CreateEventEnvelope(ctx, "thread.created", event.ChatEvent)
		_, err := topics_chat.ThreadCreatedTopic.Publish(ctx, *envelope)
		return err

	default:
		return nil // Unknown topic, skip
	}
}
