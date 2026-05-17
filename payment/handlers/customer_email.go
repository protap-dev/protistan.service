package handlers

import (
	"context"
	"fmt"
	"strings"

	"encore.app/user"
	"encore.dev/types/uuid"
)

func (h *PaymentsHandler) fetchCustomerEmail(ctx context.Context, customerID string) (string, error) {
	if h.getCustomerEmail != nil {
		return h.getCustomerEmail(ctx, customerID)
	}
	return fetchCustomerEmailFromUserService(ctx, customerID)
}

func fetchCustomerEmailFromUserService(ctx context.Context, customerID string) (string, error) {
	parsedUserID, err := uuid.FromString(customerID)
	if err != nil {
		return "", fmt.Errorf("parse customer id: %w", err)
	}

	resp, err := user.FetchInternal(ctx, &user.InternalUserFetchRequest{
		UserID:      parsedUserID,
		IncludeUser: true,
	})
	if err != nil {
		return "", err
	}
	if resp == nil || resp.User == nil || strings.TrimSpace(resp.User.Email) == "" {
		return "", fmt.Errorf("customer email not found")
	}
	return strings.TrimSpace(resp.User.Email), nil
}
