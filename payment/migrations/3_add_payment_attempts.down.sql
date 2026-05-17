ALTER TABLE transactions
    DROP CONSTRAINT IF EXISTS fk_transactions_current_attempt;

DROP INDEX IF EXISTS idx_payment_attempts_single_live_attempt;
DROP INDEX IF EXISTS idx_payment_attempts_transaction_attempt_no;
DROP INDEX IF EXISTS idx_payment_attempts_status;
DROP INDEX IF EXISTS idx_payment_attempts_internal_ref;
DROP INDEX IF EXISTS idx_payment_attempts_transaction_id;

DROP TABLE IF EXISTS payment_attempts;

ALTER TABLE transactions
    DROP COLUMN IF EXISTS current_attempt_id;
