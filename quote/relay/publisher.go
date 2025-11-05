package relay

import (
	"context"

	eventscommon "encore.app/core/events"
	topics_quote "encore.app/core/events/topics/quotes"
	"encore.app/core/repository"
	quoteevents "encore.app/quote/events"
)

// QuoteEvent implements the core EventData interface
// This makes QuoteEvent compatible with the core relay system
type QuoteEvent struct {
	eventscommon.QuoteEvent
}

// EventType returns the event type for routing
func (e QuoteEvent) EventType() string {
	// Return event type based on the state
	return "quote"
}

// QuotePublisher implements the core relay Publisher interface for quote events
type QuotePublisher struct{}

// PublishToTopic publishes the quote event to the correct topic based on the stored topic name in outbox
func (p *QuotePublisher) PublishToTopic(ctx context.Context, outboxEvent *repository.OutboxEvent, event QuoteEvent) error {
	// Use the stored topic name from outbox table for routing
	switch outboxEvent.Topic {
	case "quote-v1-proposed":
		envelope := quoteevents.CreateEventEnvelope(ctx, "quote.proposed", event.QuoteEvent)
		_, err := topics_quote.QuoteProposedTopic.Publish(ctx, *envelope)
		return err

	case "quote-v1-accepted":
		envelope := quoteevents.CreateEventEnvelope(ctx, "quote.accepted", event.QuoteEvent)
		_, err := topics_quote.QuoteAcceptedTopic.Publish(ctx, *envelope)
		return err

	case "quote-v1-rejected":
		envelope := quoteevents.CreateEventEnvelope(ctx, "quote.rejected", event.QuoteEvent)
		_, err := topics_quote.QuoteRejectedTopic.Publish(ctx, *envelope)
		return err

	case "quote-v1-expired":
		envelope := quoteevents.CreateEventEnvelope(ctx, "quote.expired", event.QuoteEvent)
		_, err := topics_quote.QuoteExpiredTopic.Publish(ctx, *envelope)
		return err
	default:
		// Default to proposed topic for unknown topic names
		envelope := quoteevents.CreateEventEnvelope(ctx, "quote.proposed", event.QuoteEvent)
		_, err := topics_quote.QuoteProposedTopic.Publish(ctx, *envelope)
		return err
	}
}
