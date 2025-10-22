-- Drop indexes (reverse order)
DROP INDEX IF EXISTS idx_bookings_created_at;
DROP INDEX IF EXISTS idx_bookings_status;
DROP INDEX IF EXISTS idx_bookings_artisan;
DROP INDEX IF EXISTS idx_bookings_customer;

-- Drop tables (reverse order)
DROP TABLE IF EXISTS booking_status_history;
DROP TABLE IF EXISTS bookings;
