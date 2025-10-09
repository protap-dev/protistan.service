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
	logger Logger
}

func NewAuthHelper(logger Logger) *AuthHelper {
	return &AuthHelper{logger: logger}
}

// ExtractUserContext extracts and validates user from auth context
func (h *AuthHelper) ExtractUserContext(ctx context.Context, action string) (*UserContext, error) {
	userID, ok := auth.UserID()
	if !ok {
		return nil, ErrUnauthenticated
	}

	userIDStr := string(userID)
	h.logger.LogUserAction(ctx, action, userIDStr)

	userUUID, err := uuid.FromString(userIDStr)
	if err != nil {
		h.logger.LogError(ctx, "invalid_uuid_format", err)
		return nil, ErrUnauthenticated
	}

	return &UserContext{
		ID:   userIDStr,
		UUID: userUUID,
	}, nil
}
