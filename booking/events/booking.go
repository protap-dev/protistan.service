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

//encore:topic booking.offered
var OfferedTopic = pubsub.NewTopic[*domain.BookingEvent]("booking-offered", pubsub.TopicConfig{
	DeliveryGuarantee: pubsub.AtLeastOnce,
})

//encore:topic booking.assigned
var AssignedTopic = pubsub.NewTopic[*domain.BookingEvent]("booking-assigned", pubsub.TopicConfig{
	DeliveryGuarantee: pubsub.AtLeastOnce,
})

//encore:topic booking.quote.accepted
var QuoteAcceptedTopic = pubsub.NewTopic[*domain.BookingEvent]("booking-quote-accepted", pubsub.TopicConfig{
	DeliveryGuarantee: pubsub.AtLeastOnce,
})

//encore:topic booking.quote.rejected
var QuoteRejectedTopic = pubsub.NewTopic[*domain.BookingEvent]("booking-quote-rejected", pubsub.TopicConfig{
	DeliveryGuarantee: pubsub.AtLeastOnce,
})

//encore:topic booking.payment.confirmed
var PaymentConfirmedTopic = pubsub.NewTopic[*domain.BookingEvent]("booking-payment-confirmed", pubsub.TopicConfig{
	DeliveryGuarantee: pubsub.AtLeastOnce,
})
