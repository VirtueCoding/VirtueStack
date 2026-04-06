BEGIN;

SET lock_timeout = '5s';

ALTER TABLE snapshots
    DROP COLUMN IF EXISTS qcow_snapshot;

ALTER TABLE snapshots
    DROP COLUMN IF EXISTS storage_backend;

COMMIT;
