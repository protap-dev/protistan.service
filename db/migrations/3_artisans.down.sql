-- ============================================================================
-- Rollback Artisan Service Migration
-- ============================================================================

-- Drop helper functions
DROP FUNCTION IF EXISTS search_artisans(TEXT, INTEGER);
DROP FUNCTION IF EXISTS update_artisan_search_vector();
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop tables (in reverse dependency order)
DROP TABLE IF EXISTS artisan_rates CASCADE;
DROP TABLE IF EXISTS artisan_portfolio CASCADE;
DROP TABLE IF EXISTS artisans CASCADE;
DROP TABLE IF EXISTS services CASCADE;
DROP TABLE IF EXISTS service_categories CASCADE;
