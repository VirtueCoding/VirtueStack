-- Migration: ISO Upload Tracking
-- Adds iso_uploads table for tracking customer ISO uploads per VM.
-- This enables database-enforced ISO limits for multi-instance deployments.
-- Previously, the ISO limit was enforced in-memory which doesn't work
-- when multiple controller instances are behind a load balancer.

BEGIN;

SET lock_timeout = '5s';

-- Create iso_uploads table to track uploaded ISO files
-- This enables accurate ISO counting per VM across controller instances
CREATE TABLE iso_uploads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vm_id UUID NOT NULL REFERENCES vms(id) ON DELETE CASCADE,
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    file_name VARCHAR(255) NOT NULL,
    file_size BIGINT NOT NULL,
    sha256 VARCHAR(64) NOT NULL,
    storage_path TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),

    -- Ensure one ISO per (vm_id, file_name) combination
    CONSTRAINT uq_iso_uploads_vm_file UNIQUE (vm_id, file_name)
);

-- Index for counting ISOs per VM (used by ISO limit enforcement)
CREATE INDEX idx_iso_uploads_vm_id ON iso_uploads(vm_id);
CREATE INDEX idx_iso_uploads_customer_id ON iso_uploads(customer_id);

-- Enable RLS on iso_uploads
ALTER TABLE iso_uploads ENABLE ROW LEVEL SECURITY;

-- RLS policy: customers can only see their own ISO uploads
CREATE POLICY customer_iso_uploads ON iso_uploads FOR ALL TO app_customer
    USING (customer_id = current_setting('app.current_customer_id')::UUID);

-- Admins can see all ISO uploads
CREATE POLICY admin_iso_uploads ON iso_uploads FOR ALL TO app_user
    USING (true);

-- Grant permissions
GRANT SELECT, INSERT, DELETE ON iso_uploads TO app_user;
GRANT SELECT, INSERT, DELETE ON iso_uploads TO app_customer;

COMMENT ON TABLE iso_uploads IS 'Tracks ISO files uploaded by customers for attachment to VMs';
COMMENT ON COLUMN iso_uploads.vm_id IS 'The VM that owns this ISO';
COMMENT ON COLUMN iso_uploads.customer_id IS 'The customer who uploaded the ISO';
COMMENT ON COLUMN iso_uploads.file_name IS 'Original filename (sanitized)';
COMMENT ON COLUMN iso_uploads.sha256 IS 'SHA-256 checksum of the file contents';
COMMENT ON COLUMN iso_uploads.storage_path IS 'Path to the ISO file on disk';

COMMIT;
