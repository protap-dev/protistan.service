-- Remove completion handshake status
--encore:service=booking
ALTER TABLE bookings DROP CONSTRAINT IF EXISTS valid_status;

ALTER TABLE bookings
    ADD CONSTRAINT valid_status CHECK (
        status IN (
            'requested',
            'offer_pending',
            'offer_rejected',
            'assigned',
            'pending_quote',
            'quote_proposed',
            'quote_accepted',
            'quote_rejected',
            'payment_pending',
            'confirmed',
            'enroute',
            'in_progress',
            'completed',
            'cancelled',
            'closed'
        )
    );
