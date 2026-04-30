BEGIN;

DROP INDEX IF EXISTS idx_audit_logs_resource_timestamp;
DROP INDEX IF EXISTS idx_audit_logs_success_timestamp;
DROP INDEX IF EXISTS idx_audit_logs_actor_type_timestamp;

COMMIT;
