package topics_payment

import (
	eventscommon "encore.app/core/events"
	"encore.dev/pubsub"
)

var PaymentConfirmedTopic = pubsub.NewTopic[*eventscommon.EventEnvelope[eventscommon.BookingEvent]]("payment-v1-confirmed", pubsub.TopicConfig{
	DeliveryGuarantee: pubsub.AtLeastOnce,
})

var PaymentFailedTopic = pubsub.NewTopic[*eventscommon.EventEnvelope[eventscommon.BookingEvent]]("payment-v1-failed", pubsub.TopicConfig{
	DeliveryGuarantee: pubsub.AtLeastOnce,
})
