-- Migration: ISO Upload Tracking (down)
-- Removes iso_uploads table

BEGIN;

SET lock_timeout = '5s';

-- Drop policies first
DROP POLICY IF EXISTS admin_iso_uploads ON iso_uploads;
DROP POLICY IF EXISTS customer_iso_uploads ON iso_uploads;

-- Disable RLS
ALTER TABLE iso_uploads DISABLE ROW LEVEL SECURITY;

-- Drop table
DROP TABLE IF EXISTS iso_uploads;

COMMIT;
