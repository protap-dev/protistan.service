package eventscommon

import (
	"context"
)

// EventMetadata contains request tracing information for event correlation
type EventMetadata struct {
	CorrelationID string
	CausationID   string
	UserID        string
	RequestID     string
}

// ContextKey for storing event metadata in context
type contextKey string

const (
	metadataKey contextKey = "event_metadata"
)

// ExtractEventMetadata extracts event metadata from context (for internal use)
func ExtractEventMetadata(ctx context.Context) (*EventMetadata, bool) {
	metadata, ok := ctx.Value(metadataKey).(*EventMetadata)
	return metadata, ok
}

// WithEventMetadata adds event metadata to context
func WithEventMetadata(ctx context.Context, metadata *EventMetadata) context.Context {
	return context.WithValue(ctx, metadataKey, metadata)
}

// GetEventMetadata creates event metadata for the current request context
func GetEventMetadata(ctx context.Context) *EventMetadata {
	if metadata, ok := ExtractEventMetadata(ctx); ok {
		return metadata
	}
	return &EventMetadata{
		CorrelationID: GenerateUUID(),
		CausationID:   "",
		UserID:        "",
		RequestID:     GenerateUUID(),
	}
}
