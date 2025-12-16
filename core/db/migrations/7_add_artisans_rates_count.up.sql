-- Migration 7: Add denormalized rates_count to artisans and maintain via triggers

-- 1) Add column
ALTER TABLE artisans
ADD COLUMN IF NOT EXISTS rates_count INTEGER NOT NULL DEFAULT 0;

-- 2) Backfill from artisan_rates
UPDATE artisans a
SET rates_count = COALESCE(src.cnt, 0)
FROM (
  SELECT artisan_id, COUNT(*) AS cnt
  FROM artisan_rates
  GROUP BY artisan_id
) AS src
WHERE a.id = src.artisan_id;

-- 3) Maintenance trigger function (recalculate for affected artisan IDs)
CREATE OR REPLACE FUNCTION set_artisan_rates_count() RETURNS TRIGGER AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    UPDATE artisans SET rates_count = (
      SELECT COUNT(*) FROM artisan_rates WHERE artisan_id = NEW.artisan_id
    ) WHERE id = NEW.artisan_id;
    RETURN NEW;
  ELSIF TG_OP = 'DELETE' THEN
    UPDATE artisans SET rates_count = (
      SELECT COUNT(*) FROM artisan_rates WHERE artisan_id = OLD.artisan_id
    ) WHERE id = OLD.artisan_id;
    RETURN OLD;
  ELSIF TG_OP = 'UPDATE' THEN
    IF NEW.artisan_id <> OLD.artisan_id THEN
      -- Recalc for both artisans if the association moved
      UPDATE artisans SET rates_count = (
        SELECT COUNT(*) FROM artisan_rates WHERE artisan_id = OLD.artisan_id
      ) WHERE id = OLD.artisan_id;
      UPDATE artisans SET rates_count = (
        SELECT COUNT(*) FROM artisan_rates WHERE artisan_id = NEW.artisan_id
      ) WHERE id = NEW.artisan_id;
    ELSE
      -- Same artisan_id, safe to recalc that one
      UPDATE artisans SET rates_count = (
        SELECT COUNT(*) FROM artisan_rates WHERE artisan_id = NEW.artisan_id
      ) WHERE id = NEW.artisan_id;
    END IF;
    RETURN NEW;
  END IF;
  RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- 4) Triggers on artisan_rates
DROP TRIGGER IF EXISTS trg_artisan_rates_after_insert ON artisan_rates;
CREATE TRIGGER trg_artisan_rates_after_insert
AFTER INSERT ON artisan_rates
FOR EACH ROW EXECUTE FUNCTION set_artisan_rates_count();

DROP TRIGGER IF EXISTS trg_artisan_rates_after_delete ON artisan_rates;
CREATE TRIGGER trg_artisan_rates_after_delete
AFTER DELETE ON artisan_rates
FOR EACH ROW EXECUTE FUNCTION set_artisan_rates_count();

DROP TRIGGER IF EXISTS trg_artisan_rates_after_update ON artisan_rates;
CREATE TRIGGER trg_artisan_rates_after_update
AFTER UPDATE OF artisan_id ON artisan_rates
FOR EACH ROW EXECUTE FUNCTION set_artisan_rates_count();

-- 5) Optional index if you plan to filter/sort by rates_count
CREATE INDEX IF NOT EXISTS idx_artisans_rates_count ON artisans(rates_count);
