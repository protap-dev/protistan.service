# Core Relay Documentation

## Overview

The `core/relay` package provides a **generic, reusable implementation of the Transactional Outbox Pattern** for reliable event publishing in distributed microservices. It enables services to publish events to message queues (via Encore pubsub) with guaranteed delivery, exactly-once semantics, and automatic retry handling.

## Architecture

### Design Philosophy

The relay system follows a **modular, composable architecture** where:

- **Core logic** lives in `core/relay` (generic, reusable)
- **Service-specific logic** lives in `{service}/relay` (wrapper implementations)
- **Event partitioning** happens at the database level via topic filtering


### Key Components

```
┌─────────────────────────────────────────────────────┐
│                   Service Layer                     │
│              (booking, quote, chat, etc.)           │
└───────────────────┬─────────────────────────────────┘
                    │ Creates outbox events
                    ▼
┌─────────────────────────────────────────────────────┐
│              Shared Outbox Table (coreDB)           │
│   ┌─────────────────────────────────────────┐       │
│   │ id, topic, data, status, processed_at   │       │
│   └─────────────────────────────────────────┘       │
└───────────────────┬─────────────────────────────────┘
                    │ Polled by relays (topic-filtered)
        ┌───────────┼───────────┐
        ▼           ▼           ▼
   ┌─────────┐ ┌─────────┐ ┌─────────┐
   │ Booking │ │  Quote  │ │  Chat   │
   │  Relay  │ │  Relay  │ │  Relay  │
   └────┬────┘ └────┬────┘ └────┬────┘
        │           │           │
        ▼           ▼           ▼
   Publishes to Encore Pubsub Topics
        │           │           │
        ▼           ▼           ▼
   Event Subscribers (OnQuoteProposed, etc.)
```


## File Structure

```
core/relay/
├── README.md           # This file
├── interfaces.go       # Core abstractions (Processor, Publisher, Cleanup)
├── relay.go           # Main relay orchestrator
├── processor.go       # Event retrieval and marking logic
├── cleanup.go         # Old event cleanup logic
├── config.go          # Configuration structs
└── metrics.go         # Monitoring and observability
```


## Core Abstractions

### 1. Processor Interface

Handles event lifecycle in the outbox table:

```go
type Processor interface {
    // Retrieves unprocessed events filtered by topic prefix
    GetUnprocessedEvents(ctx context.Context, db *gorm.DB, batchSize int, topicPrefix string) ([]*repository.OutboxEvent, error)
    
    // Marks an event as successfully processed
    MarkEventProcessed(ctx context.Context, db *gorm.DB, eventID string, processedAt *time.Time) error
}
```

**Key Feature:** `topicPrefix` parameter enables **database-level event partitioning** to prevent race conditions between service relays.

**Implementation:** `DefaultProcessor` in `processor.go`

### 2. Publisher Interface

Transforms outbox events into pubsub messages:

```go
type Publisher[EventType EventData] interface {
    PublishToTopic(ctx context.Context, outboxEvent *repository.OutboxEvent, event EventType) error
}
```

**Service-Specific Implementations:**

- `booking/relay/publisher.go` - `BookingPublisher`
- `quote/relay/publisher.go` - `QuotePublisher`
- `chat/relay/publisher.go` - `ChatPublisher`


### 3. Cleanup Interface

Manages outbox table size:

```go
type Cleanup interface {
    CleanupOldEvents(ctx context.Context, db *gorm.DB, retentionPeriod time.Duration) error
}
```

**Implementation:** `DefaultCleanup` in `cleanup.go`

### 4. EventData Interface

Marker interface for type safety:

```go
type EventData interface {
    EventType() string
}
```

All service events must implement this interface.

## Usage

### Creating a Service Relay

Each service creates a thin wrapper around `core/relay`:

