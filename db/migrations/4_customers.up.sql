-- ============================================================================
-- CUSTOMER ADDRESSES MIGRATION
-- ============================================================================

-- Add PostGIS extension for geospatial features
CREATE EXTENSION IF NOT EXISTS postgis;

-- Ensure POINT type is available (built-in PostgreSQL, no extension needed)
-- ============================================================================

-- User addresses for service delivery and location-based matching
CREATE TABLE customer_addresses (
    id UUID PRIMARY KEY DEFAULT generate_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    -- Address identification and labels
    label VARCHAR(50) NOT NULL CHECK (label IN ('home', 'office', 'other')),
    street_address TEXT NOT NULL CHECK (LENGTH(street_address) >= 5 AND LENGTH(street_address) <= 500),
    city VARCHAR(100) NOT NULL CHECK (LENGTH(city) >= 2 AND LENGTH(city) <= 100),
    state VARCHAR(100) NOT NULL CHECK (LENGTH(state) >= 2 AND LENGTH(state) <= 100),
    postal_code VARCHAR(20),
    country VARCHAR(100) DEFAULT 'Nigeria' CHECK (LENGTH(country) >= 2 AND LENGTH(country) <= 100),
    
    -- Geospatial (simple lat/long point)
    coordinates POINT, -- Format: (longitude, latitude) e.g., POINT(-74.0059 40.7128)
    
    -- Preferences
    is_default BOOLEAN DEFAULT FALSE,
    
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================================================
-- PERFORMANCE INDEXES
-- ============================================================================

-- Core lookups by user
CREATE INDEX idx_customer_addresses_user_id ON customer_addresses(user_id);

-- Quick access to defaults
CREATE INDEX idx_customer_addresses_is_default ON customer_addresses(is_default) WHERE is_default = TRUE;

-- Geospatial index for proximity queries
CREATE INDEX idx_customer_addresses_coordinates ON customer_addresses USING GIST(coordinates);

-- Combined index for address lookup by user + label
CREATE INDEX idx_customer_addresses_user_label ON customer_addresses(user_id, label);

-- Ensure only one default address per user
CREATE UNIQUE INDEX one_default_address_per_user ON customer_addresses (user_id) WHERE is_default = TRUE;

-- ============================================================================
-- AUTO-UPDATE TIMESTAMPS
-- ============================================================================

-- Reuse the generic update_updated_at_column function from 1_setup.up.sql
-- Trigger for updated_at
CREATE TRIGGER update_customer_addresses_updated_at
    BEFORE UPDATE ON customer_addresses
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- COMMENTS FOR DOCUMENTATION
-- ============================================================================

COMMENT ON TABLE customer_addresses IS 'User addresses for service delivery, matching, and location-based features';
COMMENT ON COLUMN customer_addresses.coordinates IS 'Geospatial point (longitude, latitude) for proximity calculations';
COMMENT ON COLUMN customer_addresses.is_default IS 'Flag for primary/default address per user (enforced unique)';
COMMENT ON INDEX one_default_address_per_user IS 'Business rule: Only one default address per user';
