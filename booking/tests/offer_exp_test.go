package booking

import (
	"context"
	"testing"
	"time"

	"encore.app/booking/domain"
	"encore.app/booking/handlers"
	binternal "encore.app/booking/internal"
	"encore.app/booking/repository"
	"encore.app/core"
	"encore.app/core/cache"
	"encore.dev/et"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// setupOfferExpiryTest creates test environment for offer expiry tests
func setupOfferExpiryTest(t *testing.T) (*handlers.BookingsHandler, *gorm.DB, func()) {
	ctx := context.Background()

	testDB, err := et.NewTestDatabase(ctx, "booking")
	require.NoError(t, err)

	gormDB, err := gorm.Open(postgres.New(postgres.Config{
		Conn: testDB.Stdlib(),
	}), &gorm.Config{})
	require.NoError(t, err)

	coreSvc := core.NewCoreService(gormDB)
	logger := binternal.NewServiceLogger("test")
	authHelper := binternal.NewAuthHelper(logger)
	validator := domain.NewBookingValidator()
	offerValidator := domain.NewOfferValidator()
	cacheImpl := cache.NewInMemoryCache()

	mockPublisher := &mockOfferPublisher{
		publishedEvents: make([]*domain.BookingEvent, 0),
	}

	repo := repository.NewBookingRepository(gormDB, gormDB)
	handler := handlers.NewBookingsHandler(repo, validator, offerValidator, logger, cacheImpl, coreSvc, authHelper, mockPublisher)

	cleanup := func() {
		// Automatic cleanup by et.NewTestDatabase
	}

	return handler, gormDB, cleanup
}

// createTestBookingForOffer creates a minimal booking for offer testing
func createTestBookingForOffer(t *testing.T, db *gorm.DB, bookingID string) {
	err := db.Exec(`
        INSERT INTO bookings (
            id, customer_id, service_category_id, customer_address_id,
            title, description, status, priority, estimated_duration_mins,
            metadata, is_specific_artisan, offers_count,
            created_at, updated_at, version
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
        ON CONFLICT (id) DO NOTHING
    `, bookingID, uuid.New().String(), uuid.New().String(), uuid.New().String(),
		"Test Booking for Offers", "Minimal booking for offer expiry tests",
		"requested", "normal", 60, "{}", false, 0,
		time.Now(), time.Now(), 1).Error
	require.NoError(t, err, "failed to create test booking")
}

// createTestOffer creates a test offer with its parent booking in the database
func createTestOffer(t *testing.T, db *gorm.DB, offer *domain.BookingOffer) {
	// Ensure parent booking exists
	createTestBookingForOffer(t, db, offer.BookingID)

	// Create the offer
	err := db.Exec(`
        INSERT INTO booking_offers (
            id, booking_id, artisan_id, status, offered_by,
            offered_at, expires_at, created_at, updated_at
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
    `, offer.ID, offer.BookingID, offer.ArtisanID, string(offer.Status),
		offer.OfferedBy, offer.OfferedAt, offer.ExpiresAt,
		offer.CreatedAt, offer.UpdatedAt).Error
	require.NoError(t, err, "failed to create test offer")
}

// TestExpireOffers_IdentifiesExpiredOffers tests that expired offers are identified
func TestExpireOffers_IdentifiesExpiredOffers(t *testing.T) {
	handler, db, cleanup := setupOfferExpiryTest(t)
	defer cleanup()

	ctx := context.Background()

	bookingID := uuid.New().String()
	now := time.Now()

	// Create offers with different expiry times
	testOffers := []struct {
		id           string
		status       domain.BookingOfferStatus
		expiresAt    time.Time
		shouldExpire bool
	}{
		{
			id:           uuid.New().String(),
			status:       domain.OfferPending,
			expiresAt:    now.Add(-2 * time.Hour), // Expired 2 hours ago
			shouldExpire: true,
		},
		{
			id:           uuid.New().String(),
			status:       domain.OfferPending,
			expiresAt:    now.Add(-5 * time.Minute), // Expired 5 minutes ago
			shouldExpire: true,
		},
		{
			id:           uuid.New().String(),
			status:       domain.OfferPending,
			expiresAt:    now.Add(2 * time.Hour), // Not expired
			shouldExpire: false,
		},
		{
			id:           uuid.New().String(),
			status:       domain.OfferAccepted, // Already accepted
			expiresAt:    now.Add(-1 * time.Hour),
			shouldExpire: false, // Should not process non-pending offers
		},
	}

	// Seed test offers
	for _, to := range testOffers {
		offer := &domain.BookingOffer{
			ID:        to.id,
			BookingID: bookingID,
			ArtisanID: uuid.New().String(),
			Status:    to.status,
			OfferedBy: uuid.New().String(),
			OfferedAt: now.Add(-24 * time.Hour),
			ExpiresAt: to.expiresAt,
			CreatedAt: now.Add(-24 * time.Hour),
			UpdatedAt: now.Add(-24 * time.Hour),
		}
		createTestOffer(t, db, offer)
	}

	t.Logf("✓ Seeded %d test offers", len(testOffers))

	// Run expiry worker
	err := handler.ExpireOffers(ctx)
	require.NoError(t, err)
	t.Log("✓ Expiry worker executed successfully")

	// Verify only expired pending offers were transitioned
	expiredCount := 0
	for _, to := range testOffers {
		var status string
		err := db.Raw("SELECT status FROM booking_offers WHERE id = ?", to.id).Scan(&status).Error
		require.NoError(t, err)

		if to.shouldExpire {
			assert.Equal(t, string(domain.OfferExpired), status,
				"offer %s should be expired", to.id)
			expiredCount++
			t.Logf("✓ Offer %s correctly expired", to.id)
		} else {
			assert.Equal(t, string(to.status), status,
				"offer %s should remain in %s status", to.id, to.status)
			t.Logf("✓ Offer %s correctly unchanged (%s)", to.id, to.status)
		}
	}

	assert.Equal(t, 2, expiredCount, "should expire exactly 2 pending expired offers")
	t.Logf("✓ Expired %d offers as expected", expiredCount)
}

// TestExpireOffers_PublishesEvents tests that events are published for expired offers
func TestExpireOffers_PublishesEvents(t *testing.T) {
	handler, db, cleanup := setupOfferExpiryTest(t)
	defer cleanup()

	ctx := context.Background()

	now := time.Now()
	bookingID := uuid.New().String()

	// Create 3 expired pending offers
	expiredOffers := []string{
		uuid.New().String(),
		uuid.New().String(),
		uuid.New().String(),
	}

	for _, offerID := range expiredOffers {
		offer := &domain.BookingOffer{
			ID:        offerID,
			BookingID: bookingID,
			ArtisanID: uuid.New().String(),
			Status:    domain.OfferPending,
			OfferedBy: uuid.New().String(),
			OfferedAt: now.Add(-25 * time.Hour),
			ExpiresAt: now.Add(-1 * time.Hour), // Expired 1 hour ago
			CreatedAt: now.Add(-25 * time.Hour),
			UpdatedAt: now.Add(-25 * time.Hour),
		}
		createTestOffer(t, db, offer)
	}

	t.Logf("✓ Seeded %d expired offers", len(expiredOffers))

	// Count events before
	var eventsBefore int
	db.Raw("SELECT COUNT(*) FROM outbox WHERE topic LIKE ?", "booking.v1.offer.expired%").Scan(&eventsBefore)

	// Run expiry worker
	err := handler.ExpireOffers(ctx)
	require.NoError(t, err)

	// Count events after
	var eventsAfter int
	db.Raw("SELECT COUNT(*) FROM outbox WHERE topic LIKE ?", "booking.v1.offer.expired%").Scan(&eventsAfter)

	// Verify events published
	eventsPublished := eventsAfter - eventsBefore
	assert.Equal(t, len(expiredOffers), eventsPublished,
		"should publish one event per expired offer")
	t.Logf("✓ Published %d events for expired offers", eventsPublished)

	// Verify event payload contains offer IDs
	for _, offerID := range expiredOffers {
		var count int
		db.Raw("SELECT COUNT(*) FROM outbox WHERE data::text LIKE ?", "%"+offerID+"%").Scan(&count)
		assert.Greater(t, count, 0, "event for offer %s should exist in outbox", offerID)
	}

	t.Log("✓ All expired offer events verified in outbox")
}

// TestExpireOffers_Idempotency tests that already expired offers are not processed again
func TestExpireOffers_Idempotency(t *testing.T) {
	handler, db, cleanup := setupOfferExpiryTest(t)
	defer cleanup()

	ctx := context.Background()

	now := time.Now()
	offerID := uuid.New().String()

	// Create already expired offer
	offer := &domain.BookingOffer{
		ID:        offerID,
		BookingID: uuid.New().String(),
		ArtisanID: uuid.New().String(),
		Status:    domain.OfferExpired, // Already expired
		OfferedBy: uuid.New().String(),
		OfferedAt: now.Add(-25 * time.Hour),
		ExpiresAt: now.Add(-1 * time.Hour),
		CreatedAt: now.Add(-25 * time.Hour),
		UpdatedAt: now.Add(-1 * time.Hour), // Updated when expired
	}
	createTestOffer(t, db, offer)

	t.Log("✓ Seeded already expired offer")

	// Count events before
	var eventsBefore int
	db.Raw("SELECT COUNT(*) FROM outbox WHERE data::text LIKE ?", "%"+offerID+"%").Scan(&eventsBefore)

	// Run expiry worker multiple times
	err := handler.ExpireOffers(ctx)
	require.NoError(t, err)

	err = handler.ExpireOffers(ctx)
	require.NoError(t, err)

	err = handler.ExpireOffers(ctx)
	require.NoError(t, err)

	t.Log("✓ Ran expiry worker 3 times")

	// Count events after
	var eventsAfter int
	db.Raw("SELECT COUNT(*) FROM outbox WHERE data::text LIKE ?", "%"+offerID+"%").Scan(&eventsAfter)

	// Verify no new events published
	assert.Equal(t, eventsBefore, eventsAfter,
		"should not publish duplicate events for already expired offers")

	// Verify status unchanged
	var status string
	db.Raw("SELECT status FROM booking_offers WHERE id = ?", offerID).Scan(&status)
	assert.Equal(t, string(domain.OfferExpired), status)

	t.Log("✓ Idempotency verified: no duplicate processing")
}

// TestExpireOffers_AtomicTransitions tests atomic state transitions
func TestExpireOffers_AtomicTransitions(t *testing.T) {
	handler, db, cleanup := setupOfferExpiryTest(t)
	defer cleanup()

	ctx := context.Background()

	now := time.Now()
	bookingID := uuid.New().String()

	// Create multiple expired offers for the same booking
	offerIDs := []string{
		uuid.New().String(),
		uuid.New().String(),
		uuid.New().String(),
	}

	for _, offerID := range offerIDs {
		offer := &domain.BookingOffer{
			ID:        offerID,
			BookingID: bookingID,
			ArtisanID: uuid.New().String(),
			Status:    domain.OfferPending,
			OfferedBy: uuid.New().String(),
			OfferedAt: now.Add(-25 * time.Hour),
			ExpiresAt: now.Add(-1 * time.Hour),
			CreatedAt: now.Add(-25 * time.Hour),
			UpdatedAt: now.Add(-25 * time.Hour),
		}
		createTestOffer(t, db, offer)
	}

	t.Logf("✓ Seeded %d offers for same booking", len(offerIDs))

	// Run expiry worker
	err := handler.ExpireOffers(ctx)
	require.NoError(t, err)

	// Verify all offers transitioned atomically
	for _, offerID := range offerIDs {
		var status string
		db.Raw("SELECT status FROM booking_offers WHERE id = ?", offerID).Scan(&status)
		assert.Equal(t, string(domain.OfferExpired), status,
			"offer %s should be expired", offerID)
	}

	t.Logf("✓ All %d offers atomically transitioned to expired", len(offerIDs))
}

// TestExpireOffers_NoExpiredOffers tests worker with no expired offers
func TestExpireOffers_NoExpiredOffers(t *testing.T) {
	handler, db, cleanup := setupOfferExpiryTest(t)
	defer cleanup()

	ctx := context.Background()

	now := time.Now()

	// Create only non-expired offers
	testOffers := []struct {
		status    domain.BookingOfferStatus
		expiresAt time.Time
	}{
		{domain.OfferPending, now.Add(2 * time.Hour)},
		{domain.OfferPending, now.Add(24 * time.Hour)},
		{domain.OfferAccepted, now.Add(-1 * time.Hour)},
		{domain.OfferRejected, now.Add(-1 * time.Hour)},
	}

	for _, to := range testOffers {
		offer := &domain.BookingOffer{
			ID:        uuid.New().String(),
			BookingID: uuid.New().String(),
			ArtisanID: uuid.New().String(),
			Status:    to.status,
			OfferedBy: uuid.New().String(),
			OfferedAt: now.Add(-1 * time.Hour),
			ExpiresAt: to.expiresAt,
			CreatedAt: now.Add(-1 * time.Hour),
			UpdatedAt: now.Add(-1 * time.Hour),
		}
		createTestOffer(t, db, offer)
	}

	t.Logf("✓ Seeded %d non-expired offers", len(testOffers))

	// Count events before
	var eventsBefore int
	db.Raw("SELECT COUNT(*) FROM outbox").Scan(&eventsBefore)

	// Run expiry worker
	err := handler.ExpireOffers(ctx)
	require.NoError(t, err)

	// Count events after
	var eventsAfter int
	db.Raw("SELECT COUNT(*) FROM outbox").Scan(&eventsAfter)

	// Verify no events published
	assert.Equal(t, eventsBefore, eventsAfter,
		"should not publish events when no offers expired")

	t.Log("✓ No events published for non-expired offers")
}

// TestExpireOffers_BoundaryConditions tests edge cases around expiry time
func TestExpireOffers_BoundaryConditions(t *testing.T) {
	handler, db, cleanup := setupOfferExpiryTest(t)
	defer cleanup()

	ctx := context.Background()

	now := time.Now()

	testCases := []struct {
		name         string
		expiresAt    time.Time
		shouldExpire bool
	}{
		{
			name:         "expires exactly now",
			expiresAt:    now,
			shouldExpire: true, // Should be expired (not strictly greater)
		},
		{
			name:         "expires 1 second ago",
			expiresAt:    now.Add(-1 * time.Second),
			shouldExpire: true,
		},
		{
			name:         "expires 1 second from now",
			expiresAt:    now.Add(1 * time.Second),
			shouldExpire: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			offerID := uuid.New().String()

			offer := &domain.BookingOffer{
				ID:        offerID,
				BookingID: uuid.New().String(),
				ArtisanID: uuid.New().String(),
				Status:    domain.OfferPending,
				OfferedBy: uuid.New().String(),
				OfferedAt: now.Add(-1 * time.Hour),
				ExpiresAt: tc.expiresAt,
				CreatedAt: now.Add(-1 * time.Hour),
				UpdatedAt: now.Add(-1 * time.Hour),
			}
			createTestOffer(t, db, offer)

			// Run expiry worker
			err := handler.ExpireOffers(ctx)
			require.NoError(t, err)

			// Verify status
			var status string
			db.Raw("SELECT status FROM booking_offers WHERE id = ?", offerID).Scan(&status)

			if tc.shouldExpire {
				assert.Equal(t, string(domain.OfferExpired), status,
					"%s: should be expired", tc.name)
			} else {
				assert.Equal(t, string(domain.OfferPending), status,
					"%s: should remain pending", tc.name)
			}

			t.Logf("✓ %s: verified correctly", tc.name)
		})
	}
}

// Mock publisher for testing
type mockOfferPublisher struct {
	publishedEvents []*domain.BookingEvent
}

func (m *mockOfferPublisher) PublishStatusEvent(ctx context.Context, event *domain.BookingEvent) {
	m.publishedEvents = append(m.publishedEvents, event)
}

func (m *mockOfferPublisher) PublishCreatedEvent(ctx context.Context, event *domain.BookingEvent)   {}
func (m *mockOfferPublisher) PublishCancelledEvent(ctx context.Context, event *domain.BookingEvent) {}
func (m *mockOfferPublisher) PublishOfferCreatedEvent(ctx context.Context, event *domain.BookingEvent) {
}
func (m *mockOfferPublisher) PublishAssignedEvent(ctx context.Context, event *domain.BookingEvent) {}
func (m *mockOfferPublisher) PublishOfferRejectedEvent(ctx context.Context, event *domain.BookingEvent) {
}
func (m *mockOfferPublisher) PublishRematchRequestedEvent(ctx context.Context, event *domain.RematchEvent) {
}
