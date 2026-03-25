-- VirtueStack Bandwidth Tracking Migration
-- Creates bandwidth_usage and bandwidth_throttle tables
-- Reference: docs/ARCHITECTURE.md lines 720-750

BEGIN;

SET lock_timeout = '5s';

-- ============================================================================
-- BANDWIDTH USAGE TABLE
-- Tracks monthly bandwidth consumption per VM
-- ============================================================================

CREATE TABLE IF NOT EXISTS bandwidth_usage (
    vm_id UUID NOT NULL REFERENCES vms(id) ON DELETE CASCADE,
    year INTEGER NOT NULL,
    month INTEGER NOT NULL CHECK (month BETWEEN 1 AND 12),
    bytes_in BIGINT DEFAULT 0 NOT NULL,
    bytes_out BIGINT DEFAULT 0 NOT NULL,
    limit_bytes BIGINT DEFAULT 0 NOT NULL,  -- Plan's bandwidth_limit_gb * 1GB
    throttled BOOLEAN DEFAULT FALSE NOT NULL,
    throttled_at TIMESTAMPTZ,
    reset_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    
    PRIMARY KEY (vm_id, year, month)
);

-- Index for querying usage by year/month (covers current month lookups)
CREATE INDEX IF NOT EXISTS idx_bandwidth_usage_year_month
    ON bandwidth_usage(year, month);

-- Index for throttled VMs
CREATE INDEX IF NOT EXISTS idx_bandwidth_usage_throttled
    ON bandwidth_usage(vm_id)
    WHERE throttled = TRUE;

-- Index for VM lookup
CREATE INDEX IF NOT EXISTS idx_bandwidth_usage_vm
    ON bandwidth_usage(vm_id, year DESC, month DESC);

-- ============================================================================
-- BANDWIDTH THROTTLE TABLE
-- Tracks currently throttled VMs (for quick lookup)
-- ============================================================================

CREATE TABLE IF NOT EXISTS bandwidth_throttle (
    vm_id UUID PRIMARY KEY REFERENCES vms(id) ON DELETE CASCADE,
    throttled_since TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    throttle_until TIMESTAMPTZ,  -- NULL means until month end
    reason VARCHAR(100) DEFAULT 'bandwidth_limit_exceeded',
    bytes_at_throttle BIGINT NOT NULL,  -- Total bytes when throttled
    limit_at_throttle BIGINT NOT NULL,  -- Limit when throttled
    applied_by VARCHAR(50) DEFAULT 'system',  -- 'system' or admin user ID
    notes TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Index for throttle cleanup (finding expired throttles)
CREATE INDEX IF NOT EXISTS idx_bandwidth_throttle_until
    ON bandwidth_throttle(throttle_until)
    WHERE throttle_until IS NOT NULL;

-- ============================================================================
-- BANDWIDTH SNAPSHOTS TABLE
-- Periodic snapshots for detailed tracking (optional, for analytics)
-- ============================================================================

CREATE TABLE IF NOT EXISTS bandwidth_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vm_id UUID NOT NULL REFERENCES vms(id) ON DELETE CASCADE,
    year INTEGER NOT NULL,
    month INTEGER NOT NULL,
    day INTEGER NOT NULL CHECK (day BETWEEN 1 AND 31),
    hour INTEGER CHECK (hour BETWEEN 0 AND 23),
    bytes_in BIGINT DEFAULT 0 NOT NULL,
    bytes_out BIGINT DEFAULT 0 NOT NULL,
    snapshot_type VARCHAR(20) DEFAULT 'hourly' CHECK (snapshot_type IN ('hourly', 'daily')),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Index for querying snapshots by VM and time
CREATE INDEX IF NOT EXISTS idx_bandwidth_snapshots_lookup
    ON bandwidth_snapshots(vm_id, year DESC, month DESC, day DESC, hour DESC);

-- Partial index for daily snapshots only
CREATE INDEX IF NOT EXISTS idx_bandwidth_snapshots_daily
    ON bandwidth_snapshots(vm_id, year DESC, month DESC, day DESC)
    WHERE snapshot_type = 'daily';

-- ============================================================================
-- TRIGGERS
-- ============================================================================

-- Auto-update updated_at timestamp
CREATE OR REPLACE FUNCTION update_bandwidth_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER bandwidth_usage_updated_at
    BEFORE UPDATE ON bandwidth_usage
    FOR EACH ROW
    EXECUTE FUNCTION update_bandwidth_updated_at();

CREATE OR REPLACE TRIGGER bandwidth_throttle_updated_at
    BEFORE UPDATE ON bandwidth_throttle
    FOR EACH ROW
    EXECUTE FUNCTION update_bandwidth_updated_at();

-- ============================================================================
-- VIEWS
-- ============================================================================

-- Current month bandwidth usage view
CREATE OR REPLACE VIEW v_bandwidth_current AS
SELECT 
    bu.vm_id,
    v.hostname,
    c.name as customer_name,
    bu.year,
    bu.month,
    bu.bytes_in,
    bu.bytes_out,
    bu.bytes_in + bu.bytes_out as total_bytes,
    bu.limit_bytes,
    CASE 
        WHEN bu.limit_bytes > 0 THEN 
            ROUND(((bu.bytes_in + bu.bytes_out)::numeric / bu.limit_bytes) * 100, 2)
        ELSE 0 
    END as usage_percent,
    bu.throttled,
    bu.throttled_at,
    p.bandwidth_limit_gb as plan_limit_gb
FROM bandwidth_usage bu
JOIN vms v ON bu.vm_id = v.id
JOIN customers c ON v.customer_id = c.id
JOIN plans p ON v.plan_id = p.id
WHERE bu.year = EXTRACT(YEAR FROM CURRENT_DATE)
AND bu.month = EXTRACT(MONTH FROM CURRENT_DATE);

-- Throttled VMs view
CREATE OR REPLACE VIEW v_bandwidth_throttled AS
SELECT 
    bt.vm_id,
    v.hostname,
    c.name as customer_name,
    c.email as customer_email,
    bt.throttled_since,
    bt.throttle_until,
    bt.reason,
    bt.bytes_at_throttle,
    bt.limit_at_throttle,
    bt.applied_by
FROM bandwidth_throttle bt
JOIN vms v ON bt.vm_id = v.id
JOIN customers c ON v.customer_id = c.id;

-- ============================================================================
-- RLS POLICIES
-- ============================================================================

-- Enable RLS on bandwidth tables
ALTER TABLE bandwidth_usage ENABLE ROW LEVEL SECURITY;
ALTER TABLE bandwidth_throttle ENABLE ROW LEVEL SECURITY;
ALTER TABLE bandwidth_snapshots ENABLE ROW LEVEL SECURITY;

-- App user can see all (for internal operations)
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'bandwidth_usage' AND policyname = 'bandwidth_usage_app_all') THEN
        CREATE POLICY bandwidth_usage_app_all ON bandwidth_usage
            FOR ALL TO app_user USING (true);
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'bandwidth_throttle' AND policyname = 'bandwidth_throttle_app_all') THEN
        CREATE POLICY bandwidth_throttle_app_all ON bandwidth_throttle
            FOR ALL TO app_user USING (true);
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'bandwidth_snapshots' AND policyname = 'bandwidth_snapshots_app_all') THEN
        CREATE POLICY bandwidth_snapshots_app_all ON bandwidth_snapshots
            FOR ALL TO app_user USING (true);
    END IF;
