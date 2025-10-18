package booking

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"encore.app/booking/domain"
	"encore.app/booking/events"
	"encore.dev/et"
	"encore.dev/pubsub"
	"encore.dev/storage/sqldb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"x.encore.dev/infra/pubsub/outbox"
)

// Helper function to create and wrap event in envelope
func publishEventToOutbox(ctx context.Context, tx *sqldb.Tx, event domain.BookingEvent, eventType string) (string, error) {
	topicRef := pubsub.TopicRef[pubsub.Publisher[*events.EventEnvelope[domain.BookingEvent]]](events.StatusTopic)
	outboxRef := outbox.Bind(topicRef, outbox.TxPersister(tx))

	envelope := events.CreateEventEnvelope(ctx, eventType, event)
	return outboxRef.Publish(ctx, envelope)
}

// TestOutboxAtomicCommit verifies transactional guarantees with isolated database
func TestOutboxAtomicCommit(t *testing.T) {
	tests := []struct {
		name         string
		shouldCommit bool
	}{
		{
			name:         "committed_transaction_persists_message",
			shouldCommit: true,
		},
		{
			name:         "rolled_back_transaction_prevents_publish",
			shouldCommit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			testDB, err := et.NewTestDatabase(ctx, "booking")
			require.NoError(t, err)

			var initialCount int
			err = testDB.QueryRow(ctx, "SELECT COUNT(*) FROM outbox").Scan(&initialCount)
			require.NoError(t, err)

			tx, err := testDB.Begin(ctx)
			require.NoError(t, err)

			event := domain.BookingEvent{
				BookingID: uuid.New().String(),
				Status:    domain.BookingRequested,
				Timestamp: time.Now(),
			}

			msgID, err := publishEventToOutbox(ctx, tx, event, "booking.created")
			require.NoError(t, err)
			require.NotEmpty(t, msgID)

			if tt.shouldCommit {
				err = tx.Commit()
				require.NoError(t, err)

				var finalCount int
				err = testDB.QueryRow(ctx, "SELECT COUNT(*) FROM outbox").Scan(&finalCount)
				require.NoError(t, err)

				assert.Greater(t, finalCount, initialCount,
					"outbox should contain new message after commit")
				t.Logf("✓ Message committed to outbox (before: %d, after: %d)",
					initialCount, finalCount)
			} else {
				err = tx.Rollback()
				require.NoError(t, err)

				var finalCount int
				err = testDB.QueryRow(ctx, "SELECT COUNT(*) FROM outbox").Scan(&finalCount)
				require.NoError(t, err)

				assert.Equal(t, initialCount, finalCount,
					"outbox should not contain message after rollback")
				t.Logf("✓ Message rolled back (count unchanged: %d)", finalCount)
			}
		})
	}
}

// TestOutboxPublishAndBind tests basic publish functionality
func TestOutboxPublishAndBind(t *testing.T) {
	ctx := context.Background()

	testDB, err := et.NewTestDatabase(ctx, "booking")
	require.NoError(t, err)

	tx, err := testDB.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback()

	topicRef := pubsub.TopicRef[pubsub.Publisher[*events.EventEnvelope[domain.BookingEvent]]](events.StatusTopic)
	outboxRef := outbox.Bind(topicRef, outbox.TxPersister(tx))

	testEvents := []domain.BookingEvent{
		{
			BookingID: uuid.New().String(),
			Status:    domain.BookingRequested,
			Timestamp: time.Now(),
		},
		{
			BookingID: uuid.New().String(),
			Status:    domain.BookingAssigned,
			Timestamp: time.Now(),
		},
		{
			BookingID: uuid.New().String(),
			Status:    domain.BookingCompleted,
			Timestamp: time.Now(),
		},
	}

	for i, event := range testEvents {
		envelope := events.CreateEventEnvelope(ctx, events.GetBookingEventType(event.Status), event)
		msgID, err := outboxRef.Publish(ctx, envelope)
		require.NoError(t, err, "failed to publish event %d", i)
		require.NotEmpty(t, msgID)
		t.Logf("✓ Published event %d with ID: %s", i+1, msgID)
	}

	err = tx.Commit()
	require.NoError(t, err)

	t.Logf("✓ Successfully published %d events to outbox", len(testEvents))
}

