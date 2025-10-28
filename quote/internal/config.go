package internal

import "time"

// ============================================================================
// CONFIGURATION MANAGEMENT
// ============================================================================

// PaginationConfig holds pagination-related configuration
type PaginationConfig struct {
	DefaultLimit int
	MaxLimit     int
}

// CacheConfig holds cache-related configuration
type CacheConfig struct {
	DefaultTTL time.Duration
	BookingTTL time.Duration
	OfferTTL   time.Duration
}

// OutboxRelayConfig holds outbox relay configuration
type OutboxRelayConfig struct {
	PollingInterval time.Duration
	BatchSize       int
	MaxRetries      int
	RetryBaseDelay  time.Duration
	RetryMaxDelay   time.Duration
	AuditRetention  time.Duration
}

// ServiceConfig holds all service-level configuration
type ServiceConfig struct {
	Pagination PaginationConfig
	Cache      CacheConfig
	Outbox     OutboxRelayConfig
}

// DefaultPaginationConfig returns production-ready pagination defaults
func DefaultPaginationConfig() PaginationConfig {
	return PaginationConfig{
		DefaultLimit: 20,
		MaxLimit:     100,
	}
}

// DefaultCacheConfig returns production-ready cache defaults
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		DefaultTTL: 30 * time.Minute,
		BookingTTL: 30 * time.Minute,
		OfferTTL:   15 * time.Minute,
	}
}

// DefaultOutboxRelayConfig returns production-ready outbox relay defaults
func DefaultOutboxRelayConfig() OutboxRelayConfig {
	return OutboxRelayConfig{
		PollingInterval: 5 * time.Second,
		BatchSize:       100,
		MaxRetries:      5,
		RetryBaseDelay:  1 * time.Second,
		RetryMaxDelay:   30 * time.Second,
		AuditRetention:  7 * 24 * time.Hour,
	}
}

// DefaultServiceConfig returns complete service configuration with all defaults
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		Pagination: DefaultPaginationConfig(),
		Cache:      DefaultCacheConfig(),
		Outbox:     DefaultOutboxRelayConfig(),
	}
}

// Global configuration instance (can be overridden for testing)
var GlobalConfig = DefaultServiceConfig()
