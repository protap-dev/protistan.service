package domain

import "time"

const (
	PaymentReservationKeyMeta       = "payment_reservation_key"
	PaymentReservationQuoteIDMeta   = "payment_quote_id"
	PaymentReservationReservedBy    = "payment_reserved_by"
	PaymentReservationReservedUntil = "payment_reserved_until"
)

type PaymentReservation struct {
	QuoteID        string
	ReservationKey string
	ReservedBy     string
	ReservedUntil  time.Time
}

func ApplyPaymentReservation(booking *Booking, reservation PaymentReservation) {
	if booking.Metadata == nil {
		booking.Metadata = map[string]string{}
	}
	booking.Metadata[PaymentReservationKeyMeta] = reservation.ReservationKey
	booking.Metadata[PaymentReservationQuoteIDMeta] = reservation.QuoteID
	booking.Metadata[PaymentReservationReservedBy] = reservation.ReservedBy
	booking.Metadata[PaymentReservationReservedUntil] = reservation.ReservedUntil.UTC().Format(time.RFC3339)
}

func ClearPaymentReservation(booking *Booking) {
	if booking.Metadata == nil {
		return
	}
	delete(booking.Metadata, PaymentReservationKeyMeta)
	delete(booking.Metadata, PaymentReservationQuoteIDMeta)
	delete(booking.Metadata, PaymentReservationReservedBy)
	delete(booking.Metadata, PaymentReservationReservedUntil)
}

func PaymentReservationMatches(booking *Booking, quoteID, reservationKey string) bool {
	if quoteID == "" || reservationKey == "" {
		return false
	}
	reservation, ok := PaymentReservationFromBooking(booking)
	return ok && reservation.QuoteID == quoteID && reservation.ReservationKey == reservationKey
}

func PaymentReservationExpired(booking *Booking, now time.Time) bool {
	reservation, ok := PaymentReservationFromBooking(booking)
	if !ok {
		return true
	}
	return !reservation.ReservedUntil.After(now)
}

func PaymentReservationFromBooking(booking *Booking) (PaymentReservation, bool) {
	if booking.Metadata == nil {
		return PaymentReservation{}, false
	}
	reservationKey := booking.Metadata[PaymentReservationKeyMeta]
	quoteID := booking.Metadata[PaymentReservationQuoteIDMeta]
	reservedUntilRaw := booking.Metadata[PaymentReservationReservedUntil]
	if reservationKey == "" || quoteID == "" || reservedUntilRaw == "" {
		return PaymentReservation{}, false
	}
	reservedUntil, err := time.Parse(time.RFC3339, reservedUntilRaw)
	if err != nil {
		return PaymentReservation{}, false
	}
	return PaymentReservation{
		QuoteID:        quoteID,
		ReservationKey: reservationKey,
		ReservedBy:     booking.Metadata[PaymentReservationReservedBy],
		ReservedUntil:  reservedUntil,
	}, true
}
