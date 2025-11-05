-- Chat messages table with idempotency support
CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT generate_uuid(),
    thread_id UUID NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    idempotency_key VARCHAR(255) NOT NULL,
    sender_id UUID NOT NULL,
    content TEXT NOT NULL,
    message_type TEXT NOT NULL DEFAULT 'user',
    metadata JSONB,
    status TEXT NOT NULL DEFAULT 'sending',
    sent_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    db_version BIGINT NOT NULL DEFAULT 1
);

-- Indexes for efficient queries and idempotency
CREATE UNIQUE INDEX idx_messages_idempotency_key ON messages(idempotency_key);
CREATE INDEX idx_messages_thread_id ON messages(thread_id, created_at DESC);
CREATE INDEX idx_messages_sender_id ON messages(sender_id);
CREATE INDEX idx_messages_status ON messages(status);

-- Trigger to update updated_at
CREATE OR REPLACE FUNCTION update_messages_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER messages_updated_at_trigger
    BEFORE UPDATE ON messages
    FOR EACH ROW
    EXECUTE FUNCTION update_messages_updated_at();

-- Check constraint for valid status transitions
ALTER TABLE messages ADD CONSTRAINT check_message_status
    CHECK (status IN ('sending', 'sent', 'delivered', 'read', 'failed'));
    
ALTER TABLE messages ADD CONSTRAINT check_message_type
    CHECK (message_type IN ('user', 'system', 'status_update'));
