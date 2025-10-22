package repository

import (
	"context"
	"errors"
	"time"

	"encore.app/booking/domain"
	"gorm.io/gorm"
)

// CreateOffer inserts a new booking offer
func (r *bookingRepository) CreateOffer(ctx context.Context, offer *domain.BookingOffer) error {
	dbModel := offerToDBModel(offer)

	if err := r.db.WithContext(ctx).Create(&dbModel).Error; err != nil {
		return err
	}

	offer.ID = dbModel.ID
	offer.CreatedAt = dbModel.CreatedAt
	offer.UpdatedAt = dbModel.UpdatedAt
	return nil
}

// GetOfferByID retrieves an offer by ID
func (r *bookingRepository) GetOfferByID(ctx context.Context, offerID string) (*domain.BookingOffer, error) {
	var dbModel offerDBModel
	if err := r.db.WithContext(ctx).First(&dbModel, "id = ?", offerID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrOfferNotFound
		}
		return nil, err
	}
	return offerToDomainModel(&dbModel), nil
}

// GetOffersByBookingID retrieves all offers for a booking
func (r *bookingRepository) GetOffersByBookingID(ctx context.Context, bookingID string) ([]*domain.BookingOffer, error) {
	var dbModels []offerDBModel
	if err := r.db.WithContext(ctx).
		Where("booking_id = ?", bookingID).
		Order("created_at DESC").
		Find(&dbModels).Error; err != nil {
		return nil, err
	}

	result := make([]*domain.BookingOffer, len(dbModels))
	for i := range dbModels {
		result[i] = offerToDomainModel(&dbModels[i])
	}
	return result, nil
}

// GetOffersByArtisanID retrieves offers for an artisan filtered by status
func (r *bookingRepository) GetOffersByArtisanID(ctx context.Context, artisanID string, status domain.BookingOfferStatus) ([]*domain.BookingOffer, error) {
	query := r.db.WithContext(ctx).Where("artisan_id = ?", artisanID)

	if status != "" {
		query = query.Where("status = ?", string(status))
	}

	var dbModels []offerDBModel
	if err := query.Order("created_at DESC").Find(&dbModels).Error; err != nil {
		return nil, err
	}

	result := make([]*domain.BookingOffer, len(dbModels))
	for i := range dbModels {
		result[i] = offerToDomainModel(&dbModels[i])
	}
	return result, nil
}

// UpdateOfferStatus updates the status of an offer
func (r *bookingRepository) UpdateOfferStatus(ctx context.Context, offerID string, status domain.BookingOfferStatus, reason *string) error {
	updates := map[string]any{
		"status":       string(status),
		"responded_at": time.Now(),
		"updated_at":   time.Now(),
	}

	if reason != nil {
		updates["reject_reason"] = *reason
	}

	result := r.db.WithContext(ctx).
		Model(&offerDBModel{}).
		Where("id = ?", offerID).
		Updates(updates)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return domain.ErrOfferNotFound
	}

	return nil
}

// CancelPendingOffers cancels all pending offers for a booking
func (r *bookingRepository) CancelPendingOffers(ctx context.Context, bookingID string) error {
	return r.db.WithContext(ctx).
		Model(&offerDBModel{}).
		Where("booking_id = ? AND status = ?", bookingID, string(domain.OfferPending)).
		Updates(map[string]interface{}{
			"status":     string(domain.OfferCancelled),
			"updated_at": time.Now(),
		}).Error
}

// FindExpiredOffers finds all pending offers that have expired
func (r *bookingRepository) FindExpiredOffers(ctx context.Context) ([]*domain.BookingOffer, error) {
	var offers []*domain.BookingOffer

	err := r.db.WithContext(ctx).
		Where("status = ? AND expires_at <= ?", domain.OfferPending, time.Now()).
		Order("expires_at ASC"). // Process oldest first
		Find(&offers).Error

	if err != nil {
		return nil, err
	}

	return offers, nil
}

// UpdateOffer updates an existing offer
func (r *bookingRepository) UpdateOffer(ctx context.Context, offer *domain.BookingOffer) error {
	offer.UpdatedAt = time.Now()

	result := r.db.WithContext(ctx).
		Model(&domain.BookingOffer{}).
		Where("id = ?", offer.ID).
		Updates(map[string]any{
			"status":       offer.Status,
			"responded_at": offer.RespondedAt,
			"updated_at":   offer.UpdatedAt,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return domain.ErrOfferNotFound
	}

	return nil
}

// CreateOfferExpiredEventInOutbox writes offer expired event to outbox
func (r *bookingRepository) CreateOfferExpiredEventInOutbox(ctx context.Context, event *domain.BookingEvent) error {
	return insertEventInOutbox(r.db, ctx, event, "booking.v1.offer.expired")
}
