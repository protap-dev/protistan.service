package events

import (
	"encore.app/booking/domain"
	eventscommon "encore.app/core/events"
	"encore.dev/pubsub"
)

// Topics that BOOKING SERVICE owns and publishes
var (
	// CancelledTopic publishes booking cancellation events
	CancelledTopic = pubsub.NewTopic[eventscommon.EventEnvelope[domain.BookingEvent]]("booking-cancelled", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})

	// OfferedTopic publishes booking offer events
	OfferedTopic = pubsub.NewTopic[eventscommon.EventEnvelope[domain.BookingEvent]]("booking-offered", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})

	// RematchTopic publishes rematch request events
	RematchTopic = pubsub.NewTopic[eventscommon.EventEnvelope[domain.RematchEvent]]("booking-rematch", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})
)