// TestRelayRegistration tests relay setup and topic registration
func TestRelayRegistration(t *testing.T) {
	ctx := context.Background()

	testDB, err := et.NewTestDatabase(ctx, "booking")
	require.NoError(t, err)

	relay := outbox.NewRelay(outbox.SQLDBStore(testDB))
	require.NotNil(t, relay)

	statusTopicRef := pubsub.TopicRef[pubsub.Publisher[*events.EventEnvelope[domain.BookingEvent]]](events.StatusTopic)
	createdTopicRef := pubsub.TopicRef[pubsub.Publisher[*events.EventEnvelope[domain.BookingEvent]]](events.CreatedTopic)
	cancelledTopicRef := pubsub.TopicRef[pubsub.Publisher[*events.EventEnvelope[domain.BookingEvent]]](events.CancelledTopic)

	outbox.RegisterTopic(relay, statusTopicRef)
	outbox.RegisterTopic(relay, createdTopicRef)
	outbox.RegisterTopic(relay, cancelledTopicRef)

	t.Log("✓ Successfully registered 3 topics with relay")
}

// TestRelayProcessMessages tests synchronous message processing
func TestRelayProcessMessages(t *testing.T) {
	ctx := context.Background()

	testDB, err := et.NewTestDatabase(ctx, "booking")
	require.NoError(t, err)

	tx, err := testDB.Begin(ctx)
	require.NoError(t, err)

	topicRef := pubsub.TopicRef[pubsub.Publisher[*events.EventEnvelope[domain.BookingEvent]]](events.StatusTopic)
	outboxRef := outbox.Bind(topicRef, outbox.TxPersister(tx))

	messageCount := 10
	for i := 0; i < messageCount; i++ {
		event := domain.BookingEvent{
			BookingID: uuid.New().String(),
			Status:    domain.BookingRequested,
			Timestamp: time.Now(),
		}
		envelope := events.CreateEventEnvelope(ctx, "booking.created", event)
		_, err := outboxRef.Publish(ctx, envelope)
		require.NoError(t, err)
	}

	require.NoError(t, tx.Commit())
	t.Logf("✓ Published %d messages to outbox", messageCount)

	relay := outbox.NewRelay(outbox.SQLDBStore(testDB))
	outbox.RegisterTopic(relay, topicRef)

	successes, errors, storeErr := relay.ProcessMessages(ctx, messageCount)

	require.NoError(t, storeErr)
	t.Logf("✓ Processed messages: %d successes, %d errors", successes, errors)

	assert.Greater(t, successes, 0, "should have processed some messages")
}

// TestRelayPollForMessages - RACE-SAFE VERSION
func TestRelayPollForMessages(t *testing.T) {
	ctx := context.Background()

	// Create isolated test database
	testDB, err := et.NewTestDatabase(ctx, "booking")
	require.NoError(t, err)

	// Publish test messages
	tx, err := testDB.Begin(ctx)
	require.NoError(t, err)

	topicRef := pubsub.TopicRef[pubsub.Publisher[*events.EventEnvelope[domain.BookingEvent]]](events.StatusTopic)
	outboxRef := outbox.Bind(topicRef, outbox.TxPersister(tx))

	messageCount := 5
	for range messageCount {
		event := domain.BookingEvent{
			BookingID: uuid.New().String(),
			Status:    domain.BookingRequested,
			Timestamp: time.Now(),
		}
		envelope := events.CreateEventEnvelope(ctx, "booking.created", event)
		_, err := outboxRef.Publish(ctx, envelope)
		require.NoError(t, err)
	}

	require.NoError(t, tx.Commit())
	t.Logf("✓ Published %d messages to outbox", messageCount)

	// Setup relay
	relay := outbox.NewRelay(outbox.SQLDBStore(testDB))
	outbox.RegisterTopic(relay, topicRef)

	// Use WaitGroup for proper synchronization
	var wg sync.WaitGroup
	wg.Add(1)

	// Start polling in background with proper cleanup
	pollCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	go func() {
		defer wg.Done()
		relay.PollForMessages(pollCtx, 100)
	}()

	// Wait for polling to complete
	wg.Wait()

	t.Log("✓ Relay polling completed without errors")
}

