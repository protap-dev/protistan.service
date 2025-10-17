package repository

import (
	"context"
	"encoding/json"
	"time"

	"encore.app/booking/domain"
	binternal "encore.app/booking/internal"
	"gorm.io/datatypes"
)

// CreateEventInOutbox writes an event directly to the outbox table within a transaction.
// This ensures events are published atomically with database changes.
func (r *bookingRepository) CreateEventInOutbox(ctx context.Context, event *domain.BookingEvent) error {
	jsonData, err := eventToJSON(event)
	if err != nil {
		return err
	}

	dbModel := &outboxEventDBModel{
		ID:         binternal.GenerateUUID(),
		Topic:      getTopicForEvent(event),
		Data:       jsonData,
		InsertedAt: time.Now(),
	}
	return r.db.WithContext(ctx).Create(dbModel).Error
}

// Helper functions for outbox functionality

// getTopicForEvent determines the appropriate topic name for a booking event
func getTopicForEvent(event *domain.BookingEvent) string {
	switch event.Status {
	case domain.BookingRequested:
		return "booking.status"
	case domain.BookingOfferPending:
		return "booking.offered"
	case domain.BookingOfferRejected:
		return "booking.status"
	case domain.BookingAssigned:
		return "booking.assigned"
	case domain.BookingPendingQuote:
		return "booking.status"
	case domain.BookingQuoteProposed:
		return "booking.status"
	case domain.BookingQuoteAccepted:
		return "booking.quote.accepted"
	case domain.BookingPaymentPending:
		return "booking.status"
	case domain.BookingConfirmed:
		return "booking.payment.confirmed"
	case domain.BookingEnroute:
		return "booking.status"
	case domain.BookingInProgress:
		return "booking.status"
	case domain.BookingCompleted:
		return "booking.status"
	case domain.BookingCancelled:
		return "booking.cancelled"
	case domain.BookingClosed:
		return "booking.status"
	default:
		return "booking.status"
	}
}

// eventToJSON converts a booking event to JSON for storage in outbox
func eventToJSON(event *domain.BookingEvent) (datatypes.JSON, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return datatypes.JSON("{}"), err
	}
	return datatypes.JSON(data), nil
}
