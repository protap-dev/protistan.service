CREATE UNIQUE INDEX IF NOT EXISTS idx_transactions_single_pending_booking_quote
    ON transactions(booking_id, quote_id)
    WHERE status = 'pending' AND quote_id IS NOT NULL;
