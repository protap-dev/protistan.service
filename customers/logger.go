package customers

import (
	"context"
	"log"
)

// ServiceLogger handles logging for the customers service
type ServiceLogger interface {
	LogUserAction(ctx context.Context, action, userID string)
	LogError(ctx context.Context, operation string, err error)
	LogAddressAction(ctx context.Context, action, addressID, userID string)
}

type serviceLogger struct {
	logger *log.Logger
}

func NewServiceLogger() ServiceLogger {
	return &serviceLogger{
		logger: log.Default(),
	}
}

func (l *serviceLogger) LogUserAction(ctx context.Context, action, userID string) {
	l.logger.Printf("[CUSTOMER_ACTION] action=%s user_id=%s", action, userID)
}

func (l *serviceLogger) LogError(ctx context.Context, operation string, err error) {
	l.logger.Printf("[CUSTOMER_ERROR] operation=%s error=%s", operation, err.Error())
}

func (l *serviceLogger) LogAddressAction(ctx context.Context, action, addressID, userID string) {
	l.logger.Printf("[ADDRESS_ACTION] action=%s address_id=%s user_id=%s", action, addressID, userID)
}
