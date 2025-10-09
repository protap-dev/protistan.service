-- ============================================================================
-- ROLLBACK AUTHENTICATION TABLES MIGRATION
-- ============================================================================

-- ============================================================================
-- DROP PERFORMANCE INDEXES
-- ============================================================================

-- Drop indexes in reverse order (safe with IF EXISTS)
DROP INDEX IF EXISTS idx_password_reset_tokens_token;
DROP INDEX IF EXISTS idx_email_verification_tokens_token;
DROP INDEX IF EXISTS idx_user_settings_user_id;
DROP INDEX IF EXISTS idx_user_profiles_user_id;
DROP INDEX IF EXISTS idx_users_email;

-- ============================================================================
-- DROP TABLES (REVERSE DEPENDENCY ORDER)
-- ============================================================================

-- Drop token tables first (depend on users)
DROP TABLE IF EXISTS password_reset_tokens CASCADE;
DROP TABLE IF EXISTS email_verification_tokens CASCADE;

-- Drop settings table (depends on users)
DROP TABLE IF EXISTS user_settings CASCADE;

-- Drop profile table (depends on users)
DROP TABLE IF EXISTS user_profiles CASCADE;

-- Drop main users table last
DROP TABLE IF EXISTS users CASCADE;
