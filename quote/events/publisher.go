package events

import (
	"context"
	"log"

	topics_quote "encore.app/core/events/topics/quotes"
	"encore.app/quote/domain"
)

// eventPublisher implements the domain.EventPublisher interface.
// It publishes events wrapped in envelopes for observability and tracing.
type eventPublisher struct{}

// NewEventPublisher creates a new event publisher.
func NewEventPublisher() domain.EventPublisher {
	return &eventPublisher{}
}

// PublishQuoteProposedEvent publishes when a quote is proposed.
func (e *eventPublisher) PublishQuoteProposedEvent(ctx context.Context, event *domain.QuoteEvent) {
	envelope := CreateEventEnvelope(ctx, "quote.proposed", *event)
	_, err := topics_quote.QuoteProposedTopic.Publish(ctx, *envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish quote proposed event: %v", err)
	}
}

// PublishQuoteAcceptedEvent publishes when a quote is accepted.
func (e *eventPublisher) PublishQuoteAcceptedEvent(ctx context.Context, event *domain.QuoteEvent) {
	envelope := CreateEventEnvelope(ctx, "quote.accepted", *event)
	_, err := topics_quote.QuoteAcceptedTopic.Publish(ctx, *envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish quote accepted event: %v", err)
	}
}

// PublishQuoteRejectedEvent publishes when a quote is rejected.
func (e *eventPublisher) PublishQuoteRejectedEvent(ctx context.Context, event *domain.QuoteEvent) {
	envelope := CreateEventEnvelope(ctx, "quote.rejected", *event)
	_, err := topics_quote.QuoteRejectedTopic.Publish(ctx, *envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish quote rejected event: %v", err)
	}
}

// PublishQuoteExpiredEvent publishes when a quote expires.
func (e *eventPublisher) PublishQuoteExpiredEvent(ctx context.Context, event *domain.QuoteEvent) {
	envelope := CreateEventEnvelope(ctx, "quote.expired", *event)
	_, err := topics_quote.QuoteExpiredTopic.Publish(ctx, *envelope)
	if err != nil {
		log.Printf("ERROR: failed to publish quote expired event: %v", err)
	}
}
