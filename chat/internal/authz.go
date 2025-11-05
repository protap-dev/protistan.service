package internal

import (
	"context"
	"encoding/json"

	"encore.app/chat/domain"
	"encore.dev/beta/errs"
)

// AuthorizeThreadParticipant ensures the user belongs to the chat thread.
func AuthorizeThreadParticipant(ctx context.Context, userID string, thread *domain.Thread) error {
	if thread == nil {
		return errs.B().Code(errs.NotFound).Msg("thread not found").Err()
	}

	// Customer is always a participant
	if thread.CustomerID == userID {
		return nil
	}

	// Check for artisan participation via metadata
	if len(thread.Metadata) > 0 {
		var metadata map[string]string
		// We can ignore the error if the metadata is not in the expected format
		if json.Unmarshal(thread.Metadata, &metadata) == nil {
			if artisanID, ok := metadata["artisan_user_id"]; ok && artisanID == userID {
				return nil
			}
		}
	}

	return errs.B().Code(errs.PermissionDenied).Msg("user is not a participant in this thread").Err()
}

// AuthorizeSendMessage validates that the user can send a message in the thread.
func AuthorizeSendMessage(ctx context.Context, userID string, thread *domain.Thread) error {
	return AuthorizeThreadParticipant(ctx, userID, thread)
}

// AuthorizeReadMessages validates that the user can read messages in the thread.
func AuthorizeReadMessages(ctx context.Context, userID string, thread *domain.Thread) error {
	return AuthorizeThreadParticipant(ctx, userID, thread)
}

// AuthorizeMarkThreadAsRead validates that the user can mark messages as read in the thread.
func AuthorizeMarkThreadAsRead(ctx context.Context, userID string, thread *domain.Thread) error {
	return AuthorizeThreadParticipant(ctx, userID, thread)
}

// AuthorizeThreadSubscription validates that the user can subscribe to real-time thread updates.
func AuthorizeThreadSubscription(ctx context.Context, userID string, thread *domain.Thread) error {
	return AuthorizeThreadParticipant(ctx, userID, thread)
}