END $$;

-- Customer isolation: customers can only see their own VMs' bandwidth
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'bandwidth_usage' AND policyname = 'bandwidth_usage_customer_isolation') THEN
        CREATE POLICY bandwidth_usage_customer_isolation ON bandwidth_usage
            FOR SELECT TO app_customer
            USING (vm_id IN (
                SELECT id FROM vms WHERE customer_id = current_setting('app.current_customer_id')::UUID
            ));
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies WHERE tablename = 'bandwidth_throttle' AND policyname = 'bandwidth_throttle_customer_isolation') THEN
        CREATE POLICY bandwidth_throttle_customer_isolation ON bandwidth_throttle
            FOR SELECT TO app_customer
            USING (vm_id IN (
                SELECT id FROM vms WHERE customer_id = current_setting('app.current_customer_id')::UUID
            ));
    END IF;
END $$;

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON TABLE bandwidth_usage IS 'Monthly bandwidth consumption tracking per VM';
COMMENT ON TABLE bandwidth_throttle IS 'Currently throttled VMs (bandwidth limit exceeded)';
COMMENT ON TABLE bandwidth_snapshots IS 'Periodic bandwidth snapshots for analytics';

COMMENT ON COLUMN bandwidth_usage.bytes_in IS 'Total bytes received (inbound traffic)';
COMMENT ON COLUMN bandwidth_usage.bytes_out IS 'Total bytes transmitted (outbound traffic)';
COMMENT ON COLUMN bandwidth_usage.limit_bytes IS 'Plan bandwidth limit in bytes (bandwidth_limit_gb * 1073741824)';
COMMENT ON COLUMN bandwidth_usage.throttled IS 'Whether VM is currently throttled to 5Mbps';

COMMENT ON COLUMN bandwidth_throttle.throttled_since IS 'When throttling was applied';
COMMENT ON COLUMN bandwidth_throttle.throttle_until IS 'When throttling expires (NULL = end of month)';
COMMENT ON COLUMN bandwidth_throttle.bytes_at_throttle IS 'Total bytes consumed when throttled';

-- ============================================================================
-- PERMISSIONS
-- ============================================================================

GRANT SELECT, INSERT, UPDATE, DELETE ON bandwidth_usage TO app_user;
GRANT SELECT, INSERT, UPDATE, DELETE ON bandwidth_throttle TO app_user;
GRANT SELECT, INSERT, UPDATE, DELETE ON bandwidth_snapshots TO app_user;

GRANT SELECT ON bandwidth_usage TO app_customer;
GRANT SELECT ON bandwidth_throttle TO app_customer;

COMMIT;
