-- Create booking_offers table
CREATE TABLE booking_offers (
    id UUID PRIMARY KEY DEFAULT generate_uuid(),
    booking_id UUID NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    artisan_id UUID NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    offered_by UUID NOT NULL,
    offered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    responded_at TIMESTAMPTZ,
    reject_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    CONSTRAINT valid_offer_status CHECK (
        status IN ('pending', 'accepted', 'rejected', 'expired', 'cancelled')
    ),
    
    -- Prevent duplicate pending offers to same artisan for same booking
    -- When status changes, this allows a new record
    CONSTRAINT unique_pending_offer UNIQUE (booking_id, artisan_id, status)
);

-- Indexes for performance
CREATE INDEX idx_booking_offers_booking_id ON booking_offers(booking_id);
CREATE INDEX idx_booking_offers_artisan_id ON booking_offers(artisan_id);
CREATE INDEX idx_booking_offers_status ON booking_offers(status);
CREATE INDEX idx_booking_offers_expires_at ON booking_offers(expires_at) WHERE status = 'pending';

-- Add offer tracking columns to bookings table
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS is_specific_artisan BOOLEAN DEFAULT false;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS offers_count INTEGER DEFAULT 0;

-- Trigger to automatically update offers_count when offer is created
CREATE OR REPLACE FUNCTION update_booking_offers_count()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE bookings 
    SET offers_count = (
        SELECT COUNT(*) FROM booking_offers WHERE booking_id = NEW.booking_id
    )
    WHERE id = NEW.booking_id;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_offers_count
AFTER INSERT ON booking_offers
FOR EACH ROW
EXECUTE FUNCTION update_booking_offers_count();
