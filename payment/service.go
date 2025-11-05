package payment

import (
	"context"
	"fmt"
	"time"

	eventscommon "encore.app/core/events"
	topics_payment "encore.app/core/events/topics/payment"
)

//encore:service
type Service struct{}

// PaymentRequest represents a payment request
type PaymentRequest struct {
	BookingID     string  `json:"booking_id"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	PaymentMethod string  `json:"payment_method"`
	CustomerID    string  `json:"customer_id"`
}

// PaymentResponse represents a payment result
type PaymentResponse struct {
	ID          string  `json:"id"`
	BookingID   string  `json:"booking_id"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	Status      string  `json:"status"`
	Reference   string  `json:"reference"`
	ProcessedAt string  `json:"processed_at"`
}

// CreatePayment processes a payment
//
//encore:api auth method=POST path=/v1/payments
func CreatePayment(ctx context.Context, req *PaymentRequest) (*PaymentResponse, error) {
	fmt.Printf("💳 Processing payment: %s for ₦%.2f\n", req.PaymentMethod, req.Amount)
	time.Sleep(2 * time.Second)

	paymentID := generateID()
	reference := fmt.Sprintf("PAY-%s", paymentID)

	success := time.Now().Unix()%20 != 0

	if success {
		response := &PaymentResponse{
			ID:          paymentID,
			BookingID:   req.BookingID,
			Amount:      req.Amount,
			Currency:    req.Currency,
			Status:      "success",
			Reference:   reference,
			ProcessedAt: time.Now().Format(time.RFC3339),
		}

		envelope := &eventscommon.EventEnvelope[eventscommon.BookingEvent]{
			EventID:       eventscommon.GenerateEventID(),
			EventType:     "payment.v1.confirmed",
			OccurredAt:    time.Now(),
			CorrelationID: req.BookingID,
			Producer:      "payment-service",
			Data: eventscommon.BookingEvent{
				BookingID:      req.BookingID,
				PaymentID:      paymentID,
				Amount:         req.Amount,
				Status:         "confirmed",
				PreviousStatus: "payment_pending",
				Timestamp:      time.Now(),
				UserID:         req.CustomerID,
			},
		}

		_, err := topics_payment.PaymentConfirmedTopic.Publish(ctx, envelope)
		if err != nil {
			return nil, err
		}

		fmt.Printf("✅ Payment confirmed: %s\n", reference)
		return response, nil
	}

	// Payment failed
	reason := "insufficient_funds"
	response := &PaymentResponse{
		ID:          paymentID,
		BookingID:   req.BookingID,
		Amount:      req.Amount,
		Currency:    req.Currency,
		Status:      "failed",
		Reference:   reference,
		ProcessedAt: time.Now().Format(time.RFC3339),
	}

	envelope := &eventscommon.EventEnvelope[eventscommon.BookingEvent]{
		EventID:       eventscommon.GenerateEventID(),
		EventType:     "payment.v1.failed",
		OccurredAt:    time.Now(),
		CorrelationID: req.BookingID,
		Producer:      "payment-service",
		Data: eventscommon.BookingEvent{
			BookingID:      req.BookingID,
			PaymentID:      paymentID,
			Amount:         req.Amount,
			Status:         "payment_pending",
			PreviousStatus: "payment_pending",
			Timestamp:      time.Now(),
			UserID:         req.CustomerID,
			Reason:         &reason,
		},
	}

	_, err := topics_payment.PaymentFailedTopic.Publish(ctx, envelope)
	if err != nil {
		return nil, err
	}

	fmt.Printf("❌ Payment failed: %s\n", reason)
	return response, nil
}

// SimulatePaymentSuccess forces a successful payment
//
//encore:api auth method=POST path=/v1/payments/simulate-success
func SimulatePaymentSuccess(ctx context.Context, req *PaymentRequest) (*PaymentResponse, error) {
	time.Sleep(1 * time.Second)

	paymentID := generateID()
	reference := fmt.Sprintf("PAY-SIM-%s", paymentID)

	response := &PaymentResponse{
		ID:          paymentID,
		BookingID:   req.BookingID,
		Amount:      req.Amount,
		Currency:    req.Currency,
		Status:      "success",
		Reference:   reference,
		ProcessedAt: time.Now().Format(time.RFC3339),
	}

	envelope := &eventscommon.EventEnvelope[eventscommon.BookingEvent]{
		EventID:       eventscommon.GenerateEventID(),
		EventType:     "payment.v1.confirmed",
		OccurredAt:    time.Now(),
		CorrelationID: req.BookingID,
		Producer:      "payment-service",
		Data: eventscommon.BookingEvent{
			BookingID:      req.BookingID,
			PaymentID:      paymentID,
			Amount:         req.Amount,
			Status:         "confirmed",
			PreviousStatus: "payment_pending",
			Timestamp:      time.Now(),
			UserID:         req.CustomerID,
		},
	}

	_, err := topics_payment.PaymentConfirmedTopic.Publish(ctx, envelope)
	return response, err
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
