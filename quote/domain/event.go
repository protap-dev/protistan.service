package domain

import (
	"context"

	eventscommon "encore.app/core/events"
)

// EventPublisher defines the interface for publishing quote-related events.
// By defining this in the domain, we decouple the application core from the pub/sub implementation
type EventPublisher interface {
	PublishQuoteProposedEvent(ctx context.Context, event *eventscommon.QuoteEvent)
	PublishQuoteAcceptedEvent(ctx context.Context, event *eventscommon.QuoteEvent)
	PublishQuoteRejectedEvent(ctx context.Context, event *eventscommon.QuoteEvent)
}
