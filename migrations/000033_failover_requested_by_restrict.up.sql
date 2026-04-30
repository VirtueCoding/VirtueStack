-- Add explicit ON DELETE RESTRICT to failover_requests.requested_by FK
-- QG-07: Document intent - failover requests should not exist without an admin

BEGIN;

SET lock_timeout = '5s';

-- Drop and recreate the constraint with explicit ON DELETE RESTRICT
-- This documents that failover requests must have a valid admin author
ALTER TABLE failover_requests
    DROP CONSTRAINT IF EXISTS failover_requests_requested_by_fkey,
    ADD CONSTRAINT failover_requests_requested_by_fkey
        FOREIGN KEY (requested_by) REFERENCES admins(id) ON DELETE RESTRICT;

COMMIT;