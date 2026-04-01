BEGIN;

SET lock_timeout = '5s';

CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_type_timestamp
    ON audit_logs(actor_type, timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_success_timestamp
    ON audit_logs(success, timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_resource_timestamp
    ON audit_logs(resource_type, resource_id, timestamp DESC);

COMMIT;
