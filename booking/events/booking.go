package events

import (
	"encore.app/booking/domain"
	"encore.dev/pubsub"
)

// Topics that BOOKING SERVICE owns and publishes
var (
	// StatusTopic publishes all booking status change events
	StatusTopic = pubsub.NewTopic[*EventEnvelope[domain.BookingEvent]]("booking-status", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})

	// CreatedTopic publishes booking creation events
	CreatedTopic = pubsub.NewTopic[*EventEnvelope[domain.BookingEvent]]("booking-created", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})

	// CancelledTopic publishes booking cancellation events
	CancelledTopic = pubsub.NewTopic[*EventEnvelope[domain.BookingEvent]]("booking-cancelled", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})

	// OfferedTopic publishes booking offer events
	OfferedTopic = pubsub.NewTopic[*EventEnvelope[domain.BookingEvent]]("booking-offered", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})

	// AssignedTopic publishes booking assignment events
	AssignedTopic = pubsub.NewTopic[*EventEnvelope[domain.BookingEvent]]("booking-assigned", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})

	// OfferRejectedTopic publishes offer rejection events
	OfferRejectedTopic = pubsub.NewTopic[*EventEnvelope[domain.BookingEvent]]("booking-offer-rejected", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})

	// OfferExpiredTopic publishes offer expiration events
	OfferExpiredTopic = pubsub.NewTopic[*EventEnvelope[domain.BookingEvent]]("booking-offer-expired", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})

	// RematchTopic publishes rematch request events
	RematchTopic = pubsub.NewTopic[*EventEnvelope[domain.RematchEvent]]("booking-rematch", pubsub.TopicConfig{
		DeliveryGuarantee: pubsub.AtLeastOnce,
	})
)
