-- Safety net: ensure a DEFAULT partition exists to catch any audit_log inserts
-- that fall outside explicitly created monthly partitions.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_class c
    JOIN pg_inherits i ON i.inhrelid = c.oid
    JOIN pg_class p ON i.inhparent = p.oid
    WHERE p.relname = 'audit_logs'
      AND pg_get_expr(c.relpartbound, c.oid) = 'DEFAULT'
  ) THEN
    EXECUTE 'CREATE TABLE audit_logs_default PARTITION OF audit_logs DEFAULT';
  END IF;
END $$;
