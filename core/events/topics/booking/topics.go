package topics_booking

import (
	"encore.app/booking/domain"
	eventscommon "encore.app/core/events"
	"encore.dev/pubsub"
)

var BookingAssigned = pubsub.NewTopic[eventscommon.EventEnvelope[domain.BookingEvent]](
	"booking-assigned",
	pubsub.TopicConfig{DeliveryGuarantee: pubsub.AtLeastOnce},
)

// StatusTopic publishes all booking status change events
var BookingStatus = pubsub.NewTopic[eventscommon.EventEnvelope[domain.BookingEvent]](
	"booking-status",
	pubsub.TopicConfig{DeliveryGuarantee: pubsub.AtLeastOnce},
)

// OfferRejectedTopic publishes offer rejection events
//	OfferRejectedTopic = pubsub.NewTopic[eventscommon.EventEnvelope[domain.BookingEvent]]("booking-offer-rejected", pubsub.TopicConfig{
//		DeliveryGuarantee: pubsub.AtLeastOnce,
//	})

//	// OfferExpiredTopic publishes offer expiration events
//	OfferExpiredTopic = pubsub.NewTopic[eventscommon.EventEnvelope[domain.BookingEvent]]("booking-offer-expired", pubsub.TopicConfig{
//		DeliveryGuarantee: pubsub.AtLeastOnce,
//	})