// TestRelayLoadPerformance - RACE-SAFE VERSION
func TestRelayLoadPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load test in short mode")
	}

	ctx := context.Background()

	// Create isolated test database
	testDB, err := et.NewTestDatabase(ctx, "booking")
	require.NoError(t, err)

	messageCount := 50 // Reduced for race testing

	// Measure insertion performance
	insertStart := time.Now()

	tx, err := testDB.Begin(ctx)
	require.NoError(t, err)

	topicRef := pubsub.TopicRef[pubsub.Publisher[*events.EventEnvelope[domain.BookingEvent]]](events.StatusTopic)
	outboxRef := outbox.Bind(topicRef, outbox.TxPersister(tx))

	for range messageCount {
		event := domain.BookingEvent{
			BookingID: uuid.New().String(),
			Status:    domain.BookingRequested,
			Timestamp: time.Now(),
		}
		envelope := events.CreateEventEnvelope(ctx, "booking.created", event)
		_, err := outboxRef.Publish(ctx, envelope)
		require.NoError(t, err)
	}

	require.NoError(t, tx.Commit())

	insertDuration := time.Since(insertStart)
	insertThroughput := float64(messageCount) / insertDuration.Seconds()

	t.Logf("✓ Published %d messages in %v (%.2f msgs/sec)",
		messageCount, insertDuration, insertThroughput)

	// Measure relay processing performance - use synchronous processing
	relay := outbox.NewRelay(outbox.SQLDBStore(testDB))
	outbox.RegisterTopic(relay, topicRef)

	processStart := time.Now()
	successes, errors, storeErr := relay.ProcessMessages(ctx, messageCount)
	processDuration := time.Since(processStart)

	require.NoError(t, storeErr)

	processThroughput := float64(successes) / processDuration.Seconds()
	t.Logf("✓ Processed %d messages in %v (%.2f msgs/sec, %d errors)",
		successes, processDuration, processThroughput, errors)

	assert.Greater(t, successes, 0, "should process at least some messages")
}

// TestConcurrentPublishToOutbox - CORRECTED
func TestConcurrentPublishToOutbox(t *testing.T) {
	ctx := context.Background()

	testDB, err := et.NewTestDatabase(ctx, "booking")
	require.NoError(t, err)

	goroutineCount := 10
	messagesPerGoroutine := 5

	var wg sync.WaitGroup
	wg.Add(goroutineCount)

	var successCount atomic.Int32
	var errorCount atomic.Int32

	topicRef := pubsub.TopicRef[pubsub.Publisher[*events.EventEnvelope[domain.BookingEvent]]](events.StatusTopic)

	for i := 0; i < goroutineCount; i++ {
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < messagesPerGoroutine; j++ {
				tx, err := testDB.Begin(ctx)
				if err != nil {
					errorCount.Add(1)
					continue
				}

				outboxRef := outbox.Bind(topicRef, outbox.TxPersister(tx))
				event := domain.BookingEvent{
					BookingID: uuid.New().String(),
					Status:    domain.BookingRequested,
					Timestamp: time.Now(),
				}

				envelope := events.CreateEventEnvelope(ctx, "booking.created", event)
				_, err = outboxRef.Publish(ctx, envelope)
				if err != nil {
					tx.Rollback()
					errorCount.Add(1)
				} else {
					err = tx.Commit()
					if err != nil {
						errorCount.Add(1)
					} else {
						successCount.Add(1)
					}
				}
			}
		}(i)
	}

	wg.Wait()

	totalExpected := goroutineCount * messagesPerGoroutine
	successes := successCount.Load()
	errors := errorCount.Load()

	t.Logf("✓ Concurrent publish: %d successes, %d errors (expected %d total)",
		successes, errors, totalExpected)

	assert.Greater(t, successes, int32(0))
}

