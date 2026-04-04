-- Migration 000029 previously attempted to create this index with
-- CREATE INDEX CONCURRENTLY, which is incompatible with golang-migrate's
-- transaction-managed execution and caused fresh installs to fail.
--
-- Keep the canonical index in the normal migration chain using a
-- transaction-safe CREATE INDEX statement. Operators that need a
-- concurrent rebuild on an already-populated production table should
-- perform that as a manual maintenance step outside golang-migrate.

CREATE INDEX IF NOT EXISTS idx_tasks_status_created_at
    ON tasks(status, created_at DESC);
