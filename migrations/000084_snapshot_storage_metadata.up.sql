BEGIN;

SET lock_timeout = '5s';

ALTER TABLE snapshots
    ADD COLUMN IF NOT EXISTS storage_backend VARCHAR(20) DEFAULT 'ceph';

ALTER TABLE snapshots
    ADD COLUMN IF NOT EXISTS qcow_snapshot VARCHAR(255);

UPDATE snapshots
SET storage_backend = 'ceph'
WHERE storage_backend IS NULL OR storage_backend = '';

ALTER TABLE snapshots
    ALTER COLUMN storage_backend SET NOT NULL;

COMMIT;
