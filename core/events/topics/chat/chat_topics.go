package topics_chat

import (
	chatdomain "encore.app/chat/domain"
	eventscommon "encore.app/core/events"
	"encore.dev/pubsub"
)

// Topics owned by Chat Service
var MessageSentTopic = pubsub.NewTopic[eventscommon.EventEnvelope[chatdomain.ChatEvent]](
	"chat-v1-message-sent",
	pubsub.TopicConfig{DeliveryGuarantee: pubsub.AtLeastOnce},
)

var MessageDeliveredTopic = pubsub.NewTopic[eventscommon.EventEnvelope[chatdomain.ChatEvent]](
	"chat-v1-message-delivered",
	pubsub.TopicConfig{DeliveryGuarantee: pubsub.AtLeastOnce},
)

var ThreadCreatedTopic = pubsub.NewTopic[eventscommon.EventEnvelope[chatdomain.ChatEvent]](
	"chat-v1-thread-created",
	pubsub.TopicConfig{DeliveryGuarantee: pubsub.AtLeastOnce},
)
