-- Rollback Migration 7: Remove rates_count and related triggers

DROP TRIGGER IF EXISTS trg_artisan_rates_after_insert ON artisan_rates;
DROP TRIGGER IF EXISTS trg_artisan_rates_after_delete ON artisan_rates;
DROP TRIGGER IF EXISTS trg_artisan_rates_after_update ON artisan_rates;

DROP FUNCTION IF EXISTS set_artisan_rates_count();

DROP INDEX IF EXISTS idx_artisans_rates_count;

ALTER TABLE artisans DROP COLUMN IF EXISTS rates_count;
