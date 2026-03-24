-- Migration 000052: Fix duplicate plan slugs
-- Addresses: F-042
--
-- Migration 000032 backfilled NULL slugs using LOWER(REPLACE(name, ' ', '-'))
-- which can produce duplicates when two plans share the same name or differ
-- only in punctuation. This migration de-duplicates any existing collisions by
-- appending the first 6 characters of the plan's UUID, then enforces uniqueness.

BEGIN;

SET lock_timeout = '5s';

-- Step 1: Re-derive slugs for any plans whose slug collides with another plan.
--         The inner query finds plans that share a slug with at least one other
--         plan; we update all of them (not just one) to the deterministic
--         slug-with-id-suffix form so the result is idempotent.

UPDATE plans
SET slug = LOWER(REGEXP_REPLACE(name, '[^a-z0-9]+', '-', 'g'))
           || '-' || SUBSTRING(id::text, 1, 6)
WHERE id IN (
    SELECT id
    FROM (
        SELECT id,
               COUNT(*) OVER (PARTITION BY slug) AS slug_count
        FROM   plans
    ) sub
    WHERE slug_count > 1
);

-- Step 2: Also fix any NULLs that somehow still exist (defensive).
UPDATE plans
SET slug = LOWER(REGEXP_REPLACE(name, '[^a-z0-9]+', '-', 'g'))
           || '-' || SUBSTRING(id::text, 1, 6)
WHERE slug IS NULL;

-- Step 3: Enforce a unique index on slug so future duplicates are impossible.
--         Use IF NOT EXISTS so this is safe to re-run.
CREATE UNIQUE INDEX IF NOT EXISTS uq_plans_slug ON plans(slug);

COMMIT;
