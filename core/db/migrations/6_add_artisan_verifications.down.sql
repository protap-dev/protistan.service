-- Migration 6: Rollback artisan_verifications table and availability_status column
-- This migration removes the artisan_verifications table and availability_status column
-- and restores the original verified boolean behavior

-- Drop indexes
DROP INDEX IF EXISTS idx_artisan_verifications_artisan_id;
DROP INDEX IF EXISTS idx_artisan_verifications_status;
DROP INDEX IF EXISTS idx_artisan_verifications_updated_at;
DROP INDEX IF EXISTS idx_artisans_availability_status;

-- Drop constraints
ALTER TABLE artisan_verifications DROP CONSTRAINT IF EXISTS chk_verification_status;
ALTER TABLE artisans DROP CONSTRAINT IF EXISTS chk_availability_status;

-- Drop the artisan_verifications table
DROP TABLE IF EXISTS artisan_verifications;

-- Remove the availability_status column from artisans table
ALTER TABLE artisans DROP COLUMN IF EXISTS availability_status;

-- Note: The original 'verified' boolean column still exists in the artisans table
-- and will continue to work as before after this rollback
