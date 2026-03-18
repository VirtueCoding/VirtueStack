-- VirtueStack Plans Slug NOT NULL Migration
-- Ensures plans.slug is NOT NULL (backfill any NULLs first)

BEGIN;

SET lock_timeout = '5s';

-- Backfill any NULL slugs from the plan name before enforcing NOT NULL
UPDATE plans SET slug = LOWER(REPLACE(name, ' ', '-')) WHERE slug IS NULL;

-- Enforce NOT NULL on the slug column
ALTER TABLE plans ALTER COLUMN slug SET NOT NULL;

COMMIT;
