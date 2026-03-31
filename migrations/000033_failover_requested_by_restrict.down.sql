-- Revert failover_requests.requested_by to implicit ON DELETE behavior

BEGIN;

SET lock_timeout = '5s';

-- Revert to default (no explicit ON DELETE - PostgreSQL defaults to NO ACTION)
ALTER TABLE failover_requests
    DROP CONSTRAINT IF EXISTS failover_requests_requested_by_fkey,
    ADD CONSTRAINT failover_requests_requested_by_fkey
        FOREIGN KEY (requested_by) REFERENCES admins(id);

COMMIT;