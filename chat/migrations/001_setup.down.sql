-- ============================================================================
-- ROLLBACK UUID SETUP
-- ============================================================================

-- Drop the custom functions (extensions remain for safety)
DROP FUNCTION IF EXISTS generate_uuid() CASCADE;