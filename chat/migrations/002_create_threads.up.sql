-- Chat threads table
CREATE TABLE threads (
    id UUID PRIMARY KEY DEFAULT generate_uuid(),
    booking_id UUID NOT NULL,
    customer_id UUID NOT NULL,
    artisan_id UUID NOT NULL,
    last_message_at TIMESTAMPTZ,
    last_message_id UUID,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for efficient queries
CREATE UNIQUE INDEX idx_threads_booking_id ON threads(booking_id);
CREATE INDEX idx_threads_customer_id ON threads(customer_id);
CREATE INDEX idx_threads_artisan_id ON threads(artisan_id);
CREATE INDEX idx_threads_last_message_at ON threads(last_message_at DESC);

-- Trigger to update updated_at
CREATE OR REPLACE FUNCTION update_threads_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER threads_updated_at_trigger
    BEFORE UPDATE ON threads
    FOR EACH ROW
    EXECUTE FUNCTION update_threads_updated_at();