-- VirtueStack Plans Slug NOT NULL Migration (Rollback)
-- Reverts plans.slug back to nullable

BEGIN;

ALTER TABLE plans ALTER COLUMN slug DROP NOT NULL;

COMMIT;
