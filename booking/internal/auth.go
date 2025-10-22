package internal

import (
	"context"
	"net/http"

	"encore.dev/beta/auth"
	"encore.dev/types/uuid"
)

// EventMetadata contains request tracing information for event correlation
type EventMetadata struct {
	CorrelationID string
	CausationID   string
	UserID        string
	RequestID     string
}

// UserContext extends the basic user context with event metadata
type UserContext struct {
	ID       string
	UUID     uuid.UUID
	Metadata *EventMetadata
}

// ContextKey for storing event metadata in context
type contextKey string

const (
	metadataKey contextKey = "event_metadata"
)

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
	metadata := h.extractEventMetadata(ctx, userIDStr)

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

// ExtractEventMetadata extracts event metadata from context (for internal use)
func ExtractEventMetadata(ctx context.Context) (*EventMetadata, bool) {
	metadata, ok := ctx.Value(metadataKey).(*EventMetadata)
	return metadata, ok
}

// WithEventMetadata adds event metadata to context
func WithEventMetadata(ctx context.Context, metadata *EventMetadata) context.Context {
	return context.WithValue(ctx, metadataKey, metadata)
}

// extractEventMetadata extracts correlation/causation IDs from HTTP headers
func (h *AuthHelper) extractEventMetadata(ctx context.Context, userID string) *EventMetadata {
	// Generate new correlation ID if not present
	correlationID := GenerateRandomID()
	causationID := GenerateRandomID()
	requestID := GenerateRandomID()

	// Try to extract from HTTP headers if available
	if req, ok := ctx.Value("http_request").(*http.Request); ok {
		// Extract correlation ID from headers
		if corrID := req.Header.Get("X-Correlation-ID"); corrID != "" {
			correlationID = corrID
		}

		// Extract causation ID from headers
		if causeID := req.Header.Get("X-Causation-ID"); causeID != "" {
			causationID = causeID
		}

		// Extract request ID from headers or generate
		if reqID := req.Header.Get("X-Request-ID"); reqID != "" {
			requestID = reqID
		} else {
			// Generate request ID based on correlation ID if not provided
			requestID = correlationID
		}
	}

	return &EventMetadata{
		CorrelationID: correlationID,
		CausationID:   causationID,
		UserID:        userID,
		RequestID:     requestID,
	}
}

// GetEventMetadata creates event metadata for the current request context
func GetEventMetadata(ctx context.Context) *EventMetadata {
	if metadata, ok := ExtractEventMetadata(ctx); ok {
		return metadata
	}
	return nil
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
