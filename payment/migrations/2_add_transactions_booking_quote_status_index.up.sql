CREATE INDEX idx_transactions_booking_quote_status
    ON transactions(booking_id, quote_id, status);
