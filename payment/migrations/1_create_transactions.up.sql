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
    amount_cents BIGINT NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'NGN',
    provider VARCHAR(50) NOT NULL,
    method VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL,
    internal_ref VARCHAR(255) NOT NULL UNIQUE,
    provider_ref VARCHAR(255),
    current_attempt_id UUID,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_transactions_booking_id ON transactions(booking_id);
CREATE INDEX idx_transactions_customer_id ON transactions(customer_id);
CREATE INDEX idx_transactions_internal_ref ON transactions(internal_ref);
CREATE INDEX idx_transactions_status ON transactions(status);
CREATE INDEX idx_transactions_booking_quote_status
    ON transactions(booking_id, quote_id, status);

CREATE UNIQUE INDEX idx_transactions_single_pending_booking_quote
    ON transactions(booking_id, quote_id)
    WHERE status = 'pending' AND quote_id IS NOT NULL;

CREATE TABLE payment_attempts (
    id UUID PRIMARY KEY DEFAULT generate_uuid(),
    transaction_id UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    attempt_no INTEGER NOT NULL,
    provider VARCHAR(50) NOT NULL,
    status VARCHAR(32) NOT NULL,
    internal_ref VARCHAR(255) NOT NULL UNIQUE,
    provider_ref VARCHAR(255),
    checkout_url TEXT,
    return_url TEXT,
    return_context TEXT,
    return_context_expires_at TIMESTAMP WITH TIME ZONE,
    cancel_requested_at TIMESTAMP WITH TIME ZONE,
    cancelled_at TIMESTAMP WITH TIME ZONE,
    superseded_by_attempt_id UUID,
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_payment_attempts_transaction_id ON payment_attempts(transaction_id);
CREATE INDEX idx_payment_attempts_internal_ref ON payment_attempts(internal_ref);
CREATE INDEX idx_payment_attempts_status ON payment_attempts(status);
CREATE UNIQUE INDEX idx_payment_attempts_transaction_attempt_no ON payment_attempts(transaction_id, attempt_no);

CREATE UNIQUE INDEX idx_payment_attempts_single_live_attempt
    ON payment_attempts(transaction_id)
    WHERE status IN ('initializing', 'active', 'cancel_requested');

ALTER TABLE payment_attempts
    ADD CONSTRAINT fk_payment_attempts_superseded_by
    FOREIGN KEY (superseded_by_attempt_id) REFERENCES payment_attempts(id) ON DELETE SET NULL;

ALTER TABLE transactions
    ADD CONSTRAINT fk_transactions_current_attempt
    FOREIGN KEY (current_attempt_id) REFERENCES payment_attempts(id) ON DELETE SET NULL;

CREATE TABLE webhook_events (
    id UUID PRIMARY KEY DEFAULT generate_uuid(),
    provider VARCHAR(50) NOT NULL,
    request_id VARCHAR(255) NOT NULL,
    received_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE (provider, request_id)
);

CREATE INDEX idx_webhook_events_created_at ON webhook_events(created_at);
