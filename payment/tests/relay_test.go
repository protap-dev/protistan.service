package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	eventscommon "encore.app/core/events"
	corerelay "encore.app/core/relay"
	"encore.app/core/repository"
	"encore.app/payment/relay"
)

func TestPaymentRelayComponents(t *testing.T) {
	publisher := &relay.PaymentPublisher{}
	processor := &corerelay.DefaultProcessor{}
	cleanup := &corerelay.DefaultCleanup{}

	var _ corerelay.Publisher[relay.PaymentEvent] = publisher
	var _ corerelay.Processor = processor
	var _ corerelay.Cleanup = cleanup

	config := corerelay.DefaultConfig()
	if config.PollingInterval <= 0 {
		t.Fatal("default relay config must set a polling interval")
	}

	if relay.NewRelay(nil, config) == nil {
		t.Fatal("NewRelay should return a relay instance")
	}
}

func TestPaymentEventUnmarshalsExistingOutboxEnvelope(t *testing.T) {
	now := time.Now().UTC()
	payload := eventscommon.EventEnvelope[eventscommon.PaymentEvent]{
		EventID:       "evt-1",
		EventType:     "payment.v1.confirmed",
		OccurredAt:    now,
		CorrelationID: "booking-1",
		Producer:      "payment-service",
		Data: eventscommon.PaymentEvent{
			TransactionID:  "txn-1",
			BookingID:      "booking-1",
			QuoteID:        "quote-1",
			CustomerID:     "customer-1",
			Status:         "successful",
			PreviousStatus: "pending",
			Provider:       "nomba",
			Amount:         1000,
			Currency:       "NGN",
			InternalRef:    "TXN-123",
			Timestamp:      now,
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	var event relay.PaymentEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatalf("unmarshal payment event: %v", err)
	}

	if event.EventType() != "payment.v1.confirmed" {
		t.Fatalf("unexpected event type: %s", event.EventType())
	}
	if event.Data.BookingID != "booking-1" {
		t.Fatalf("unexpected booking id: %s", event.Data.BookingID)
	}
}

func TestPaymentPublisherIgnoresUnknownTopics(t *testing.T) {
	publisher := &relay.PaymentPublisher{}
	event := relay.PaymentEvent{
		EventEnvelope: eventscommon.EventEnvelope[eventscommon.PaymentEvent]{
			EventType: "payment.v1.unknown",
		},
	}
	outboxEvent := &repository.OutboxEvent{
		ID:    "outbox-1",
		Topic: "payment-v1-unknown",
	}

	if err := publisher.PublishToTopic(context.Background(), outboxEvent, event); err != nil {
		t.Fatalf("unknown topics should be ignored: %v", err)
	}
}
