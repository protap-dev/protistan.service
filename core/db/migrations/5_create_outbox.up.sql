-- ============================================================================
-- Outbox Pattern Migration
-- Creates the outbox table for transactional event publishing
-- This is the single source of truth for the outbox schema.
-- ============================================================================

-- Create the outbox table for transactional event publishing
CREATE TABLE outbox (
    id UUID PRIMARY KEY DEFAULT generate_uuid(),
    topic TEXT NOT NULL,
    data JSONB NOT NULL,
    inserted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    next_retry_at TIMESTAMPTZ,
    retry_count INT NOT NULL DEFAULT 0,
    last_error TEXT,
    status TEXT NOT NULL DEFAULT 'pending'
);

-- Create index for efficient polling by topic and insertion order
CREATE INDEX outbox_topic_idx ON outbox (topic, inserted_at);

-- Create index for efficient cleanup of old processed events (processed_at IS NOT NULL)
CREATE INDEX idx_outbox_processed_at_cleanup ON outbox(processed_at) WHERE processed_at IS NOT NULL;

-- Create index for efficient querying of events that need retry (processed_at IS NULL)
CREATE INDEX IF NOT EXISTS idx_outbox_next_retry ON outbox(next_retry_at) WHERE processed_at IS NULL;

-- Create index for efficient polling by workers
CREATE INDEX idx_outbox_poll_query ON outbox (status, next_retry_at, inserted_at) WHERE processed_at IS NULL;

-- ============================================================================
-- COMMENTS FOR DOCUMENTATION
-- ============================================================================

COMMENT ON TABLE outbox IS 'Transactional outbox table for guaranteed event publishing. Events are inserted here within database transactions and polled by the outbox relay for publishing.';
COMMENT ON COLUMN outbox.id IS 'Unique identifier for the outbox event (UUID)';
COMMENT ON COLUMN outbox.topic IS 'The topic name this event should be published to';
COMMENT ON COLUMN outbox.data IS 'The JSON payload of the event to be published';
COMMENT ON COLUMN outbox.inserted_at IS 'Timestamp when the event was inserted into the outbox';
COMMENT ON COLUMN outbox.processed_at IS 'Timestamp when the event was successfully processed by the outbox relay (NULL for unprocessed events)';
COMMENT ON COLUMN outbox.next_retry_at IS 'Timestamp for the next retry attempt in case of failure';
COMMENT ON COLUMN outbox.retry_count IS 'Number of times a retry has been attempted';
COMMENT ON COLUMN outbox.last_error IS 'The last error message recorded during a failed processing attempt';
COMMENT ON COLUMN outbox.status IS 'The current status of the event (e.g., pending, processing, failed)';
