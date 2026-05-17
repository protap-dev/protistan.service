package booking

import (
	"testing"
	"time"

	"encore.app/booking/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyPaymentReservationStoresOpaqueReservation(t *testing.T) {
	now := time.Now().UTC().Add(10 * time.Minute).Truncate(time.Second)
	booking := &domain.Booking{}

	domain.ApplyPaymentReservation(booking, domain.PaymentReservation{
		QuoteID:        "quote-1",
		ReservationKey: "reservation-1",
		ReservedBy:     "customer-1",
		ReservedUntil:  now,
	})

	reservation, ok := domain.PaymentReservationFromBooking(booking)
	require.True(t, ok)
	assert.Equal(t, "quote-1", reservation.QuoteID)
	assert.Equal(t, "reservation-1", reservation.ReservationKey)
	assert.Equal(t, "customer-1", reservation.ReservedBy)
	assert.Equal(t, now, reservation.ReservedUntil)
}

func TestPaymentReservationMatchesOnlySameQuoteAndKey(t *testing.T) {
	booking := &domain.Booking{}
	domain.ApplyPaymentReservation(booking, domain.PaymentReservation{
		QuoteID:        "quote-1",
		ReservationKey: "reservation-1",
		ReservedBy:     "customer-1",
		ReservedUntil:  time.Now().UTC().Add(time.Minute),
	})

	assert.True(t, domain.PaymentReservationMatches(booking, "quote-1", "reservation-1"))
	assert.False(t, domain.PaymentReservationMatches(booking, "quote-2", "reservation-1"))
	assert.False(t, domain.PaymentReservationMatches(booking, "quote-1", "reservation-2"))
}

func TestPaymentReservationMatchesRejectsEmptyInput(t *testing.T) {
	booking := &domain.Booking{Metadata: map[string]string{}}

	assert.False(t, domain.PaymentReservationMatches(booking, "", ""))

	domain.ApplyPaymentReservation(booking, domain.PaymentReservation{
		QuoteID:        "quote-1",
		ReservationKey: "reservation-1",
		ReservedBy:     "customer-1",
		ReservedUntil:  time.Now().UTC().Add(time.Minute),
	})

	assert.False(t, domain.PaymentReservationMatches(booking, "", "reservation-1"))
	assert.False(t, domain.PaymentReservationMatches(booking, "quote-1", ""))
}

func TestPaymentReservationExpired(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	booking := &domain.Booking{}

	domain.ApplyPaymentReservation(booking, domain.PaymentReservation{
		QuoteID:        "quote-1",
		ReservationKey: "reservation-1",
		ReservedBy:     "customer-1",
		ReservedUntil:  now.Add(-time.Second),
	})
	assert.True(t, domain.PaymentReservationExpired(booking, now))

	domain.ApplyPaymentReservation(booking, domain.PaymentReservation{
		QuoteID:        "quote-1",
		ReservationKey: "reservation-1",
		ReservedBy:     "customer-1",
		ReservedUntil:  now.Add(time.Minute),
	})
	assert.False(t, domain.PaymentReservationExpired(booking, now))
}

func TestClearPaymentReservationRemovesReservationMetadata(t *testing.T) {
	booking := &domain.Booking{}
	domain.ApplyPaymentReservation(booking, domain.PaymentReservation{
		QuoteID:        "quote-1",
		ReservationKey: "reservation-1",
		ReservedBy:     "customer-1",
		ReservedUntil:  time.Now().UTC().Add(time.Minute),
	})

	domain.ClearPaymentReservation(booking)

	_, ok := domain.PaymentReservationFromBooking(booking)
	assert.False(t, ok)
	assert.False(t, domain.PaymentReservationMatches(booking, "quote-1", "reservation-1"))
}
