-- ============================================================================
-- UUID Setup Migration (Simplified)
-- Sets up basic UUID generation for the application
-- ============================================================================

-- Install fallback extension for tests (always safe)
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Simple UUID generator function (uses v4 for now)
CREATE OR REPLACE FUNCTION generate_uuid()
RETURNS UUID AS $$
BEGIN
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

COMMENT ON FUNCTION generate_uuid IS 'Generates UUIDv4 for consistent ID generation across environments';
COMMENT ON EXTENSION "uuid-ossp" IS 'Provides uuid_generate_v4 function for UUID generation';
COMMENT ON FUNCTION update_updated_at_column IS 'Generic trigger function to update updated_at timestamp';

