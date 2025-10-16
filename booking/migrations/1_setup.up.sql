-- ============================================================================
-- UUID Setup Migration (Simplified)
-- Sets up basic UUID generation for the application
-- ============================================================================

-- Install standard extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Try UUIDv7 (test if function exists)
DO $$
BEGIN
    -- Test if v7 function exists (without executing)
    PERFORM pg_get_function_identity_arguments('uuid_generate_v7'::regproc);
    RAISE NOTICE 'UUID v7 is available';
EXCEPTION WHEN undefined_function THEN
    RAISE NOTICE 'UUID v7 not available, will use v4';
END $$;

-- Robust function with fallback
CREATE OR REPLACE FUNCTION generate_uuid() RETURNS UUID AS $$
BEGIN
    RETURN uuid_generate_v7();
EXCEPTION
    WHEN undefined_function THEN
        RETURN uuid_generate_v4();
END;
$$ LANGUAGE plpgsql;

-- Generic function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- COMMENTS FOR DOCUMENTATION
-- ============================================================================

COMMENT ON FUNCTION generate_uuid IS 'Generates UUIDv7 when available, falls back to UUIDv4. Provides optimal indexing performance with timestamp-based UUIDs while maintaining compatibility across environments';
COMMENT ON EXTENSION "uuid-ossp" IS 'Provides UUID generation functions including uuid_generate_v4() as fallback for environments without UUIDv7 support';
COMMENT ON FUNCTION update_updated_at_column IS 'Generic trigger function to update updated_at timestamp on record modifications';

