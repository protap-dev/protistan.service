package relay

import "time"

// Config holds configuration for the outbox relay
type Config struct {
	PollingInterval time.Duration // How often to poll for events
	BatchSize       int           // Number of events to process per batch
	MaxRetries      int           // Maximum retry attempts for failed publishes
	RetryBaseDelay  time.Duration // Base delay for exponential backoff
	RetryMaxDelay   time.Duration // Maximum delay for exponential backoff
	AuditRetention  time.Duration // How long to keep processed events for audit
}

// DefaultConfig returns production-ready default configuration
func DefaultConfig() Config {
	return Config{
		PollingInterval: 5 * time.Second,
		BatchSize:       100,
		MaxRetries:      5,
		RetryBaseDelay:  1 * time.Second,
		RetryMaxDelay:   30 * time.Second,
		AuditRetention:  7 * 24 * time.Hour, // Keep for 7 days
	}
}
