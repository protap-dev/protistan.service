-- ============================================================================
-- Outbox Pattern Migration
-- Creates the outbox table for transactional event publishing
-- ============================================================================

-- Create the outbox table for transactional event publishing
CREATE TABLE outbox (
    id BIGSERIAL PRIMARY KEY,
    topic TEXT NOT NULL,
    data JSONB NOT NULL,
    inserted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ -- Production-ready: tracks when event was processed for audit trail
);

-- Create index for efficient polling by topic and insertion order
CREATE INDEX outbox_topic_idx ON outbox (topic, inserted_at);

-- Create index for efficient querying of unprocessed events (processed_at IS NULL)
CREATE INDEX idx_outbox_processed_at ON outbox(processed_at) WHERE processed_at IS NULL;

-- Create index for efficient cleanup of old processed events (processed_at IS NOT NULL)
CREATE INDEX idx_outbox_processed_at_cleanup ON outbox(processed_at) WHERE processed_at IS NOT NULL;

-- ============================================================================
-- COMMENTS FOR DOCUMENTATION
-- ============================================================================

COMMENT ON TABLE outbox IS 'Transactional outbox table for guaranteed event publishing. Events are inserted here within database transactions and polled by the outbox relay for publishing.';
COMMENT ON COLUMN outbox.topic IS 'The topic name this event should be published to';
COMMENT ON COLUMN outbox.data IS 'The JSON payload of the event to be published';
COMMENT ON COLUMN outbox.inserted_at IS 'Timestamp when the event was inserted into the outbox';
COMMENT ON COLUMN outbox.processed_at IS 'Timestamp when the event was successfully processed by the outbox relay (NULL for unprocessed events)';
