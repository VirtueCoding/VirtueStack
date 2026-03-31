BEGIN;

-- Remove version and description columns from templates table
DROP INDEX IF EXISTS idx_templates_version;
ALTER TABLE templates DROP COLUMN IF EXISTS description;
ALTER TABLE templates DROP COLUMN IF EXISTS version;
ALTER TABLE templates DROP COLUMN IF EXISTS updated_at;

COMMIT;