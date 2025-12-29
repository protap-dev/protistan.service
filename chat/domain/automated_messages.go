package domain

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	bookingdomain "encore.app/booking/domain"
)

func buildStandardMetadata(event *bookingdomain.BookingEvent) map[string]interface{} {
	metadata := map[string]interface{}{
		"booking_id": event.BookingID,
	}

	// Add artisan_id if available
	if event.ArtisanID != nil && *event.ArtisanID != "" {
		metadata["artisan_id"] = *event.ArtisanID
	}

	// Add all other metadata from event
	if event.Metadata != nil {
		for k, v := range event.Metadata {
			metadata[k] = v
		}
	}

	return metadata
}

// AutomatedMessageTemplate defines templates for system messages
type AutomatedMessageTemplate struct {
	EventType    string
	MessageType  MessageType
	ContentFunc  func(data map[string]interface{}) string
	MetadataFunc func(event *bookingdomain.BookingEvent) map[string]interface{}
}

// AutomatedMessages registry - system messages for the chat room
var AutomatedMessages = map[string]AutomatedMessageTemplate{
	// 1. Artisan Assigned - Thread creation moment
	"booking.assigned": {
		EventType:   "booking.assigned",
		MessageType: MessageTypeStatusUpdate,
		ContentFunc: func(data map[string]interface{}) string {
			return "An artisan has been assigned to this booking."
		},
		MetadataFunc: buildStandardMetadata,
	},

	// 2. Quote Proposed
	"booking.quote_proposed": {
		EventType:   "booking.quote_proposed",
		MessageType: MessageTypeStatusUpdate,
		ContentFunc: func(data map[string]interface{}) string {
			amount := safeFloat(data, "amount", 0)
			currency := safeString(data, "currency", "USD")
			return fmt.Sprintf("A quote for %s %.2f has been submitted. Review the details to proceed.",
				currency, amount)
		},
		MetadataFunc: buildStandardMetadata,
	},

	// 3. Quote Accepted
	"booking.quote_accepted": {
		EventType:   "booking.quote_accepted",
		MessageType: MessageTypeStatusUpdate,
		ContentFunc: func(data map[string]interface{}) string {
			return "The quote has been accepted. Proceeding to payment."
		},
	},

	// 4. Quote Rejected
	"booking.quote_rejected": {
		EventType:   "booking.quote_rejected",
		MessageType: MessageTypeStatusUpdate,
		ContentFunc: func(data map[string]interface{}) string {
			reason := strings.TrimSpace(safeString(data, "reason", ""))
			if reason != "" {
				return fmt.Sprintf("The quote was declined. Reason: %s", reason)
			}
			return "The quote was declined. A revised quote can be submitted."
		},
	},

	// 5. Payment Pending
	"booking.payment_pending": {
		EventType:   "booking.payment_pending",
		MessageType: MessageTypeStatusUpdate,
		ContentFunc: func(data map[string]interface{}) string {
			amount := safeFloat(data, "amount", 0)
			currency := safeString(data, "currency", "USD")
			return fmt.Sprintf("Payment of %s %.2f is required to confirm this booking.",
				currency, amount)
		},
	},

	// 6. Booking Confirmed
	"booking.confirmed": {
		EventType:   "booking.confirmed",
		MessageType: MessageTypeStatusUpdate,
		ContentFunc: func(data map[string]interface{}) string {
			if scheduledAt, ok := safeTime(data, "scheduled_at"); ok {
				return fmt.Sprintf("This booking has been confirmed for %s.",
					scheduledAt.Format("Mon, Jan 2 at 3:04 PM"))
			}
			return "This booking has been confirmed."
		},
	},

	// 7. Artisan En Route
	"booking.enroute": {
		EventType:   "booking.enroute",
		MessageType: MessageTypeStatusUpdate,
		ContentFunc: func(data map[string]interface{}) string {
			artisanName := safeString(data, "artisan_name", "The artisan")
			eta := safeInt(data, "eta_minutes", 0)
			if eta > 0 {
				return fmt.Sprintf("%s is on the way. Estimated arrival in %d minutes.",
					artisanName, eta)
			}
			return fmt.Sprintf("%s is on the way.", artisanName)
		},
	},

	// 8. Work In Progress
	"booking.in_progress": {
		EventType:   "booking.in_progress",
		MessageType: MessageTypeStatusUpdate,
		ContentFunc: func(data map[string]interface{}) string {
			return "Work on this booking has started."
		},
	},

	// 9. Work Completed
	"booking.completed": {
		EventType:   "booking.completed",
		MessageType: MessageTypeStatusUpdate,
		ContentFunc: func(data map[string]interface{}) string {
			return "Work on this booking has been completed. Please review and rate the service."
		},
	},

	// 10. Booking Cancelled
	"booking.cancelled": {
		EventType:   "booking.cancelled",
		MessageType: MessageTypeStatusUpdate,
		ContentFunc: func(data map[string]interface{}) string {
			cancelledBy := safeString(data, "cancelled_by", "the customer")
			reason := strings.TrimSpace(safeString(data, "reason", ""))
			if reason != "" {
				return fmt.Sprintf("This booking was cancelled by %s. Reason: %s", cancelledBy, reason)
			}
			return fmt.Sprintf("This booking was cancelled by %s.", cancelledBy)
		},
	},
}

// GetAutomatedMessage retrieves template by booking status
func GetAutomatedMessage(status string) (AutomatedMessageTemplate, bool) {
	eventType := fmt.Sprintf("booking.%s", status)
	template, exists := AutomatedMessages[eventType]
	return template, exists
}

// Helper accessors to safely read template data without panics
func safeString(data map[string]interface{}, key string, fallback string) string {
	if v, ok := data[key]; ok {
		switch val := v.(type) {
		case string:
			if val != "" {
				return val
			}
		case fmt.Stringer:
			str := val.String()
			if str != "" {
				return str
			}
		case []byte:
			if len(val) > 0 {
				return string(val)
			}
		default:
			str := fmt.Sprintf("%v", val)
			if str != "" && str != "<nil>" {
				return str
			}
		}
	}
	return fallback
}

func safeFloat(data map[string]interface{}, key string, fallback float64) float64 {
	if v, ok := data[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		case float32:
			return float64(val)
		case int:
			return float64(val)
		case int64:
			return float64(val)
		case uint:
			return float64(val)
		case uint64:
			return float64(val)
		case string:
			if parsed, err := strconv.ParseFloat(val, 64); err == nil {
				return parsed
			}
		case json.Number:
			if parsed, err := val.Float64(); err == nil {
				return parsed
			}
		}
	}
	return fallback
}

func safeInt(data map[string]interface{}, key string, fallback int) int {
	if v, ok := data[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case int32:
			return int(val)
		case int64:
			return int(val)
		case uint:
			return int(val)
		case uint32:
			return int(val)
		case uint64:
			return int(val)
		case float32:
			return int(val)
		case float64:
			return int(val)
		case string:
			if parsed, err := strconv.Atoi(val); err == nil {
				return parsed
			}
		}
	}
	return fallback
}

func safeTime(data map[string]interface{}, key string) (time.Time, bool) {
	if v, ok := data[key]; ok {
		switch val := v.(type) {
		case time.Time:
			if !val.IsZero() {
				return val, true
			}
		case *time.Time:
			if val != nil && !val.IsZero() {
				return *val, true
			}
		case string:
			if parsed, err := time.Parse(time.RFC3339, val); err == nil {
				return parsed, true
			}
		}
	}
	return time.Time{}, false
}
