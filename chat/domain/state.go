package domain

// MessageStatus represents the delivery status of a message
type MessageStatus string

const (
	MessageSending   MessageStatus = "sending"   // Being sent
	MessageSent      MessageStatus = "sent"      // Sent to server
	MessageDelivered MessageStatus = "delivered" // Delivered to recipient
	MessageRead      MessageStatus = "read"      // Read by recipient
	MessageFailed    MessageStatus = "failed"    // Failed to send
)

// MessageType categorizes messages
type MessageType string

const (
	MessageTypeUser         MessageType = "user"          // User-sent message
	MessageTypeSystem       MessageType = "system"        // System message
	MessageTypeStatusUpdate MessageType = "status_update" // Automated status update
)

// IsValid returns true when the message type is one of the supported values.
func (mt MessageType) IsValid() bool {
	switch mt {
	case MessageTypeUser, MessageTypeSystem, MessageTypeStatusUpdate:
		return true
	default:
		return false
	}
}

// Valid state transitions map for messages
var validTransitions = map[MessageStatus]map[MessageStatus]bool{
	MessageSending: {
		MessageSent:   true,
		MessageFailed: true,
	},
	MessageSent: {
		MessageDelivered: true,
		MessageFailed:    true,
	},
	MessageDelivered: {
		MessageRead: true,
	},
	MessageRead:   {}, // Terminal
	MessageFailed: {}, // Terminal
}

// CanTransition checks if a state transition is valid
func CanTransition(from, to MessageStatus) bool {
	transitions, exists := validTransitions[from]
	if !exists {
		return false
	}
	return transitions[to]
}

// IsTerminal checks if the status is terminal
func (ms MessageStatus) IsTerminal() bool {
	return ms == MessageRead || ms == MessageFailed
}

// IsDeliveredOrRead returns true when the status indicates the recipient has seen the message.
func (ms MessageStatus) IsDeliveredOrRead() bool {
	return ms == MessageDelivered || ms == MessageRead
}

// IsPendingDelivery returns true for messages that are still in flight to the recipient.
func (ms MessageStatus) IsPendingDelivery() bool {
	return ms == MessageSending || ms == MessageSent
}

// IsFailure returns true when the message is in a terminal failure state.
func (ms MessageStatus) IsFailure() bool {
	return ms == MessageFailed
}
