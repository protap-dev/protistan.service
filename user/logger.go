package user

import (
	"context"
	"log"
)

// ServiceLogger handles logging for the user service
type ServiceLogger interface {
	LogUserAction(ctx context.Context, action, userID string)
	LogError(ctx context.Context, operation string, err error)
}

type serviceLogger struct {
	logger *log.Logger
}

func NewServiceLogger() ServiceLogger {
	return &serviceLogger{
		logger: log.Default(), // Use the default logger, can be configured as needed
	}
}

func (l *serviceLogger) LogUserAction(ctx context.Context, action, userID string) {
	l.logger.Printf("[USER_ACTION] action=%s user_id=%s", action, userID)
}

func (l *serviceLogger) LogError(ctx context.Context, operation string, err error) {
	l.logger.Printf("[USER_ERROR] operation=%s error=%s", operation, err.Error())
}
