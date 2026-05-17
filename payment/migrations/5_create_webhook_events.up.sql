CREATE TABLE webhook_events (
    id UUID PRIMARY KEY DEFAULT generate_uuid(),
    provider VARCHAR(50) NOT NULL,
    request_id VARCHAR(255) NOT NULL,
    received_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE (provider, request_id)
);

CREATE INDEX idx_webhook_events_created_at ON webhook_events(created_at);