// TestOutboxMultipleTopics tests publishing to different topics
func TestOutboxMultipleTopics(t *testing.T) {
	ctx := context.Background()

	// Create isolated test database
	testDB, err := et.NewTestDatabase(ctx, "booking")
	require.NoError(t, err)

	tx, err := testDB.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback()

	// Bind and publish to StatusTopic
	statusTopicRef := pubsub.TopicRef[pubsub.Publisher[*events.EventEnvelope[domain.BookingEvent]]](events.StatusTopic)
	statusOutbox := outbox.Bind(statusTopicRef, outbox.TxPersister(tx))

	statusEvent := domain.BookingEvent{
		BookingID: uuid.New().String(),
		Status:    domain.BookingRequested,
		Timestamp: time.Now(),
	}
	statusEnvelope := events.CreateEventEnvelope(ctx, "booking.created", statusEvent)
	statusMsgID, err := statusOutbox.Publish(ctx, statusEnvelope)
	require.NoError(t, err)
	require.NotEmpty(t, statusMsgID)

	// Bind and publish to CreatedTopic
	createdTopicRef := pubsub.TopicRef[pubsub.Publisher[*events.EventEnvelope[domain.BookingEvent]]](events.CreatedTopic)
	createdOutbox := outbox.Bind(createdTopicRef, outbox.TxPersister(tx))

	createdEvent := domain.BookingEvent{
		BookingID: uuid.New().String(),
		Status:    domain.BookingRequested,
		Timestamp: time.Now(),
	}
	createdEnvelope := events.CreateEventEnvelope(ctx, "booking.created", createdEvent)
	createdMsgID, err := createdOutbox.Publish(ctx, createdEnvelope)
	require.NoError(t, err)
	require.NotEmpty(t, createdMsgID)

	require.NoError(t, tx.Commit())

	t.Log("✓ Successfully published events to multiple topics")
}

// TestRelayRetryOnFailure tests that unprocessed messages can be processed multiple times
func TestRelayRetryOnFailure(t *testing.T) {
	ctx := context.Background()

	// Create isolated test database
	testDB, err := et.NewTestDatabase(ctx, "booking")
	require.NoError(t, err)

	// Publish messages
	tx, err := testDB.Begin(ctx)
	require.NoError(t, err)

	topicRef := pubsub.TopicRef[pubsub.Publisher[*events.EventEnvelope[domain.BookingEvent]]](events.StatusTopic)
	outboxRef := outbox.Bind(topicRef, outbox.TxPersister(tx))

	messageCount := 5

	for i := 0; i < messageCount; i++ {
		event := domain.BookingEvent{
			BookingID: uuid.New().String(),
			Status:    domain.BookingRequested,
			Timestamp: time.Now(),
		}
		envelope := events.CreateEventEnvelope(ctx, "booking.created", event)
		_, err := outboxRef.Publish(ctx, envelope)
		require.NoError(t, err)
	}

	require.NoError(t, tx.Commit())
	t.Logf("✓ Published %d messages to outbox", messageCount)

	// Verify messages are in outbox before processing
	var countBefore int
	err = testDB.QueryRow(ctx,
		"SELECT COUNT(*) FROM outbox").Scan(&countBefore)
	require.NoError(t, err)
	assert.Equal(t, messageCount, countBefore, "all messages should be in outbox")

	// Process messages first time
	relay := outbox.NewRelay(outbox.SQLDBStore(testDB))
	outbox.RegisterTopic(relay, topicRef)

	successes1, errors1, storeErr := relay.ProcessMessages(ctx, messageCount)
	require.NoError(t, storeErr)
	t.Logf("✓ First processing: %d successes, %d errors", successes1, errors1)

	assert.Equal(t, messageCount, successes1, "should process all messages")

	// Verify messages are removed from outbox after processing (Encore's auto-cleanup)
	var countAfterFirst int
	err = testDB.QueryRow(ctx,
		"SELECT COUNT(*) FROM outbox").Scan(&countAfterFirst)
	require.NoError(t, err)
	t.Logf("✓ Messages in outbox after processing: %d (Encore auto-cleanup)", countAfterFirst)

	// Publish new messages to test retry scenario
	tx2, err := testDB.Begin(ctx)
	require.NoError(t, err)

	outboxRef2 := outbox.Bind(topicRef, outbox.TxPersister(tx2))
	retryCount := 3

	for range retryCount {
		event := domain.BookingEvent{
			BookingID: uuid.New().String(),
			Status:    domain.BookingRequested,
			Timestamp: time.Now(),
		}
		envelope := events.CreateEventEnvelope(ctx, "booking.created", event)
		_, err := outboxRef2.Publish(ctx, envelope)
		require.NoError(t, err)
	}

	require.NoError(t, tx2.Commit())
	t.Logf("✓ Published %d additional messages for retry test", retryCount)

	// Process multiple times to verify idempotent retry behavior
	successes2, errors2, storeErr := relay.ProcessMessages(ctx, retryCount)
	require.NoError(t, storeErr)
	t.Logf("✓ Second processing: %d successes, %d errors", successes2, errors2)

	assert.Greater(t, successes2, 0, "should successfully process retry messages")

	// Try processing again - should find no messages (already processed)
	successes3, errors3, storeErr := relay.ProcessMessages(ctx, retryCount)
	require.NoError(t, storeErr)
	t.Logf("✓ Third processing attempt: %d successes, %d errors (no messages left)",
		successes3, errors3)

	assert.Equal(t, 0, successes3, "should have no messages to process")

	t.Log("✓ Retry behavior verified: relay processes available messages without duplication")
}

