package repository

import (
	"context"
	"encoding/json"
	"time"

	"encore.app/booking/domain"
	binternal "encore.app/booking/internal"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// insertEventInOutbox is a generic function for inserting events into the outbox table
func insertEventInOutbox[T any](db *gorm.DB, ctx context.Context, event T, topic string) error {
	jsonData, err := eventToJSON(event)
	if err != nil {
		return err
	}

	dbModel := &outboxEventDBModel{
		ID:          binternal.GenerateUUID(),
		Topic:       topic,
		Data:        jsonData,
		InsertedAt:  time.Now(),
		ProcessedAt: nil, // initially nil, set when processed
	}
	return db.WithContext(ctx).Create(dbModel).Error
}

// CreateEventInOutbox writes a booking event directly to the outbox table within a transaction.
// This ensures events are published atomically with database changes.
func (r *bookingRepository) CreateEventInOutbox(ctx context.Context, event *domain.BookingEvent) error {
	return insertEventInOutbox(r.db, ctx, event, getTopicForEvent(event))
}

// CreateRematchEventInOutbox writes a rematch event directly to the outbox table within a transaction.
// This ensures rematch events are published atomically.
func (r *bookingRepository) CreateRematchEventInOutbox(ctx context.Context, event *domain.RematchEvent) error {
	return insertEventInOutbox(r.db, ctx, event, "booking-rematch")
}

// Helper functions for outbox functionality

// eventToJSON converts any serializable event to JSON for storage in outbox
func eventToJSON[T any](event T) (datatypes.JSON, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return datatypes.JSON("{}"), err
	}
	return datatypes.JSON(data), nil
}

// getTopicForEvent determines the appropriate topic name for a booking event
func getTopicForEvent(event *domain.BookingEvent) string {
	switch event.Status {
	case domain.BookingRequested:
		return "booking.created"
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
	case domain.BookingQuoteRejected:
		return "booking.quote.rejected"
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
