package quote

import (
	"context"
	"time"

	eventscommon "encore.app/core/events"
	"encore.dev/pubsub"
)

//encore:service
type Service struct{}

// QuoteRequest represents a quote request
type QuoteRequest struct {
	BookingID             string `json:"booking_id"`
	EstimatedDurationMins int    `json:"estimated_duration_mins"`
	ServiceCategoryID     string `json:"service_category_id"`
}

// AcceptQuoteRequest represents accepting a quote
type AcceptQuoteRequest struct {
	BookingID string `json:"booking_id"`
}

// RejectQuoteRequest represents rejecting a quote
type RejectQuoteRequest struct {
	BookingID string  `json:"booking_id"`
	Reason    *string `json:"reason,omitempty"`
}

// QuoteResponse represents a quote
type QuoteResponse struct {
	ID        string  `json:"id"`
	BookingID string  `json:"booking_id"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
	Status    string  `json:"status"`
}

// Topics - Quote service OWNS these topics
var QuoteProposedTopic = pubsub.NewTopic[*eventscommon.EventEnvelope[eventscommon.BookingEvent]]("quote-v1-proposed", pubsub.TopicConfig{
	DeliveryGuarantee: pubsub.AtLeastOnce,
})

var QuoteAcceptedTopic = pubsub.NewTopic[*eventscommon.EventEnvelope[eventscommon.BookingEvent]]("quote-v1-accepted", pubsub.TopicConfig{
	DeliveryGuarantee: pubsub.AtLeastOnce,
})

var QuoteRejectedTopic = pubsub.NewTopic[*eventscommon.EventEnvelope[eventscommon.BookingEvent]]("quote-v1-rejected", pubsub.TopicConfig{
	DeliveryGuarantee: pubsub.AtLeastOnce,
})

// ProposeQuote creates and proposes a quote for a booking
//
//encore:api auth method=POST path=/v1/quotes
func ProposeQuote(ctx context.Context, req *QuoteRequest) (*QuoteResponse, error) {
	time.Sleep(1 * time.Second)

	quoteID := generateID()
	amount := calculateQuoteAmount(req.EstimatedDurationMins)

	quote := &QuoteResponse{
		ID:        quoteID,
		BookingID: req.BookingID,
		Amount:    amount,
		Currency:  "NGN",
		Status:    "proposed",
	}

	// Publish event using shared types
	envelope := &eventscommon.EventEnvelope[eventscommon.BookingEvent]{
		EventID:       eventscommon.GenerateEventID(),
		EventType:     "quote.v1.proposed",
		OccurredAt:    time.Now(),
		CorrelationID: req.BookingID,
		Producer:      "quote-service",
		Data: eventscommon.BookingEvent{
			BookingID:      req.BookingID,
			QuoteID:        quoteID,
			Amount:         amount,
			Status:         "quote_proposed",
			PreviousStatus: "pending_quote",
			Timestamp:      time.Now(),
			UserID:         "system",
		},
	}

	_, err := QuoteProposedTopic.Publish(ctx, envelope)
	if err != nil {
		return nil, err
	}

	return quote, nil
}

// AcceptQuote accepts a proposed quote
//
//encore:api auth method=POST path=/v1/quotes/:id/accept
func AcceptQuote(ctx context.Context, id string, req *AcceptQuoteRequest) (*QuoteResponse, error) {
	time.Sleep(500 * time.Millisecond)

	envelope := &eventscommon.EventEnvelope[eventscommon.BookingEvent]{
		EventID:       eventscommon.GenerateEventID(),
		EventType:     "quote.v1.accepted",
		OccurredAt:    time.Now(),
		CorrelationID: req.BookingID,
		Producer:      "quote-service",
		Data: eventscommon.BookingEvent{
			BookingID:      req.BookingID,
			QuoteID:        id,
			Status:         "quote_accepted",
			PreviousStatus: "quote_proposed",
			Timestamp:      time.Now(),
			UserID:         getCurrentUserID(ctx),
		},
	}

	_, err := QuoteAcceptedTopic.Publish(ctx, envelope)
	if err != nil {
		return nil, err
	}

	return &QuoteResponse{
		ID:       id,
		Status:   "accepted",
		Currency: "NGN",
	}, nil
}

// RejectQuote rejects a proposed quote
//
//encore:api auth method=POST path=/v1/quotes/:id/reject
func RejectQuote(ctx context.Context, id string, req *RejectQuoteRequest) (*QuoteResponse, error) {
	time.Sleep(500 * time.Millisecond)

	envelope := &eventscommon.EventEnvelope[eventscommon.BookingEvent]{
		EventID:       eventscommon.GenerateEventID(),
		EventType:     "quote.v1.rejected",
		OccurredAt:    time.Now(),
		CorrelationID: req.BookingID,
		Producer:      "quote-service",
		Data: eventscommon.BookingEvent{
			BookingID:      req.BookingID,
			QuoteID:        id,
			Status:         "quote_rejected",
			PreviousStatus: "quote_proposed",
			Timestamp:      time.Now(),
			UserID:         getCurrentUserID(ctx),
			Reason:         req.Reason,
		},
	}

	_, err := QuoteRejectedTopic.Publish(ctx, envelope)
	if err != nil {
		return nil, err
	}

	return &QuoteResponse{
		ID:     id,
		Status: "rejected",
	}, nil
}

// Helper functions
func generateID() string {
	return "mock-" + time.Now().Format("20060102150405")
}

func calculateQuoteAmount(durationMins int) float64 {
	baseRate := 5000.0
	hours := float64(durationMins) / 60.0
	return baseRate * hours
}

func getCurrentUserID(ctx context.Context) string {
	return "user-from-context"
}
