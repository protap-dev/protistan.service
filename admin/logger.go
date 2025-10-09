package admin

import (
	"context"
	"log"
)

// ServiceLogger handles logging for admin service operations
type ServiceLogger interface {
	LogAdminAction(ctx context.Context, action, adminUserID, targetUserID string)
	LogError(ctx context.Context, operation string, err error)
}

type serviceLogger struct{}

func NewServiceLogger() ServiceLogger {
	return &serviceLogger{}
}

func (l *serviceLogger) LogAdminAction(ctx context.Context, action, adminUserID, targetUserID string) {
	log.Printf("[ADMIN] Admin action: %s by admin %s on user %s", action, adminUserID, targetUserID)
}

func (l *serviceLogger) LogError(ctx context.Context, operation string, err error) {
	log.Printf("[ADMIN] Error in %s: %v", operation, err)
}
