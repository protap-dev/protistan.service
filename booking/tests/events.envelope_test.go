package booking

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"encore.app/booking/domain"
	"encore.app/booking/events"
	"encore.app/booking/internal"
	eventscommon "encore.app/core/events"
	"encore.dev/et"
	"encore.dev/pubsub"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"x.encore.dev/infra/pubsub/outbox"
)

// TestEventEnvelopeCreation tests that event envelopes are created correctly with metadata
func TestEventEnvelopeCreation(t *testing.T) {
	// Create test context with metadata
	correlationID := "test-correlation-123"
	causationID := "test-causation-456" // ID of previous event that caused this
	userID := "test-user-789"
	requestID := "test-request-abc"

	metadata := &internal.EventMetadata{
		CorrelationID: correlationID,
		CausationID:   causationID,
		UserID:        userID,
		RequestID:     requestID,
	}

	ctx := eventscommon.WithEventMetadata(context.Background(), metadata)

	// Create test event
	event := domain.BookingEvent{
		BookingID: "booking-123",
		Status:    domain.BookingRequested,
		Timestamp: time.Now(),
		UserID:    userID,
	}

	// Create envelope
	envelope := events.CreateEventEnvelope(ctx, "booking.created", event)

	// Verify envelope fields
	require.NotNil(t, envelope)
	assert.NotEmpty(t, envelope.EventID, "event ID should be generated")
	assert.Equal(t, "booking.created", envelope.EventType)
	assert.Equal(t, correlationID, envelope.CorrelationID, "correlation ID should come from metadata")
	assert.Equal(t, causationID, envelope.CausationID, "causation ID should be previous event's ID from metadata")
	assert.Equal(t, "booking-service", envelope.Producer)
	assert.Equal(t, event, envelope.Data)
	assert.False(t, envelope.OccurredAt.IsZero(), "occurred_at should be set")

	t.Logf("✓ Envelope created: EventID=%s, CorrelationID=%s, CausationID=%s",
		envelope.EventID, envelope.CorrelationID, envelope.CausationID)
}

// TestEventEnvelopeWithEmptyMetadata tests envelope creation with no metadata
func TestEventEnvelopeWithEmptyMetadata(t *testing.T) {
	ctx := context.Background()

	event := domain.BookingEvent{
		BookingID: "booking-456",
		Status:    domain.BookingAssigned,
		Timestamp: time.Now(),
		UserID:    "test-user-999",
	}

	envelope := events.CreateEventEnvelope(ctx, "booking.assigned", event)

	// Verify envelope fields - should generate new IDs
	require.NotNil(t, envelope)
	assert.NotEmpty(t, envelope.EventID, "event ID should be generated")
	assert.Equal(t, "booking.assigned", envelope.EventType)
	assert.NotEmpty(t, envelope.CorrelationID, "correlation ID should be generated when not in metadata")
	assert.Empty(t, envelope.CausationID, "causation ID should be empty for root events (no previous event)")
	assert.Equal(t, "booking-service", envelope.Producer)
	assert.Equal(t, event, envelope.Data)

	t.Logf("✓ Envelope created without metadata: EventID=%s, CorrelationID=%s",
		envelope.EventID, envelope.CorrelationID)
}

