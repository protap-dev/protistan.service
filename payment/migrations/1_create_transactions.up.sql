CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Robust function with fallback
CREATE OR REPLACE FUNCTION generate_uuid() RETURNS UUID AS $$
BEGIN
    RETURN uuid_generate_v7();
EXCEPTION
    WHEN undefined_function THEN
        RETURN uuid_generate_v4();
END;
$$ LANGUAGE plpgsql;

CREATE TABLE transactions (
    id UUID PRIMARY KEY DEFAULT generate_uuid(),
    customer_id UUID NOT NULL,
    booking_id UUID NOT NULL,
    quote_id UUID,
    amount DECIMAL(15, 2) NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'NGN',
    provider VARCHAR(50) NOT NULL,
    method VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL,
    internal_ref VARCHAR(255) NOT NULL UNIQUE,
    provider_ref VARCHAR(255),
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_transactions_booking_id ON transactions(booking_id);
CREATE INDEX idx_transactions_customer_id ON transactions(customer_id);
CREATE INDEX idx_transactions_internal_ref ON transactions(internal_ref);
CREATE INDEX idx_transactions_status ON transactions(status);
