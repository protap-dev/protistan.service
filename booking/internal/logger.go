package internal

import (
	"context"
	"log"
)

// ServiceLogger handles logging for the booking service
type ServiceLogger interface {
	Info(ctx context.Context, message string, fields map[string]interface{})
	Error(ctx context.Context, message string, err error, fields map[string]interface{})
	Warn(ctx context.Context, message string, fields map[string]interface{})
}

type serviceLogger struct {
	serviceName string
	logger      *log.Logger
}

func NewServiceLogger(serviceName string) ServiceLogger {
	return &serviceLogger{
		serviceName: serviceName,
		logger:      log.Default(),
	}
}

func (l *serviceLogger) Info(ctx context.Context, message string, fields map[string]interface{}) {
	l.logger.Printf("[%s_INFO] %s %v", l.serviceName, message, fields)
}

func (l *serviceLogger) Error(ctx context.Context, message string, err error, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields["error"] = err.Error()
	l.logger.Printf("[%s_ERROR] %s %v", l.serviceName, message, fields)
}

func (l *serviceLogger) Warn(ctx context.Context, message string, fields map[string]interface{}) {
	l.logger.Printf("[%s_WARN] %s %v", l.serviceName, message, fields)
}
