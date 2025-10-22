package events

import (
	"context"

	"encore.app/booking/domain"
)

// EventSubscriber defines the interface for handling booking events from other services
// This interface is implemented by the Service in service.go
type EventSubscriber interface {
	OnQuoteProposed(ctx context.Context, event *domain.BookingEvent) error
	OnQuoteAccepted(ctx context.Context, event *domain.BookingEvent) error
	OnQuoteRejected(ctx context.Context, event *domain.BookingEvent) error
	OnPaymentConfirmed(ctx context.Context, event *domain.BookingEvent) error
}