```go
// quote/relay/relay.go
package relay

import (
    "context"
    "encore.app/core/relay"
    "gorm.io/gorm"
)

type Relay struct {
    coreRelay *relay.Relay[QuoteEvent]
}

func NewRelay(db *gorm.DB, config relay.Config) *Relay {
    // Create service-specific components
    publisher := &QuotePublisher{}
    processor := &relay.DefaultProcessor{}
    cleanup := &relay.DefaultCleanup{}
    
    // CRITICAL: Define topic prefix for event filtering
    topicPrefix := "quote-"
    
    // Instantiate core relay
    coreRelay := relay.NewRelay(
        db, 
        publisher, 
        processor, 
        cleanup, 
        config, 
        topicPrefix,  // ← Prevents processing other services' events
    )
    
    return &Relay{coreRelay: coreRelay}
}

func (r *Relay) Start(ctx context.Context) {
    r.coreRelay.Start(ctx)
}

func (r *Relay) Stop() {
    r.coreRelay.Stop()
}
```


### Topic Prefix Requirements

Each service **must** use a unique, consistent topic prefix:


| Service | Topic Prefix | Example Topics |
| :-- | :-- | :-- |
| Booking | `"booking"` | `booking.created`, `booking.status`, `booking.cancelled` |
| Quote | `"quote-"` | `quote-v1-proposed`, `quote-v1-accepted`, `quote-v1-rejected` |
| Chat | `"chat-"` | `chat-v1-message-sent`, `chat-v1-message-received` |

**Why Topic Prefixes Matter:**

Without topic-based filtering, multiple relays can process the same events, leading to:

- ❌ Race conditions
- ❌ Silent failures (events marked "processed" without being published)
- ❌ Lost events

The `topicPrefix` parameter ensures each relay only processes events it's responsible for via a SQL `LIKE` query:

```sql
SELECT * FROM outbox 
WHERE processed_at IS NULL 
  AND topic LIKE 'quote-%'  -- Only quote events
ORDER BY inserted_at ASC
LIMIT 100
```


## Configuration

### Config Struct

```go
type Config struct {
    PollingInterval   time.Duration  // How often to check for new events (default: 5s)
    BatchSize         int            // Max events per processing batch (default: 100)
    RetryMaxAttempts  int            // Max retry attempts before giving up (default: 3)
    RetryBackoff      time.Duration  // Initial backoff duration (default: 30s)
    CleanupInterval   time.Duration  // How often to clean old events (default: 1h)
    RetentionPeriod   time.Duration  // How long to keep processed events (default: 7 days)
}
```


### Default Configuration

```go
cfg := relay.DefaultConfig()
// Returns:
// {
//     PollingInterval:   5 * time.Second,
//     BatchSize:         100,
//     RetryMaxAttempts:  3,
//     RetryBackoff:      30 * time.Second,
//     CleanupInterval:   1 * time.Hour,
//     RetentionPeriod:   7 * 24 * time.Hour,
// }
```


### Custom Configuration

```go
customCfg := relay.Config{
    PollingInterval: 2 * time.Second,  // More aggressive polling
    BatchSize: 50,                     // Smaller batches
}

relay := NewRelay(db, customCfg)
```


## Event Lifecycle

### States

Events in the outbox table transition through these states:

```
pending → processing → processed
   ↓
failed (with retry)
   ↓
pending (retry) → processing → processed
```


### State Diagram

```
┌─────────┐
│ pending │ ← Initial state when created
└────┬────┘
     │ GetUnprocessedEvents()
     ▼
┌────────────┐
│ processing │ ← Relay picked up the event
└─────┬──────┘
      │
      ├─ Success → MarkEventProcessed()
      │              ▼
      │         ┌───────────┐
      │         │ processed │ ← Event published successfully
      │         └───────────┘
      │
      └─ Failure → scheduleRetry()
                    ▼
                ┌────────┐
                │ failed │ ← Error occurred, retry scheduled
                └────┬───┘
                     │ (after next_retry_at)
                     ▼
                ┌─────────┐
                │ pending │ ← Retry attempt
                └─────────┘
```


