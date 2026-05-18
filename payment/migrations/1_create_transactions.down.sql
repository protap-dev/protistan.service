ALTER TABLE transactions
    DROP CONSTRAINT IF EXISTS fk_transactions_current_attempt;

DROP TABLE IF EXISTS webhook_events;
DROP TABLE IF EXISTS payment_attempts;
DROP TABLE IF EXISTS transactions;
DROP FUNCTION IF EXISTS generate_uuid;