// TestGetBookingEventType tests event type mapping
func TestGetBookingEventType(t *testing.T) {
	tests := []struct {
		status   domain.BookingStatus
		expected string
	}{
		{domain.BookingRequested, "booking.created"},
		{domain.BookingOfferPending, "booking.offer.pending"},
		{domain.BookingOfferRejected, "booking.offer.rejected"},
		{domain.BookingAssigned, "booking.assigned"},
		{domain.BookingPendingQuote, "booking.quote.pending"},
		{domain.BookingQuoteProposed, "booking.quote.proposed"},
		{domain.BookingQuoteAccepted, "booking.quote.accepted"},
		{domain.BookingPaymentPending, "booking.payment.pending"},
		{domain.BookingConfirmed, "booking.confirmed"},
		{domain.BookingEnroute, "booking.enroute"},
		{domain.BookingInProgress, "booking.in_progress"},
		{domain.BookingCompleted, "booking.completed"},
		{domain.BookingCancelled, "booking.cancelled"},
		{domain.BookingClosed, "booking.closed"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			result := events.GetBookingEventType(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}

	t.Logf("✓ All %d event type mappings verified", len(tests))
}

// TestEventMetadataExtraction tests metadata extraction from context
func TestEventMetadataExtraction(t *testing.T) {
	// Test with metadata in context
	metadata := &internal.EventMetadata{
		CorrelationID: "test-correlation",
		CausationID:   "test-causation",
		UserID:        "test-user",
		RequestID:     "test-request",
	}

	ctx := eventscommon.WithEventMetadata(context.Background(), metadata)

	extracted, ok := eventscommon.ExtractEventMetadata(ctx)
	require.True(t, ok, "should extract metadata from context")
	assert.Equal(t, metadata, extracted)

	// Test without metadata in context
	ctx2 := context.Background()
	extracted2, ok2 := eventscommon.ExtractEventMetadata(ctx2)
	assert.False(t, ok2, "should return false when no metadata in context")
	assert.Nil(t, extracted2)

	t.Log("✓ Metadata extraction from context verified")
}

// TestEventCorrelationAcrossSagaSteps tests correlation ID flow through multi-step saga
// This is the CRITICAL test from acceptance criteria
func TestEventCorrelationAcrossSagaSteps(t *testing.T) {
	ctx := context.Background()

	// Step 1: Create booking (root event - starts the saga)
	createEvent := domain.BookingEvent{
		BookingID: "booking-123",
		Status:    domain.BookingRequested,
		Timestamp: time.Now(),
		UserID:    "user-456",
	}

	createEnvelope := events.CreateEventEnvelope(ctx, "booking.created", createEvent)
	correlationID := createEnvelope.CorrelationID // This will flow through entire saga

	t.Logf("Step 1 - Created: EventID=%s, CorrelationID=%s, CausationID=%s",
		createEnvelope.EventID, createEnvelope.CorrelationID, createEnvelope.CausationID)

	// Step 2: Offer booking (caused by create event)
	metadata2 := &internal.EventMetadata{
		CorrelationID: createEnvelope.CorrelationID, // SAME correlation as root
		CausationID:   createEnvelope.EventID,       // CAUSED BY create event
		UserID:        "user-456",
	}
	ctx2 := eventscommon.WithEventMetadata(ctx, metadata2)

	offerEvent := domain.BookingEvent{
		BookingID: "booking-123",
		Status:    domain.BookingOfferPending,
		Timestamp: time.Now(),
		UserID:    "user-456",
	}

	offerEnvelope := events.CreateEventEnvelope(ctx2, "booking.offered", offerEvent)

	t.Logf("Step 2 - Offered: EventID=%s, CorrelationID=%s, CausationID=%s",
		offerEnvelope.EventID, offerEnvelope.CorrelationID, offerEnvelope.CausationID)

	// Step 3: Assign booking (caused by offer event)
	metadata3 := &internal.EventMetadata{
		CorrelationID: offerEnvelope.CorrelationID, // SAME correlation as root
		CausationID:   offerEnvelope.EventID,       // CAUSED BY offer event
		UserID:        "user-456",
	}
	ctx3 := eventscommon.WithEventMetadata(ctx, metadata3)

	artisanID := "artisan-789"
	assignEvent := domain.BookingEvent{
		BookingID: "booking-123",
		Status:    domain.BookingAssigned,
		Timestamp: time.Now(),
		UserID:    "user-456",
		ArtisanID: &artisanID,
	}

	assignEnvelope := events.CreateEventEnvelope(ctx3, "booking.assigned", assignEvent)

	t.Logf("Step 3 - Assigned: EventID=%s, CorrelationID=%s, CausationID=%s",
		assignEnvelope.EventID, assignEnvelope.CorrelationID, assignEnvelope.CausationID)

	// VERIFY CORRELATION FLOWS THROUGH ENTIRE SAGA
	assert.Equal(t, correlationID, createEnvelope.CorrelationID,
		"create event should have correlation ID")
	assert.Equal(t, correlationID, offerEnvelope.CorrelationID,
		"offer event should have SAME correlation ID (flows through saga)")
	assert.Equal(t, correlationID, assignEnvelope.CorrelationID,
		"assign event should have SAME correlation ID (flows through saga)")

	// VERIFY CAUSATION CHAIN (event lineage)
	assert.Empty(t, createEnvelope.CausationID,
		"first event should have no causation (root of saga)")
	assert.Equal(t, createEnvelope.EventID, offerEnvelope.CausationID,
		"offer event was caused by create event")
	assert.Equal(t, offerEnvelope.EventID, assignEnvelope.CausationID,
		"assign event was caused by offer event")

	// VERIFY EVENT IDS ARE UNIQUE (each event has unique ID)
	assert.NotEqual(t, createEnvelope.EventID, offerEnvelope.EventID,
		"each event should have unique event ID")
	assert.NotEqual(t, offerEnvelope.EventID, assignEnvelope.EventID,
		"each event should have unique event ID")

	t.Log("✓ Correlation ID flows through entire saga (create → offer → assign)")
	t.Log("✓ Causation chain correctly links events in sequence")
}

// TestEventEnvelopePublishingThroughOutbox tests actual event publishing
func TestEventEnvelopePublishingThroughOutbox(t *testing.T) {
	ctx := context.Background()

	testDB, err := et.NewTestDatabase(ctx, "booking")
	require.NoError(t, err)

	// Create event with metadata
	metadata := &internal.EventMetadata{
		CorrelationID: "test-correlation-123",
		CausationID:   "test-causation-456", // ADD CAUSATION so it appears in JSON
		UserID:        "user-456",
		RequestID:     "req-789",
	}
	ctx = eventscommon.WithEventMetadata(ctx, metadata)

	// Publish event through actual outbox
	tx, err := testDB.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback()

	event := domain.BookingEvent{
		BookingID: "booking-123",
		Status:    domain.BookingRequested,
		Timestamp: time.Now(),
		UserID:    "user-456",
	}

	// Create envelope
	envelope := events.CreateEventEnvelope(ctx, "booking.created", event)

	// Publish through outbox (event wrapped in envelope)
	topicRef := pubsub.TopicRef[pubsub.Publisher[*events.EventEnvelope[domain.BookingEvent]]](events.RematchTopic)
	outboxRef := outbox.Bind(topicRef, outbox.TxPersister(tx))

	msgID, err := outboxRef.Publish(ctx, envelope)
	require.NoError(t, err)
	require.NotEmpty(t, msgID)

	require.NoError(t, tx.Commit())

	t.Logf("✓ Published envelope to outbox: msgID=%s", msgID)

	// Verify message in outbox contains envelope structure
	var dataJSON string
	err = testDB.QueryRow(ctx,
		"SELECT data::text FROM outbox WHERE id = $1", msgID).Scan(&dataJSON)
	require.NoError(t, err)
	require.NotEmpty(t, dataJSON)

	// Verify JSON contains envelope fields
	assert.Contains(t, dataJSON, "event_id", "should contain event_id field")
	assert.Contains(t, dataJSON, "correlation_id", "should contain correlation_id field")
	assert.Contains(t, dataJSON, "causation_id", "should contain causation_id field") // Now it will be there!
	assert.Contains(t, dataJSON, "producer", "should contain producer field")
	assert.Contains(t, dataJSON, "occurred_at", "should contain occurred_at field")
	assert.Contains(t, dataJSON, "booking-123", "should contain event data")

	t.Logf("✓ Event published with complete envelope structure in outbox")
}

// TestEventEnvelopeSerialization tests that envelopes can be properly serialized
func TestEventEnvelopeSerialization(t *testing.T) {
	metadata := &internal.EventMetadata{
		CorrelationID: "test-correlation",
		CausationID:   "test-causation",
		UserID:        "test-user",
		RequestID:     "test-request",
	}

	ctx := eventscommon.WithEventMetadata(context.Background(), metadata)

	event := domain.BookingEvent{
		BookingID: "booking-123",
		Status:    domain.BookingRequested,
		Timestamp: time.Now(),
		UserID:    "test-user",
	}

	envelope := events.CreateEventEnvelope(ctx, "booking.created", event)

	// Actually serialize to JSON
	jsonBytes, err := json.Marshal(envelope)
	require.NoError(t, err, "should serialize to JSON")
	require.NotEmpty(t, jsonBytes)

	t.Logf("✓ Serialized envelope: %s", string(jsonBytes))

	// Deserialize back
	var deserialized events.EventEnvelope[domain.BookingEvent]
	err = json.Unmarshal(jsonBytes, &deserialized)
	require.NoError(t, err, "should deserialize from JSON")

	// Verify round-trip preserves all fields
	assert.Equal(t, envelope.EventID, deserialized.EventID)
	assert.Equal(t, envelope.EventType, deserialized.EventType)
	assert.Equal(t, envelope.CorrelationID, deserialized.CorrelationID)
	assert.Equal(t, envelope.CausationID, deserialized.CausationID)
	assert.Equal(t, envelope.Producer, deserialized.Producer)
	assert.Equal(t, envelope.Data.BookingID, deserialized.Data.BookingID)
	assert.Equal(t, envelope.Data.Status, deserialized.Data.Status)
	assert.Equal(t, envelope.Data.UserID, deserialized.Data.UserID)

	// Verify timestamp (with small tolerance for time precision)
	timeDiff := envelope.OccurredAt.Sub(deserialized.OccurredAt)
	assert.Less(t, timeDiff, 1*time.Second, "timestamps should be nearly identical")

	t.Log("✓ Envelope serializes/deserializes correctly (round-trip verified)")
}

// TestEventMetadataExtractionFromHTTPHeaders tests metadata extraction from HTTP request
func TestEventMetadataExtractionFromHTTPHeaders(t *testing.T) {
	req, err := http.NewRequest("POST", "/v0/bookings", nil)
	require.NoError(t, err)

	// Set correlation/causation headers
	req.Header.Set("X-Correlation-ID", "http-correlation-123")
	req.Header.Set("X-Causation-ID", "http-causation-456")
	req.Header.Set("X-Request-ID", "http-request-789")

	// Extract metadata from request
	metadata := internal.ExtractMetadataFromHTTPRequest(req)

	// Verify extraction
	require.NotNil(t, metadata)
	assert.Equal(t, "http-correlation-123", metadata.CorrelationID)
	assert.Equal(t, "http-causation-456", metadata.CausationID)
	assert.Equal(t, "http-request-789", metadata.RequestID)

	t.Log("✓ Metadata extracted from HTTP headers correctly")
}

// TestEventMetadataWithoutHTTPHeaders tests metadata generation when no headers present
func TestEventMetadataWithoutHTTPHeaders(t *testing.T) {
	req, err := http.NewRequest("POST", "/v0/bookings", nil)
	require.NoError(t, err)
	// No headers set

	// Extract metadata (should generate new IDs)
	metadata := internal.ExtractMetadataFromHTTPRequest(req)

	// Verify metadata generated
	require.NotNil(t, metadata)
	assert.NotEmpty(t, metadata.CorrelationID, "should generate correlation ID when not in headers")
	assert.NotEmpty(t, metadata.RequestID, "should generate request ID when not in headers")
	// Causation ID should be empty (this is the root event)
	assert.Empty(t, metadata.CausationID, "causation ID should be empty for root request")

	t.Log("✓ Metadata generated when HTTP headers not present")
}

// TestEventEnvelopeWithoutHTTPRequest tests metadata generation when no HTTP request
func TestEventEnvelopeWithoutHTTPRequest(t *testing.T) {
	ctx := context.Background()

	// Create envelope without HTTP request context (e.g., from background job)
	event := domain.BookingEvent{
		BookingID: "booking-123",
		Status:    domain.BookingRequested,
		Timestamp: time.Now(),
		UserID:    "test-user",
	}

	envelope := events.CreateEventEnvelope(ctx, "booking.created", event)

	// Verify that envelope is created even without HTTP request
	require.NotNil(t, envelope)
	assert.NotEmpty(t, envelope.CorrelationID, "should generate correlation ID")
	assert.Empty(t, envelope.CausationID, "should have empty causation (root event)")
	assert.NotEmpty(t, envelope.EventID, "should generate event ID")
	assert.Equal(t, "booking.created", envelope.EventType)
	assert.Equal(t, "booking-service", envelope.Producer)

	t.Log("✓ Envelope created without HTTP context (background job scenario)")
}

// TestMultipleEventsWithSameCorrelation tests that multiple events can share correlation
func TestMultipleEventsWithSameCorrelation(t *testing.T) {
	// Simulate a single HTTP request that generates multiple events
	correlationID := uuid.New().String()

	metadata := &internal.EventMetadata{
		CorrelationID: correlationID,
		UserID:        "user-123",
		RequestID:     "req-456",
	}

	ctx := eventscommon.WithEventMetadata(context.Background(), metadata)

	// Create first event
	event1 := domain.BookingEvent{
		BookingID: "booking-1",
		Status:    domain.BookingRequested,
		Timestamp: time.Now(),
		UserID:    "user-123",
	}
	envelope1 := events.CreateEventEnvelope(ctx, "booking.created", event1)

	// Update context with causation from first event
	metadata2 := &internal.EventMetadata{
		CorrelationID: correlationID,     // SAME correlation
		CausationID:   envelope1.EventID, // Caused by first event
		UserID:        "user-123",
		RequestID:     "req-456",
	}
	ctx2 := eventscommon.WithEventMetadata(ctx, metadata2)

	// Create second event (in response to first)
	event2 := domain.BookingEvent{
		BookingID: "booking-1",
		Status:    domain.BookingOfferPending,
		Timestamp: time.Now(),
		UserID:    "user-123",
	}
	envelope2 := events.CreateEventEnvelope(ctx2, "booking.offered", event2)

	// Verify correlation IDs are the same (same request/saga)
	assert.Equal(t, correlationID, envelope1.CorrelationID)
	assert.Equal(t, correlationID, envelope2.CorrelationID)
	assert.Equal(t, envelope1.CorrelationID, envelope2.CorrelationID,
		"both events should have same correlation ID")

	// Verify causation shows event lineage
	assert.Empty(t, envelope1.CausationID, "first event has no causation")
	assert.Equal(t, envelope1.EventID, envelope2.CausationID,
		"second event was caused by first event")

	// Verify event IDs are different
	assert.NotEqual(t, envelope1.EventID, envelope2.EventID,
		"each event should have unique event ID")

	t.Log("✓ Multiple events share correlation ID but have unique event IDs and causation chain")
}

// TestEventEnvelopeTimestampOrdering tests that event timestamps are properly ordered
func TestEventEnvelopeTimestampOrdering(t *testing.T) {
	ctx := context.Background()

	event1 := domain.BookingEvent{
		BookingID: "booking-1",
		Status:    domain.BookingRequested,
		Timestamp: time.Now(),
		UserID:    "user-123",
	}
	envelope1 := events.CreateEventEnvelope(ctx, "booking.created", event1)

	// Small delay to ensure different timestamps
	time.Sleep(10 * time.Millisecond)

	event2 := domain.BookingEvent{
		BookingID: "booking-1",
		Status:    domain.BookingAssigned,
		Timestamp: time.Now(),
		UserID:    "user-123",
	}
	envelope2 := events.CreateEventEnvelope(ctx, "booking.assigned", event2)

	// Verify timestamps show chronological order
	assert.True(t, envelope2.OccurredAt.After(envelope1.OccurredAt),
		"second event should have later timestamp")

	timeDiff := envelope2.OccurredAt.Sub(envelope1.OccurredAt)
	assert.GreaterOrEqual(t, timeDiff, 10*time.Millisecond,
		"timestamp difference should reflect actual time passed")

	t.Logf("✓ Event timestamps properly ordered (diff: %v)", timeDiff)
}

// TestEventProducerField tests that producer field is correctly set
func TestEventProducerField(t *testing.T) {
	ctx := context.Background()

	event := domain.BookingEvent{
		BookingID: "booking-123",
		Status:    domain.BookingRequested,
		Timestamp: time.Now(),
		UserID:    "user-456",
	}

	envelope := events.CreateEventEnvelope(ctx, "booking.created", event)

	assert.Equal(t, "booking-service", envelope.Producer,
		"producer should identify the service that created the event")

	t.Log("✓ Producer field correctly identifies booking service")
}
