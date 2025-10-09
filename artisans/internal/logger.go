package internal

import (
	"context"
	"log"
)

type Logger interface {
	LogUserAction(ctx context.Context, action, userID string)
	LogArtisanAction(ctx context.Context, action, artisanID, userID string)
	LogError(ctx context.Context, operation string, err error)
}

type logger struct{}

func NewLogger() Logger {
	return &logger{}
}

func (l *logger) LogUserAction(ctx context.Context, action, userID string) {
	log.Printf("[USER_ACTION] action=%s user_id=%s", action, userID)
}

func (l *logger) LogArtisanAction(ctx context.Context, action, artisanID, userID string) {
	log.Printf("[ARTISAN_ACTION] action=%s artisan_id=%s user_id=%s", action, artisanID, userID)
}

func (l *logger) LogError(ctx context.Context, operation string, err error) {
	log.Printf("[ERROR] operation=%s error=%v", operation, err)
}