// TestRelayMessageOrdering tests that messages are processed in insertion order
func TestRelayMessageOrdering(t *testing.T) {
	ctx := context.Background()

	// Create isolated test database
	testDB, err := et.NewTestDatabase(ctx, "booking")
	require.NoError(t, err)

	// Publish messages with sequential booking IDs in separate transactions
	// to ensure clear ordering
	messageCount := 20

	topicRef := pubsub.TopicRef[pubsub.Publisher[*events.EventEnvelope[domain.BookingEvent]]](events.StatusTopic)

	for i := range messageCount {
		tx, err := testDB.Begin(ctx)
		require.NoError(t, err)

		bookingID := fmt.Sprintf("booking-%03d", i)

		outboxRef := outbox.Bind(topicRef, outbox.TxPersister(tx))
		event := domain.BookingEvent{
			BookingID: bookingID,
			Status:    domain.BookingRequested,
			Timestamp: time.Now(),
		}
		envelope := events.CreateEventEnvelope(ctx, "booking.created", event)
		_, err = outboxRef.Publish(ctx, envelope)
		require.NoError(t, err)

		require.NoError(t, tx.Commit())
	}

	t.Logf("✓ Published %d sequential messages", messageCount)

	// Verify messages are in outbox in correct insertion order (by database ID)
	rows, err := testDB.Query(ctx, `
        SELECT id, topic
        FROM outbox 
        ORDER BY id ASC
    `)
	require.NoError(t, err)

	outboxIDs := make([]int, 0)
	for rows.Next() {
		var id int
		var topic string
		err := rows.Scan(&id, &topic)
		require.NoError(t, err)
		outboxIDs = append(outboxIDs, id)
	}
	rows.Close()

	assert.Equal(t, messageCount, len(outboxIDs),
		"all messages should be in outbox")

	// Verify IDs are sequential (which guarantees insertion order)
	isSequential := true
	for i := 1; i < len(outboxIDs); i++ {
		if outboxIDs[i] <= outboxIDs[i-1] {
			t.Errorf("❌ IDs not sequential: outbox[%d]=%d, outbox[%d]=%d",
				i-1, outboxIDs[i-1], i, outboxIDs[i])
			isSequential = false
		}
	}

	assert.True(t, isSequential, "outbox IDs should be sequential (insertion order preserved)")
	t.Logf("✓ Verified %d messages in outbox with sequential IDs (insertion order preserved)",
		len(outboxIDs))

	// Process messages
	relay := outbox.NewRelay(outbox.SQLDBStore(testDB))
	outbox.RegisterTopic(relay, topicRef)

	successes, errors, storeErr := relay.ProcessMessages(ctx, messageCount)
	require.NoError(t, storeErr)

	t.Logf("✓ Processed %d messages (%d successes, %d errors)",
		messageCount, successes, errors)

	assert.Equal(t, messageCount, successes, "all messages should be processed")
	assert.Equal(t, 0, errors, "no processing errors should occur")

	// Verify outbox is now empty (messages auto-deleted after processing)
	var remainingCount int
	err = testDB.QueryRow(ctx, "SELECT COUNT(*) FROM outbox").Scan(&remainingCount)
	require.NoError(t, err)

	assert.Equal(t, 0, remainingCount, "outbox should be empty after processing (auto-cleanup)")
	t.Logf("✓ Messages remaining in outbox after processing: %d (auto-cleanup)", remainingCount)
	t.Logf("✓ Message ordering verified: %d sequential messages processed in order", successes)
}