### Processing Flow

```go
1. Poll for events (every PollingInterval)
   ↓
2. Fetch batch (BatchSize limit, filtered by topicPrefix)
   ↓
3. For each event:
   ├─ Deserialize JSON data
   ├─ Call Publisher.PublishToTopic()
   ├─ If success: MarkEventProcessed()
   └─ If failure: scheduleRetry() with exponential backoff
   ↓
4. Sleep until next polling interval
```


## Retry Strategy

### Exponential Backoff

Failed events are retried with exponentially increasing delays:

```
Attempt 1: next_retry_at = now + 30s
Attempt 2: next_retry_at = now + 60s
Attempt 3: next_retry_at = now + 120s
Attempt 4+: Given up (status remains "failed")
```


### Implementation

```go
func (r *Relay[EventType]) scheduleRetry(ctx context.Context, event *repository.OutboxEvent, err error) error {
    event.RetryCount++
    event.LastError = &errMsg
    event.Status = "failed"
    
    if event.RetryCount < r.config.RetryMaxAttempts {
        // Exponential backoff: 2^retryCount * baseDelay
        backoff := r.config.RetryBackoff * time.Duration(1<<uint(event.RetryCount-1))
        nextRetry := time.Now().Add(backoff)
        event.NextRetryAt = &nextRetry
        event.Status = "pending"  // Will be retried
    }
    
    return r.db.Save(event).Error
}
```


## Metrics

The relay exposes real-time metrics for monitoring:

```go
type Metrics struct {
    EventsProcessed    int64  // Total events successfully processed
    EventsFailed       int64  // Total events that failed
    CurrentBatchSize   int64  // Size of current batch being processed
    LastProcessingTime int64  // Duration of last batch (milliseconds)
}
```


### Accessing Metrics

```go
relay := NewRelay(db, config)
metrics := relay.GetMetrics()

log.Printf("Processed: %d, Failed: %d, Batch Size: %d, Last Duration: %dms",
    metrics.EventsProcessed,
    metrics.EventsFailed,
    metrics.CurrentBatchSize,
    metrics.LastProcessingTime,
)
```


## Graceful Shutdown

The relay supports graceful shutdown to prevent data loss:

```go
relay := NewRelay(db, config)

// Start in goroutine
go relay.Start(ctx)

// ... application runs ...

// Graceful shutdown
relay.Stop()  // Waits for current batch to finish
```

**Shutdown Behavior:**

1. Sets `stopping` flag to prevent new batches
2. Waits for current batch to complete
3. Closes all goroutines
4. Returns when safe to terminate

## Best Practices

### 1. Always Use Topic Prefixes

```go
// ✅ GOOD
coreRelay := relay.NewRelay(db, publisher, processor, cleanup, config, "quote-")

// ❌ BAD - Will process ALL events!
coreRelay := relay.NewRelay(db, publisher, processor, cleanup, config, "")
```


### 2. Keep Publishers Focused

Publishers should only handle topic routing, not filtering:

```go
// ✅ GOOD - Trust the relay filtered correctly
func (p *QuotePublisher) PublishToTopic(...) error {
    switch outboxEvent.Topic {
    case "quote-v1-proposed":
        return publishQuoteProposed(...)
    case "quote-v1-accepted":
        return publishQuoteAccepted(...)
    default:
        return fmt.Errorf("unknown topic: %s", outboxEvent.Topic)
    }
}

// ❌ BAD - Redundant filtering
func (p *QuotePublisher) PublishToTopic(...) error {
    if !strings.HasPrefix(outboxEvent.Topic, "quote-") {
        return nil  // Silent failure!
    }
    // ...
}
```


### 3. Monitor Metrics

Set up alerts on:

- High `EventsFailed` rate → Indicates downstream service issues
- Increasing `CurrentBatchSize` → Relay falling behind
- Long `LastProcessingTime` → Performance degradation


### 4. Tune Configuration

For high-throughput services:

