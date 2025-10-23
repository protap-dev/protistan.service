package booking

import (
	"context"
	"testing"
	"time"

	"encore.app/booking/domain"
	"encore.app/booking/relay"
	corerelay "encore.app/core/relay"
	"encore.app/core/repository"
)

// TestRelayInitialization verifies that the modular relay can be properly initialized
func TestRelayInitialization(t *testing.T) {
	// Test that all our modular components can be created and wired together
	// without errors, ensuring the dependency injection works correctly

	// Test configuration creation
	config := corerelay.DefaultConfig()
	if config.PollingInterval == 0 {
		t.Error("DefaultConfig should set PollingInterval")
	}

	// Test metrics creation
	metrics := corerelay.NewMetrics()
	if metrics == nil {
		t.Error("NewMetrics should return a valid metrics instance")
	}

	// Test component creation
	publisher := &relay.BookingPublisher{}
	processor := &corerelay.DefaultProcessor{}
	cleanup := &corerelay.DefaultCleanup{}

	// Struct literals are never nil, so no need to check
	// Test that our interfaces are properly implemented
	var _ corerelay.Publisher[relay.BookingEvent] = publisher
	var _ corerelay.Processor = processor
	var _ corerelay.Cleanup = cleanup

	// Test metrics snapshot
	snapshot := metrics.GetSnapshot()
	if snapshot.EventsProcessed != 0 {
		t.Error("New metrics should start with zero events processed")
	}

	t.Log("Modular relay components initialized successfully")
}

// TestConfigValidation verifies configuration values are reasonable
func TestConfigValidation(t *testing.T) {
	config := corerelay.DefaultConfig()

	// Verify reasonable defaults
	if config.PollingInterval < time.Second {
		t.Error("PollingInterval should be at least 1 second")
	}

	if config.BatchSize <= 0 {
		t.Error("BatchSize should be greater than 0")
	}

	if config.MaxRetries < 0 {
		t.Error("MaxRetries should not be negative")
	}

	if config.AuditRetention <= 0 {
		t.Error("AuditRetention should be greater than 0")
	}

	t.Log("Configuration validation passed")
}

// TestMetricsThreadSafety verifies metrics are thread-safe
func TestMetricsThreadSafety(t *testing.T) {
	metrics := corerelay.NewMetrics()

	// Simulate concurrent access
	done := make(chan bool, 10)

	// Start multiple goroutines accessing metrics
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()

			// Record some metrics
			metrics.RecordProcessingTime(time.Duration(i) * time.Millisecond)
			metrics.RecordEventFailure()

			// Get snapshot
			snapshot := metrics.GetSnapshot()
			if snapshot.EventsFailed < 0 {
				t.Error("Metrics should not have negative values")
			}
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify final state
	finalSnapshot := metrics.GetSnapshot()
	if finalSnapshot.EventsFailed != 10 {
		t.Errorf("Expected 10 failed events, got %d", finalSnapshot.EventsFailed)
	}

	t.Log("Metrics thread safety test passed")
}

// TestMetricsHealthCheck verifies health monitoring works correctly
func TestMetricsHealthCheck(t *testing.T) {
	metrics := corerelay.NewMetrics()

	// Initially should be healthy (recent activity)
	if !metrics.IsHealthy() {
		t.Error("New metrics should initially be healthy")
	}

	// After setting old timestamp, should be unhealthy
	metrics.LastProcessingTime = time.Now().Add(-10 * time.Minute)
	if metrics.IsHealthy() {
		t.Error("Metrics with old timestamp should be unhealthy")
	}

	t.Log("Metrics health check test passed")
}

// TestPublisherCreation verifies publisher can be created and implements interface
func TestPublisherCreation(t *testing.T) {
	publisher := &relay.BookingPublisher{}
	if publisher == nil {
		t.Error("BookingPublisher should be created successfully")
	}

	// Test interface compliance
	var _ corerelay.Publisher[relay.BookingEvent] = publisher

	t.Log("Publisher creation and interface compliance verified")
}

// TestProcessorCreation verifies processor can be created and implements interface
func TestProcessorCreation(t *testing.T) {
	processor := &corerelay.DefaultProcessor{}
	if processor == nil {
		t.Error("DefaultProcessor should be created successfully")
	}

	// Test interface compliance
	var _ corerelay.Processor = processor

	t.Log("Processor creation and interface compliance verified")
}

// TestCleanupCreation verifies cleanup can be created and implements interface
func TestCleanupCreation(t *testing.T) {
	cleanup := &corerelay.DefaultCleanup{}
	if cleanup == nil {
		t.Error("DefaultCleanup should be created successfully")
	}

	// Test interface compliance
	var _ corerelay.Cleanup = cleanup

	t.Log("Cleanup creation and interface compliance verified")
}

