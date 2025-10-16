package internal

import (
	"context"

	"encore.dev/beta/auth"
	"encore.dev/types/uuid"
)

type UserContext struct {
	ID   string
	UUID uuid.UUID
}

type AuthHelper struct {
	logger ServiceLogger
}

func NewAuthHelper(logger ServiceLogger) *AuthHelper {
	return &AuthHelper{logger: logger}
}

// ExtractUserContext extracts and validates user from auth context
func (h *AuthHelper) ExtractUserContext(ctx context.Context, action string) (*UserContext, error) {
	userID, ok := auth.UserID()
	if !ok {
		return nil, ErrUnauthenticated
	}

	userIDStr := string(userID)
	h.logger.Info(ctx, action, map[string]any{"user_id": userIDStr})

	userUUID, err := uuid.FromString(userIDStr)
	if err != nil {
		h.logger.Error(ctx, "invalid_uuid_format", err, map[string]any{"user_id": userIDStr})
		return nil, ErrUnauthenticated
	}

	return &UserContext{
		ID:   userIDStr,
		UUID: userUUID,
	}, nil
}
