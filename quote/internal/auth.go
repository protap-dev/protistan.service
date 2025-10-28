package internal

import (
	"context"
	"net/http"

	eventscommon "encore.app/core/events"
	"encore.dev/beta/auth"
	"encore.dev/types/uuid"
)

// UserContext extends the basic user context with event metadata
type UserContext struct {
	ID       string
	UUID     uuid.UUID
	Metadata *EventMetadata
}

type EventMetadata = eventscommon.EventMetadata

// AuthHelper handles authentication and request metadata extraction
type AuthHelper struct {
	logger ServiceLogger
}

// NewAuthHelper creates a new auth helper with logging
func NewAuthHelper(logger ServiceLogger) *AuthHelper {
	return &AuthHelper{logger: logger}
}

// ExtractUserContext extracts user and request metadata from context
func (h *AuthHelper) ExtractUserContext(ctx context.Context, action string) (*UserContext, error) {
	userID, ok := auth.UserID()
	if !ok {
		return nil, ErrUnauthenticated
	}

	userIDStr := string(userID)

	// Extract correlation/causation IDs from HTTP headers if available
	metadata, _ := eventscommon.ExtractEventMetadata(ctx)

	h.logger.Info(ctx, action, map[string]any{
		"user_id":        userIDStr,
		"correlation_id": metadata.CorrelationID,
		"causation_id":   metadata.CausationID,
		"request_id":     metadata.RequestID,
	})

	userUUID, err := uuid.FromString(userIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid_uuid_format", err, map[string]any{"user_id": userIDStr})
		return nil, ErrUnauthenticated
	}

	return &UserContext{
		ID:       userIDStr,
		UUID:     userUUID,
		Metadata: metadata,
	}, nil
}

// ExtractMetadataFromHTTPRequest extracts event metadata from HTTP request headers
func ExtractMetadataFromHTTPRequest(req *http.Request) *EventMetadata {
	correlationID := req.Header.Get("X-Correlation-ID")
	causationID := req.Header.Get("X-Causation-ID")
	requestID := req.Header.Get("X-Request-ID")

	// Generate IDs if not present in headers
	if correlationID == "" {
		correlationID = GenerateRandomID()
	}

	if requestID == "" {
		requestID = correlationID // Use correlation ID as request ID if not provided
	}

	// Note: causationID left empty if not in headers (this is the first event)

	return &EventMetadata{
		CorrelationID: correlationID,
		CausationID:   causationID,
		RequestID:     requestID,
		UserID:        "", // Will be filled by auth system
	}
}
