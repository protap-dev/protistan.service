package internal

import (
	"fmt"

	"github.com/google/uuid"
)

// MaxInt returns the maximum of two ints
func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// MinInt returns the minimum of two ints
func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// BookingCacheKey builds the cache key for a booking
func BookingCacheKey(id string) string {
	return fmt.Sprintf("booking:%s", id)
}

// GenerateUUID generates a new UUID for outbox events and other purposes
func GenerateUUID() string {
	return uuid.New().String()
}