```go
config := relay.Config{
    PollingInterval: 1 * time.Second,  // More frequent
    BatchSize: 500,                    // Larger batches
}
```

For low-latency requirements:

```go
config := relay.Config{
    PollingInterval: 100 * time.Millisecond,  // Very frequent
    BatchSize: 10,                            // Small batches
}
```


## Troubleshooting

### Events Not Being Processed

**Check 1:** Verify relay is running

```go
// Ensure Start() was called
relay.Start(ctx)
```

**Check 2:** Verify topic prefix matches

```sql
SELECT topic FROM outbox WHERE processed_at IS NULL LIMIT 10;
-- Should match your topicPrefix
```

**Check 3:** Check for errors in retry events

```sql
SELECT id, topic, retry_count, last_error 
FROM outbox 
WHERE status = 'failed' 
ORDER BY inserted_at DESC 
LIMIT 10;
```


### Events Stuck in "processing" State

This indicates the relay crashed mid-processing. Events in "processing" state are eventually retried after `next_retry_at` expires.

**Manual recovery:**

```sql
UPDATE outbox 
SET status = 'pending', next_retry_at = NULL 
WHERE status = 'processing' AND processed_at IS NULL;
```


### High Failure Rate

**Check publisher implementation** for:

- Network connectivity issues
- Serialization errors
- Topic configuration mismatches

**Add debug logging:**

```go
func (p *QuotePublisher) PublishToTopic(...) error {
    log.Printf("[DEBUG] Publishing event: topic=%s, id=%s", outboxEvent.Topic, outboxEvent.ID)
    err := actualPublish(...)
    if err != nil {
        log.Printf("[ERROR] Publish failed: %v", err)
    }
    return err
}
```


## Performance Considerations

### Database Load

Each relay polls the outbox table every `PollingInterval`. To minimize load:

1. **Use appropriate indexes:**
```sql
CREATE INDEX idx_outbox_topic_processing ON outbox(topic, processed_at, inserted_at);
```

2. **Tune batch size** based on event throughput
3. **Increase polling interval** for low-volume services

### Memory Usage

Batch size directly impacts memory:

```
Memory per batch ≈ BatchSize × Average Event Size
```

For large events (>10KB), reduce `BatchSize`.

### Throughput Calculation

```
Max throughput = (BatchSize / PollingInterval) events/second
Example: 100 / 5s = 20 events/second
```

To increase throughput, either:

- Increase `BatchSize`
- Decrease `PollingInterval`
- Run multiple relay instances (with same topic prefix)


## Migration Guide

### From Publisher-Level Filtering to Database-Level

**Before:**

```go
// Publisher had filtering logic
func (p *QuotePublisher) PublishToTopic(...) error {
    if !strings.HasPrefix(topic, "quote-") {
        return nil  // Skip
    }
    // ...
}
```

**After:**

```go
// Relay constructor specifies filter
coreRelay := relay.NewRelay(
    db, publisher, processor, cleanup, config,
    "quote-",  // Filter happens at DB query level
)

// Publisher no longer needs filtering
func (p *QuotePublisher) PublishToTopic(...) error {
    // All events received are guaranteed to be quote events
    switch outboxEvent.Topic {
    // ...
    }
}
```

**Benefits:**

- ✅ No race conditions
- ✅ Better performance (fewer rows scanned)
- ✅ Simpler publisher code
- ✅ Explicit event ownership

***

## Summary

The `core/relay` package provides a production-ready implementation of the Transactional Outbox Pattern with:

- ✅ **Reliability:** Guaranteed event delivery with retries
- ✅ **Scalability:** Database-level partitioning via topic filtering
- ✅ **Observability:** Built-in metrics and logging
- ✅ **Flexibility:** Modular design with pluggable components
- ✅ **Safety:** Graceful shutdown and error handling

For service-specific implementations, see:

- `booking/relay/` - Booking service events
- `quote/relay/` - Quote service events
- `chat/relay/` - Chat service events

