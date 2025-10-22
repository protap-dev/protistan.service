-- Drop triggers and functions
DROP TRIGGER IF EXISTS trigger_update_offers_count ON booking_offers;
DROP FUNCTION IF EXISTS update_booking_offers_count();

-- Remove columns from bookings
ALTER TABLE bookings DROP COLUMN IF EXISTS offers_count;
ALTER TABLE bookings DROP COLUMN IF EXISTS is_specific_artisan;

-- Drop indexes (will be automatically dropped with table, but explicit for clarity)
DROP INDEX IF EXISTS idx_booking_offers_expires_at;
DROP INDEX IF EXISTS idx_booking_offers_status;
DROP INDEX IF EXISTS idx_booking_offers_artisan_id;
DROP INDEX IF EXISTS idx_booking_offers_booking_id;

-- Drop table
DROP TABLE IF EXISTS booking_offers;
