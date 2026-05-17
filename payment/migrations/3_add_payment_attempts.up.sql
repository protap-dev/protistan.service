ALTER TABLE transactions
    ADD COLUMN IF NOT EXISTS current_attempt_id UUID;

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