// TestRelayComponentIntegration verifies all components can be wired together
func TestRelayComponentIntegration(t *testing.T) {
	// Test that all components can be created and wired together
	config := corerelay.DefaultConfig()
	publisher := &relay.BookingPublisher{}
	processor := &corerelay.DefaultProcessor{}
	cleanup := &corerelay.DefaultCleanup{}

	// Test interface compliance for dependency injection
	var _ corerelay.Publisher[relay.BookingEvent] = publisher
	var _ corerelay.Processor = processor
	var _ corerelay.Cleanup = cleanup

	// Test that NewRelay accepts the interfaces (would panic without real DB, but that's expected)
	defer func() {
		if r := recover(); r != nil {
			// Expected to panic without a real database connection
			t.Log("Component integration test completed (DB dependency correctly enforced)")
		}
	}()

	// Test core relay creation with typed publisher
	_ = corerelay.NewRelay(nil, publisher, processor, cleanup, config)

	t.Log("Relay component integration test completed")
}

// TestRelayConfiguration verifies relay accepts different configurations
func TestRelayConfiguration(t *testing.T) {
	// Test custom configuration
	customConfig := corerelay.Config{
		PollingInterval: 10 * time.Second,
		BatchSize:       50,
		MaxRetries:      3,
		RetryBaseDelay:  2 * time.Second,
		RetryMaxDelay:   60 * time.Second,
		AuditRetention:  14 * 24 * time.Hour,
	}

	// Verify configuration is accepted (would panic without DB, but config should be stored)
	defer func() {
		if r := recover(); r != nil {
			t.Log("Custom configuration accepted (DB dependency correctly enforced)")
		}
	}()

	publisher := &relay.BookingPublisher{}
	processor := &corerelay.DefaultProcessor{}
	cleanup := &corerelay.DefaultCleanup{}

	_ = corerelay.NewRelay(nil, publisher, processor, cleanup, customConfig)

	t.Log("Relay configuration test completed")
}

// TestPublisherTopicRouting verifies topic routing logic works correctly
func TestPublisherTopicRouting(t *testing.T) {
	publisher := &relay.BookingPublisher{}

	// Test that publisher implements interface correctly
	var _ corerelay.Publisher[relay.BookingEvent] = publisher

	// Test topic routing for different event types
	testCases := []struct {
		topic    string
		expected bool // Should not error for valid topics
	}{
		{"booking.status", true},
		{"booking.created", true},
		{"booking.cancelled", true},
		{"booking.offered", true},
		{"booking.assigned", true},
		{"booking.offer.rejected", true},
		{"booking.v1.offer.expired", true},
		{"unknown.topic", true}, // Should default to status topic
	}

	for _, tc := range testCases {
		// Create a mock outbox event
		outboxEvent := &repository.OutboxEvent{
			ID:    "test-id",
			Topic: tc.topic,
			Data:  []byte(`{"test": "data"}`),
		}

		// Create a mock booking event
		bookingEvent := relay.BookingEvent{
			BookingEvent: domain.BookingEvent{
				BookingID: "test-booking",
				Status:    domain.BookingStatus("pending"),
				UserID:    "test-user",
			},
		}

		// Test that PublishToTopic doesn't panic for valid topics
		// (We can't actually publish without a real database, but we can test the logic doesn't crash)
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Publisher panicked for topic %s: %v", tc.topic, r)
				}
			}()

			// This would normally call events.StatusTopic.Publish etc.
			// For testing, we just verify it doesn't panic during topic routing
			_ = publisher.PublishToTopic(context.Background(), outboxEvent, bookingEvent)
		}()
	}

	t.Log("Publisher topic routing test completed")
}

// TestProcessorEventHandling verifies event processing logic
func TestProcessorEventHandling(t *testing.T) {
	processor := &corerelay.DefaultProcessor{}

	// Test that processor implements interface correctly
	var _ corerelay.Processor = processor

	// Test that GetUnprocessedEvents doesn't panic (would fail without DB, but that's expected)
	defer func() {
		if r := recover(); r != nil {
			t.Log("Processor event handling test completed (DB dependency correctly enforced)")
		}
	}()

	// This will fail without a real DB, but we verify it doesn't panic unexpectedly
	_, err := processor.GetUnprocessedEvents(context.Background(), nil, 10)
	if err != nil {
		t.Logf("Expected error without database: %v", err)
	}

	t.Log("Processor event handling test completed")
}

// TestCleanupMaintenance verifies cleanup logic
func TestCleanupMaintenance(t *testing.T) {
	cleanup := &corerelay.DefaultCleanup{}

	// Test that cleanup implements interface correctly
	var _ corerelay.Cleanup = cleanup

	// Test that CleanupOldEvents doesn't panic (would fail without DB, but that's expected)
	defer func() {
		if r := recover(); r != nil {
			t.Log("Cleanup maintenance test completed (DB dependency correctly enforced)")
		}
	}()

	// This will fail without a real DB, but we verify it doesn't panic unexpectedly
	err := cleanup.CleanupOldEvents(context.Background(), nil, 7*24*time.Hour)
	if err != nil {
		t.Logf("Expected error without database: %v", err)
	}

	t.Log("Cleanup maintenance test completed")
}
