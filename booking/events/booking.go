package events

import (
	"encore.app/booking/domain"
	"encore.dev/pubsub"
)

// Topics now publish EventEnvelope instead of raw BookingEvent
var (
	// StatusTopic publishes all booking status change events wrapped in envelope
	StatusTopic = pubsub.NewTopic[*EventEnvelope[domain.BookingEvent]]("booking-status", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})

	// CreatedTopic publishes booking creation events wrapped in envelope
	CreatedTopic = pubsub.NewTopic[*EventEnvelope[domain.BookingEvent]]("booking-created", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})

	// CancelledTopic publishes booking cancellation events wrapped in envelope
	CancelledTopic = pubsub.NewTopic[*EventEnvelope[domain.BookingEvent]]("booking-cancelled", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})

	// OfferedTopic publishes booking offer events wrapped in envelope
	OfferedTopic = pubsub.NewTopic[*EventEnvelope[domain.BookingEvent]]("booking-offered", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})

	// AssignedTopic publishes booking assignment events wrapped in envelope
	AssignedTopic = pubsub.NewTopic[*EventEnvelope[domain.BookingEvent]]("booking-assigned", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})

	// QuoteAcceptedTopic publishes quote acceptance events wrapped in envelope
	QuoteAcceptedTopic = pubsub.NewTopic[*EventEnvelope[domain.BookingEvent]]("booking-quote-accepted", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})

	// QuoteRejectedTopic publishes quote rejection events wrapped in envelope
	QuoteRejectedTopic = pubsub.NewTopic[*EventEnvelope[domain.BookingEvent]]("booking-quote-rejected", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})

	// PaymentConfirmedTopic publishes payment confirmation events wrapped in envelope
	PaymentConfirmedTopic = pubsub.NewTopic[*EventEnvelope[domain.BookingEvent]]("booking-payment-confirmed", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})

	// RematchTopic publishes rematch request events wrapped in envelope
	RematchTopic = pubsub.NewTopic[*EventEnvelope[domain.RematchEvent]]("booking-rematch", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})
)
