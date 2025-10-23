package topics_quote

import (
	eventscommon "encore.app/core/events"
	"encore.dev/pubsub"
)

// Quote Event Topics - Shared between services to avoid import cycles
var QuoteProposedTopic = pubsub.NewTopic[*eventscommon.EventEnvelope[eventscommon.BookingEvent]]("quote-v1-proposed", pubsub.TopicConfig{
	DeliveryGuarantee: pubsub.AtLeastOnce,
})

var QuoteAcceptedTopic = pubsub.NewTopic[*eventscommon.EventEnvelope[eventscommon.BookingEvent]]("quote-v1-accepted", pubsub.TopicConfig{
	DeliveryGuarantee: pubsub.AtLeastOnce,
})

var QuoteRejectedTopic = pubsub.NewTopic[*eventscommon.EventEnvelope[eventscommon.BookingEvent]]("quote-v1-rejected", pubsub.TopicConfig{
	DeliveryGuarantee: pubsub.AtLeastOnce,
})
