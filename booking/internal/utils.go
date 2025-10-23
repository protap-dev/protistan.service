package internal

import (
	"crypto/rand"
	"encoding/hex"
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

// GenerateUUID generates a new UUID using Google UUID library
func GenerateUUID() string {
	newuid, _ := uuid.NewV7()
	return newuid.String()
}

// GenerateRandomID generates a random 16-byte ID encoded as hex string
// This is used for correlation/causation IDs and event IDs
func GenerateRandomID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// GenerateEventID is an alias for GenerateRandomID for backward compatibility
func GenerateEventID() string {
	return GenerateRandomID()
}
