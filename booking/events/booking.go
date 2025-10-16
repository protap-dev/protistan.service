package events

import (
	"encore.app/booking/domain"
	"encore.dev/pubsub"
)

//encore:topic booking.status
var StatusTopic = pubsub.NewTopic[*domain.BookingEvent]("booking-status", pubsub.TopicConfig{
	DeliveryGuarantee: pubsub.AtLeastOnce,
})

//encore:topic booking.created
var CreatedTopic = pubsub.NewTopic[*domain.BookingEvent]("booking-created", pubsub.TopicConfig{
	DeliveryGuarantee: pubsub.AtLeastOnce,
})

//encore:topic booking.cancelled
var CancelledTopic = pubsub.NewTopic[*domain.BookingEvent]("booking-cancelled", pubsub.TopicConfig{
	DeliveryGuarantee: pubsub.AtLeastOnce,
})
