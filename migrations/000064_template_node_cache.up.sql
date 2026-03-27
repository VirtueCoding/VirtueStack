-- Template Node Cache: tracks which templates are cached on which nodes.
-- Ceph nodes skip caching entirely (shared pool access).
-- QCOW/LVM nodes cache templates locally via lazy pull or admin-triggered distribution.

SET lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS template_node_cache (
    template_id UUID NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
    node_id     UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    status      VARCHAR(20) NOT NULL DEFAULT 'pending',
    local_path  TEXT,
    size_bytes  BIGINT,
    synced_at   TIMESTAMPTZ,
    error_msg   TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (template_id, node_id),
    CONSTRAINT template_node_cache_status_check CHECK (
        status IN ('pending', 'downloading', 'ready', 'failed')
    )
);

CREATE INDEX idx_template_node_cache_node_id ON template_node_cache (node_id);
CREATE INDEX idx_template_node_cache_status ON template_node_cache (status);

COMMENT ON TABLE template_node_cache IS 'Tracks template distribution status across QCOW/LVM nodes. Ceph nodes use shared pool access.';
