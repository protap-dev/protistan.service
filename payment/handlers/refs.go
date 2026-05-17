package handlers

import (
	"fmt"

	eventscommon "encore.app/core/events"
)

func (h *PaymentsHandler) generateInternalRef(bookingID string) string {
	return fmt.Sprintf("TXN-%s-%s", bookingID, eventscommon.GenerateUUID())
}

func sanitizeProviderError(err error) string {
	if err == nil {
		return ""
	}
	return "provider_initialization_failed"
}

func redactPaymentRef(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "***"
	}
	return value[:4] + "..." + value[len(value)-4:]
}
