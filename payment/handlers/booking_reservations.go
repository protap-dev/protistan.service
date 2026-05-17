package handlers

import (
	"context"
	"fmt"
	"time"

	"encore.app/booking"
	bookinghandlers "encore.app/booking/handlers"
	eventscommon "encore.app/core/events"
	pdomain "encore.app/payment/domain"
	"encore.app/quote"
	"encore.dev/beta/errs"
)

func (h *PaymentsHandler) reserveBookingForPayment(ctx context.Context, status any, txn *pdomain.Transaction, userID, reservationKey string, reservedUntil time.Time) error {
	statusValue := fmt.Sprint(status)
	if statusValue != bookingStatusQuoteAccepted && statusValue != bookingStatusPaymentPending {
		err := errs.B().Code(errs.FailedPrecondition).Msgf("booking is in incompatible state %s for pending transaction", statusValue).Err()
		h.markBookingRepairRequired(ctx, txn, err)
		return err
	}

	if _, err := h.moveBookingToPaymentPending(ctx, txn.BookingID, &booking.MarkPaymentPendingRequest{
		UserID:         userID,
		QuoteID:        txn.QuoteID,
		ReservationKey: reservationKey,
		ReservedUntil:  reservedUntil,
	}); err != nil {
		h.markBookingRepairRequired(ctx, txn, err)
		return errs.B().Code(errs.FailedPrecondition).Msg("payment initialized but booking state repair is required; retry initialize to repair").Err()
	}

	h.clearBookingRepairMetadata(ctx, txn)
	return nil
}

func (h *PaymentsHandler) releaseBookingReservationAfterFailure(ctx context.Context, txn *pdomain.Transaction, userID, reservationKey, reasonText string) {
	reason := reasonText
	if _, err := h.releaseBookingReservation(ctx, txn.BookingID, &booking.ReleasePaymentReservationRequest{
		UserID:         userID,
		QuoteID:        txn.QuoteID,
		ReservationKey: reservationKey,
		Reason:         &reason,
	}); err != nil {
		h.markBookingRepairRequired(ctx, txn, err)
	}
}

func (h *PaymentsHandler) validateRetryBookingState(status any) error {
	statusValue := fmt.Sprint(status)
	switch statusValue {
	case bookingStatusPaymentPending, bookingStatusQuoteAccepted:
		return nil
	default:
		return errs.B().Code(errs.FailedPrecondition).Msgf("existing pending transaction cannot be retried while booking is %s", statusValue).Err()
	}
}

func (h *PaymentsHandler) ensurePaymentReservationKey(txn *pdomain.Transaction) (string, bool) {
	if txn.Metadata == nil {
		txn.Metadata = map[string]string{}
	}
	if key := txn.Metadata[paymentReservationKeyMeta]; key != "" {
		return key, false
	}
	key := eventscommon.GenerateUUID()
	txn.Metadata[paymentReservationKeyMeta] = key
	return key, true
}

func (h *PaymentsHandler) markBookingRepairRequired(ctx context.Context, txn *pdomain.Transaction, repairErr error) {
	if txn.Metadata == nil {
		txn.Metadata = map[string]string{}
	}
	txn.Metadata["booking_state_repair_required"] = "true"
	txn.Metadata["booking_state_repair_error"] = repairErr.Error()
	txn.Metadata["booking_state_repair_policy"] = "cancel_existing_checkout_and_retry_initialize"
	txn.UpdatedAt = time.Now()
	_ = h.repo.Update(ctx, txn)
}

func (h *PaymentsHandler) clearBookingRepairMetadata(ctx context.Context, txn *pdomain.Transaction) {
	if txn.Metadata == nil {
		return
	}

	if _, exists := txn.Metadata["booking_state_repair_required"]; !exists {
		return
	}

	delete(txn.Metadata, "booking_state_repair_required")
	delete(txn.Metadata, "booking_state_repair_error")
	delete(txn.Metadata, "booking_state_repair_policy")
	txn.UpdatedAt = time.Now()
	_ = h.repo.Update(ctx, txn)
}

func (h *PaymentsHandler) fetchBooking(ctx context.Context, bookingID string) (*bookinghandlers.BookingResponse, error) {
	if h.getBooking != nil {
		return h.getBooking(ctx, bookingID)
	}
	return booking.GetBooking(ctx, bookingID)
}

func (h *PaymentsHandler) fetchQuoteForPayment(ctx context.Context, quoteID string) (*quote.PaymentQuoteResponse, error) {
	if h.getQuoteForPayment != nil {
		return h.getQuoteForPayment(ctx, quoteID)
	}
	return quote.GetQuoteForPayment(ctx, quoteID)
}

func (h *PaymentsHandler) moveBookingToPaymentPending(ctx context.Context, bookingID string, req *booking.MarkPaymentPendingRequest) (*booking.BookingPaymentStatusResponse, error) {
	if h.markBookingPaymentPending != nil {
		return h.markBookingPaymentPending(ctx, bookingID, req)
	}
	return booking.MarkPaymentPending(ctx, bookingID, req)
}

func (h *PaymentsHandler) releaseBookingReservation(ctx context.Context, bookingID string, req *booking.ReleasePaymentReservationRequest) (*booking.BookingPaymentStatusResponse, error) {
	if h.releaseBookingPaymentReservation != nil {
		return h.releaseBookingPaymentReservation(ctx, bookingID, req)
	}
	return booking.ReleasePaymentReservation(ctx, bookingID, req)
}
