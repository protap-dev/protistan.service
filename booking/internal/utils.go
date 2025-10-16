package internal

import "fmt"

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
