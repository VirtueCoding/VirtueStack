-- Migration 000049: Document bandwidth_throttle schema redundancy
-- Addresses: F-109
--
-- TECH DEBT: bandwidth_throttle duplicates throttle state that is already
-- tracked in bandwidth_usage (is_throttled, throttled_since, throttle_speed_mbps).
-- Having two sources of truth for throttle state risks inconsistency if one
-- table is updated without the other.
--
-- Recommended future action:
--   1. Decide which table is the authoritative source for throttle state.
--   2. Remove the duplicated columns from the other table.
--   3. Update all application code to read/write only the authoritative source.
--
-- This migration only records the comment; no schema changes are made because
-- both tables may already have data in production.

BEGIN;

SET lock_timeout = '5s';

COMMENT ON TABLE bandwidth_throttle IS
    'Records throttle events for VMs that exceeded their bandwidth limit. '
    'TECH DEBT (F-109): throttle state (is_throttled, throttled_since, '
    'throttle_speed_mbps) is duplicated in bandwidth_usage. A future migration '
    'should consolidate these into a single authoritative source and remove '
    'the redundant columns.';

COMMIT;
