package repository

import (
	"context"

	"encore.app/booking/domain"
)

// CreateStatusHistory creates an audit trail entry for a booking status change.
func (r *bookingRepository) CreateStatusHistory(ctx context.Context, history *domain.BookingEvent) error {
	dbModel := &statusHistoryDBModel{
		BookingID:      history.BookingID,
		Status:         string(history.Status),
		PreviousStatus: string(history.PreviousStatus),
		ChangedBy:      history.UserID,
		Reason:         history.Reason,
		CreatedAt:      history.Timestamp,
	}
	return r.db.WithContext(ctx).Create(dbModel).Error
}
