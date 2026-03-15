BEGIN;

CREATE INDEX IF NOT EXISTS idx_templates_os_family_active
    ON templates(os_family, is_active);

CREATE INDEX IF NOT EXISTS idx_templates_name
    ON templates(name);

CREATE INDEX IF NOT EXISTS idx_templates_sort_order_name
    ON templates(sort_order, name);

COMMIT;
