BEGIN;

CREATE TABLE IF NOT EXISTS failover_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    requested_by UUID NOT NULL REFERENCES admins(id),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    reason TEXT,
    result JSONB,
    approved_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_failover_requests_node_id ON failover_requests(node_id);
CREATE INDEX IF NOT EXISTS idx_failover_requests_status ON failover_requests(status);
CREATE INDEX IF NOT EXISTS idx_failover_requests_created_at ON failover_requests(created_at);

COMMIT;
