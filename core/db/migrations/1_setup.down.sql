-- ============================================================================
-- ROLLBACK UUID SETUP
-- ============================================================================

-- Drop the custom functions (extensions remain for safety)
DROP FUNCTION IF EXISTS generate_uuid() CASCADE;
DROP FUNCTION IF EXISTS update_updated_at_column() CASCADE;
