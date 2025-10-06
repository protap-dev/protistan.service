-- ============================================================================
-- ROLLBACK CUSTOMER ADDRESSES MIGRATION
-- ============================================================================

-- ============================================================================
-- DROP PERFORMANCE INDEXES (REVERSE ORDER)
-- ============================================================================

-- Drop indexes in reverse order (safe with IF EXISTS)
DROP INDEX IF EXISTS idx_customer_addresses_user_label;
-- DROP INDEX IF EXISTS idx_customer_addresses_coordinates; -- (Uncomment when geospatial features added)
DROP INDEX IF EXISTS idx_customer_addresses_is_default;
DROP INDEX IF EXISTS idx_customer_addresses_user_id;

-- ============================================================================
-- DROP TABLES AND TRIGGERS
-- ============================================================================

-- Drop trigger first (depends on table)
DROP TRIGGER IF EXISTS update_customer_addresses_updated_at ON customer_addresses;

-- Drop the main table (CASCADE handles foreign key constraints)
DROP TABLE IF EXISTS customer_addresses CASCADE;