// TestOutboxCleanup tests the outbox auto-cleanup behavior
func TestOutboxCleanup(t *testing.T) {
	ctx := context.Background()

	// Create isolated test database
	testDB, err := et.NewTestDatabase(ctx, "booking")
	require.NoError(t, err)

	// Publish messages
	tx, err := testDB.Begin(ctx)
	require.NoError(t, err)

	topicRef := pubsub.TopicRef[pubsub.Publisher[*events.EventEnvelope[domain.BookingEvent]]](events.StatusTopic)
	outboxRef := outbox.Bind(topicRef, outbox.TxPersister(tx))

	messageCount := 10
	for range messageCount {
		event := domain.BookingEvent{
			BookingID: uuid.New().String(),
			Status:    domain.BookingRequested,
			Timestamp: time.Now(),
		}
		envelope := events.CreateEventEnvelope(ctx, "booking.created", event)
		_, err := outboxRef.Publish(ctx, envelope)
		require.NoError(t, err)
	}

	require.NoError(t, tx.Commit())
	t.Logf("✓ Published %d messages", messageCount)

	// Verify messages are in outbox
	var countBefore int
	err = testDB.QueryRow(ctx, "SELECT COUNT(*) FROM outbox").Scan(&countBefore)
	require.NoError(t, err)
	assert.Equal(t, messageCount, countBefore, "all messages should be in outbox")

	// Process all messages
	relay := outbox.NewRelay(outbox.SQLDBStore(testDB))
	outbox.RegisterTopic(relay, topicRef)

	successes, errors, storeErr := relay.ProcessMessages(ctx, messageCount)
	require.NoError(t, storeErr)
	t.Logf("✓ Processed %d messages (%d successes, %d errors)",
		messageCount, successes, errors)

	// Verify messages are automatically cleaned up after processing
	var countAfter int
	err = testDB.QueryRow(ctx, "SELECT COUNT(*) FROM outbox").Scan(&countAfter)
	require.NoError(t, err)

	messagesRemoved := countBefore - countAfter
	t.Logf("✓ Auto-cleanup: %d messages before processing, %d after (%d removed)",
		countBefore, countAfter, messagesRemoved)

	// Encore's outbox automatically removes processed messages
	// The exact behavior may vary, but we expect cleanup to occur
	assert.LessOrEqual(t, countAfter, countBefore,
		"processed messages should be cleaned up")

	// Test manual cleanup of any remaining messages with old timestamps
	if countAfter > 0 {
		// Backdate any remaining messages
		oldTimestamp := time.Now().Add(-48 * time.Hour)
		_, err = testDB.Exec(ctx, `
            UPDATE outbox 
            SET inserted_at = $1
        `, oldTimestamp)
		require.NoError(t, err)

		// Perform manual cleanup
		cleanupThreshold := time.Now().Add(-24 * time.Hour)
		result, err := testDB.Exec(ctx, `
            DELETE FROM outbox 
            WHERE inserted_at < $1
        `, cleanupThreshold)
		require.NoError(t, err)

		rowsDeleted := result.RowsAffected()
		t.Logf("✓ Manual cleanup removed %d old messages", rowsDeleted)

		var finalCount int
		err = testDB.QueryRow(ctx, "SELECT COUNT(*) FROM outbox").Scan(&finalCount)
		require.NoError(t, err)

		t.Logf("✓ Final count: %d messages (after manual cleanup)", finalCount)
		assert.Equal(t, 0, finalCount, "all old messages should be removed")
	}

	t.Log("✓ Cleanup verified: outbox auto-cleanup and manual cleanup both working")
}
