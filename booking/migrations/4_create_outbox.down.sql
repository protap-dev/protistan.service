-- ============================================================================
-- Outbox Pattern Migration (Down)
-- Drops the outbox table
-- ============================================================================

-- Drop the outbox table
DROP TABLE IF EXISTS outbox;

-- ============================================================================
-- COMMENTS FOR DOCUMENTATION
-- ============================================================================

COMMENT ON TABLE outbox IS 'This migration has been rolled back';
