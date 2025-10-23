package eventscommon

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// CreateEventEnvelope creates a new event envelope with the provided data and metadata
func CreateEventEnvelope[T any](ctx context.Context, eventType string, data T, producer string) *EventEnvelope[T] {
	metadata, _ := ExtractEventMetadata(ctx)

	// Generate new correlation ID if not present in metadata
	correlationID := ""
	causationID := "" // Causation is the ID of the PREVIOUS event that caused this one

	if metadata != nil {
		correlationID = metadata.CorrelationID
		causationID = metadata.CausationID // Use causation from metadata (previous event's ID)
	}

	// If no correlation ID in metadata, generate new one (this is a root event)
	if correlationID == "" {
		correlationID = GenerateUUID()
	}

	// If no causation ID in metadata, leave it empty (this is a root event with no prior cause)
	// DO NOT generate a random causation ID - it should only be set if there's an actual previous event

	return &EventEnvelope[T]{
		EventID:       GenerateUUID(), // New unique ID for THIS event
		EventType:     eventType,
		OccurredAt:    time.Now(),
		CorrelationID: correlationID, // Same for all events in saga
		CausationID:   causationID,   // ID of previous event that caused this (empty for root)
		Producer:      producer,
		Data:          data,
	}
}

// GenerateUUID generates a new UUID using Google UUID library
func GenerateUUID() string {
	newuid, _ := uuid.NewV7()
	return newuid.String()
}
