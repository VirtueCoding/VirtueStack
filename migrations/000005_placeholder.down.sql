BEGIN;

DROP INDEX IF EXISTS idx_templates_sort_order_name;
DROP INDEX IF EXISTS idx_templates_name;
DROP INDEX IF EXISTS idx_templates_os_family_active;

COMMIT;
