-- ============================================================================
-- Artisan Service Migration
-- Implements artisan profiles, portfolios, rates, and services with FTS
-- ============================================================================

-- Ensure UUID generation function exists (handled in 0_uuid_setup, but idempotent reference)
-- ============================================================================

-- ============================================================================
-- SERVICE CATEGORIES & SERVICES
-- ============================================================================

-- Service categories for artisan classification
CREATE TABLE service_categories (
    id UUID PRIMARY KEY DEFAULT generate_uuid(),
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Individual services within categories
CREATE TABLE services (
    id UUID PRIMARY KEY DEFAULT generate_uuid(),
    category_id UUID NOT NULL REFERENCES service_categories(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    base_price_cents BIGINT CHECK (base_price_cents >= 0),
    currency TEXT DEFAULT 'NGN' CHECK (currency IN ('NGN', 'USD', 'GBP', 'EUR')),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(category_id, name)
);

-- ============================================================================
-- ARTISAN PROFILES
-- ============================================================================

-- Core artisan profiles (matches ArtisanProfile interface)
CREATE TABLE artisans (
    id UUID PRIMARY KEY DEFAULT generate_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_ids UUID[] NOT NULL DEFAULT '{}', 
    bio TEXT NOT NULL,
    years_experience INTEGER NOT NULL CHECK (years_experience >= 0 AND years_experience <= 70),
    languages TEXT[] DEFAULT '{}',
    rating DECIMAL(3,2) DEFAULT 0.00 CHECK (rating >= 0 AND rating <= 5),
    reviews_count INTEGER DEFAULT 0 CHECK (reviews_count >= 0),
    verified BOOLEAN DEFAULT FALSE,
    max_travel_distance_km DECIMAL(10,2) DEFAULT 50 CHECK (max_travel_distance_km >= 0 AND max_travel_distance_km <= 500),
    avatar_url TEXT,
    
    -- Location for customer matching (coordinates as POINT: longitude latitude)
    coordinates POINT,
    
    -- Coarse location fallback (for basic matching if coordinates missing)
    preferred_city VARCHAR(100),
    preferred_state VARCHAR(100),
    preferred_country VARCHAR(100) DEFAULT 'Nigeria' CHECK (LENGTH(preferred_country) >= 2 AND LENGTH(preferred_country) <= 100),
    
    -- Full-text search column
    search_vector TSVECTOR,
    
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    
    UNIQUE(user_id) -- One artisan profile per user
);

-- ============================================================================
-- ARTISAN PORTFOLIO
-- ============================================================================

-- Portfolio items showcasing artisan work
CREATE TABLE artisan_portfolio (
    id UUID PRIMARY KEY DEFAULT generate_uuid(),
    artisan_id UUID NOT NULL REFERENCES artisans(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT,
    image_url TEXT NOT NULL, -- Primary portfolio image
    category_id UUID REFERENCES service_categories(id) ON DELETE SET NULL,
    display_order INTEGER DEFAULT 0, -- For sorting portfolio items
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    
    CHECK (LENGTH(title) >= 3 AND LENGTH(title) <= 200),
    CHECK (LENGTH(description) <= 1000)
);

-- ============================================================================
-- ARTISAN RATES
-- ============================================================================

-- Artisan-specific rates for services they offer
CREATE TABLE artisan_rates (
    id UUID PRIMARY KEY DEFAULT generate_uuid(),
    artisan_id UUID NOT NULL REFERENCES artisans(id) ON DELETE CASCADE,
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    hourly_rate_cents BIGINT CHECK (hourly_rate_cents >= 0),
    minimum_charge_cents BIGINT CHECK (minimum_charge_cents >= 0),
    currency TEXT DEFAULT 'NGN' CHECK (currency IN ('NGN', 'USD', 'GBP', 'EUR')),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    
    UNIQUE(artisan_id, service_id), -- One rate per service per artisan
    CHECK (minimum_charge_cents IS NULL OR hourly_rate_cents IS NULL OR minimum_charge_cents <= hourly_rate_cents * 800) -- Minimum shouldn't exceed 8 hours (in cents)
);

-- ============================================================================
-- PERFORMANCE INDEXES
-- ============================================================================

-- Artisan indexes
CREATE INDEX idx_artisans_user_id ON artisans(user_id);
CREATE INDEX idx_artisans_category_ids ON artisans USING GIN(category_ids);
CREATE INDEX idx_artisans_rating ON artisans(rating DESC) WHERE rating > 0;
CREATE INDEX idx_artisans_verified ON artisans(verified) WHERE verified = true;
CREATE INDEX idx_artisans_experience ON artisans(years_experience DESC);
CREATE INDEX idx_artisans_reviews_count ON artisans(reviews_count DESC);

-- Location indexes for matching
CREATE INDEX idx_artisans_coordinates ON artisans USING GIST(coordinates);
CREATE INDEX idx_artisans_location_coarse ON artisans(preferred_city, preferred_state) WHERE preferred_city IS NOT NULL;

-- Full-text search index (GIN for best FTS performance)
CREATE INDEX idx_artisans_search_vector ON artisans USING GIN(search_vector);

-- Service indexes
CREATE INDEX idx_services_category_id ON services(category_id);
CREATE INDEX idx_service_categories_name ON service_categories(name);

-- Portfolio indexes
CREATE INDEX idx_portfolio_artisan_id ON artisan_portfolio(artisan_id);
CREATE INDEX idx_portfolio_category_id ON artisan_portfolio(category_id);
CREATE INDEX idx_portfolio_display_order ON artisan_portfolio(artisan_id, display_order);

-- Rates indexes
CREATE INDEX idx_rates_artisan_id ON artisan_rates(artisan_id);
CREATE INDEX idx_rates_service_id ON artisan_rates(service_id);
CREATE INDEX idx_rates_hourly_rate ON artisan_rates(hourly_rate_cents) WHERE hourly_rate_cents IS NOT NULL;

-- ============================================================================
-- FULL-TEXT SEARCH TRIGGERS
-- ============================================================================

-- Function to update search vector for artisans (includes location for FTS matching)
CREATE OR REPLACE FUNCTION update_artisan_search_vector() 
RETURNS TRIGGER AS $$
BEGIN
    NEW.search_vector := 
        setweight(to_tsvector('english', COALESCE(NEW.bio, '')), 'A') ||
        setweight(to_tsvector('english', COALESCE(array_to_string(NEW.languages, ' '), '')), 'B') ||
        setweight(to_tsvector('english', 
            COALESCE(NEW.preferred_city, '') || ' ' || 
            COALESCE(NEW.preferred_state, '') || ' ' || 
            COALESCE(NEW.preferred_country, '')
        ), 'C');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to auto-update search vector on insert/update
CREATE TRIGGER artisan_search_vector_update
    BEFORE INSERT OR UPDATE OF bio, languages, preferred_city, preferred_state, preferred_country
    ON artisans
    FOR EACH ROW
    EXECUTE FUNCTION update_artisan_search_vector();

-- ============================================================================
-- AUTO-UPDATE TIMESTAMPS
-- ============================================================================

--  Reuse the generic update_updated_at_column function from 1_setup.up.sql

-- Triggers for updated_at
CREATE TRIGGER update_artisans_updated_at
    BEFORE UPDATE ON artisans
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_portfolio_updated_at
    BEFORE UPDATE ON artisan_portfolio
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_rates_updated_at
    BEFORE UPDATE ON artisan_rates
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- SEED DATA - SERVICE CATEGORIES
-- ============================================================================

INSERT INTO service_categories (name, description) VALUES
    ('Plumbing', 'Water systems, pipes, fixtures installation and repair'),
    ('Electrical', 'Electrical systems, wiring, fixtures installation and maintenance'),
    ('Carpentry', 'Wood working, furniture making, structural carpentry'),
    ('Painting', 'Interior and exterior painting, decorative finishes'),
    ('Cleaning', 'Home and office cleaning services'),
    ('Gardening', 'Landscaping and garden maintenance'),
    ('Appliance Repair', 'Home appliance installation and repair'),
    ('Moving Services', 'Local moving and transportation'),
    ('Masonry', 'Brickwork, concrete, and stone construction'),
    ('Chef', 'Cooking and food preparation services'),
    ('Tailoring', 'Clothing design, alteration, and textile work'),
    ('Auto Mechanic', 'Vehicle maintenance and repair services')
ON CONFLICT (name) DO NOTHING;

-- ============================================================================
-- HELPER FUNCTIONS FOR FTS AND LOCATION MATCHING
-- ============================================================================

-- Function to search artisans by text query
CREATE OR REPLACE FUNCTION search_artisans(
    search_query TEXT,
    max_results INTEGER DEFAULT 50
)
RETURNS TABLE (
    artisan_id UUID,
    user_id UUID,
    bio TEXT,
    rating DECIMAL,
    reviews_count INTEGER,
    verified BOOLEAN,
    rank REAL
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        a.id,
        a.user_id,
        a.bio,
        a.rating,
        a.reviews_count,
        a.verified,
        ts_rank(a.search_vector, plainto_tsquery('english', search_query)) as rank
    FROM artisans a
    WHERE a.search_vector @@ plainto_tsquery('english', search_query)
    ORDER BY rank DESC, a.rating DESC, a.reviews_count DESC
    LIMIT max_results;
END;
$$ LANGUAGE plpgsql STABLE;

-- Haversine distance function (km) for proximity matching - no extensions needed
CREATE OR REPLACE FUNCTION haversine_distance(
    lon1 DECIMAL, lat1 DECIMAL,  -- Point 1 (e.g., customer)
    lon2 DECIMAL, lat2 DECIMAL   -- Point 2 (e.g., artisan)
)
RETURNS DECIMAL AS $$
DECLARE
    r DECIMAL := 6371;  -- Earth's radius in km
    dlon DECIMAL := radians(lon2 - lon1);
    dlat DECIMAL := radians(lat2 - lat1);
    a DECIMAL;
    c DECIMAL;
BEGIN
    a := sin(dlat / 2) * sin(dlat / 2) + cos(radians(lat1)) * cos(radians(lat2)) * sin(dlon / 2) * sin(dlon / 2);
    c := 2 * atan2(sqrt(a), sqrt(1 - a));
    RETURN r * c;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Helper to extract lat/lon from POINT (uses ST_X/ST_Y for compatibility)
CREATE OR REPLACE FUNCTION extract_point_coords(p POINT)
RETURNS TABLE(lon DECIMAL, lat DECIMAL) AS $$
BEGIN
    lon := ST_X(p);  -- Longitude
    lat := ST_Y(p);  -- Latitude
    RETURN NEXT;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- ============================================================================
-- COMMENTS FOR DOCUMENTATION
-- ============================================================================

COMMENT ON TABLE artisans IS 'Core artisan profiles with FTS and location for customer matching';
COMMENT ON TABLE artisan_portfolio IS 'Portfolio items showcasing artisan work';
COMMENT ON TABLE artisan_rates IS 'Service-specific rates set by artisans';
COMMENT ON TABLE service_categories IS 'Artisan service categories';
COMMENT ON TABLE services IS 'Individual services within categories';
COMMENT ON COLUMN artisans.search_vector IS 'Full-text search vector for bio, languages, and location';
COMMENT ON COLUMN artisans.category_ids IS 'Array of service category IDs the artisan provides';
COMMENT ON COLUMN artisans.coordinates IS 'Base location (POINT: longitude latitude) for proximity matching';
COMMENT ON COLUMN artisans.max_travel_distance_km IS 'Maximum distance artisan willing to travel (km)';
COMMENT ON COLUMN artisan_rates.minimum_charge_cents IS 'Minimum charge regardless of time spent (in cents)';
COMMENT ON FUNCTION search_artisans IS 'Full-text search for artisans with ranking';
COMMENT ON FUNCTION haversine_distance IS 'Calculate distance between two lat/lon points in km';
