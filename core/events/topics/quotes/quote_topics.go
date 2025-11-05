package topics_quote

import (
	eventscommon "encore.app/core/events"
	"encore.dev/pubsub"
)

// Topics owned by Quote Service
var QuoteProposedTopic = pubsub.NewTopic[eventscommon.EventEnvelope[eventscommon.QuoteEvent]](
	"quote-v1-proposed",
	pubsub.TopicConfig{DeliveryGuarantee: pubsub.AtLeastOnce},
)

var QuoteAcceptedTopic = pubsub.NewTopic[eventscommon.EventEnvelope[eventscommon.QuoteEvent]](
	"quote-v1-accepted",
	pubsub.TopicConfig{DeliveryGuarantee: pubsub.AtLeastOnce},
)

var QuoteRejectedTopic = pubsub.NewTopic[eventscommon.EventEnvelope[eventscommon.QuoteEvent]](
	"quote-v1-rejected",
	pubsub.TopicConfig{DeliveryGuarantee: pubsub.AtLeastOnce},
)

var QuoteExpiredTopic = pubsub.NewTopic[eventscommon.EventEnvelope[eventscommon.QuoteEvent]](
	"quote-v1-expired",
	pubsub.TopicConfig{DeliveryGuarantee: pubsub.AtLeastOnce},
)
