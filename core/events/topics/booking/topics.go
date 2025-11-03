package topics_booking

import (
	eventscommon "encore.app/core/events"
	"encore.dev/pubsub"
)

var BookingAssignedTopic = pubsub.NewTopic[eventscommon.EventEnvelope[eventscommon.BookingEvent]](
	"booking-assigned",
	pubsub.TopicConfig{DeliveryGuarantee: pubsub.AtLeastOnce},
)
