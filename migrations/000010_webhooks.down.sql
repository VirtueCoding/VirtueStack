-- VirtueStack Webhook Delivery System - Down Migration
-- Rolls back webhooks and webhook_deliveries tables

BEGIN;

-- ============================================================================
-- DROP RLS POLICIES
-- ============================================================================

DROP POLICY IF EXISTS webhooks_customer_isolation ON webhooks;
DROP POLICY IF EXISTS webhook_deliveries_customer_isolation ON webhook_deliveries;
DROP POLICY IF EXISTS webhooks_app_all ON webhooks;
DROP POLICY IF EXISTS webhook_deliveries_app_all ON webhook_deliveries;

-- ============================================================================
-- DROP TRIGGERS
-- ============================================================================

DROP TRIGGER IF EXISTS enforce_webhook_limit ON webhooks;
DROP TRIGGER IF EXISTS webhooks_updated_at ON webhooks;
DROP TRIGGER IF EXISTS webhook_deliveries_updated_at ON webhook_deliveries;

-- ============================================================================
-- DROP VIEWS
-- ============================================================================

DROP VIEW IF EXISTS v_webhook_delivery_stats;
DROP VIEW IF EXISTS v_active_webhooks;

-- ============================================================================
-- DROP FUNCTIONS
-- ============================================================================

DROP FUNCTION IF EXISTS check_webhook_limit();
DROP FUNCTION IF EXISTS update_webhook_updated_at();

-- ============================================================================
-- DROP TABLES
-- ============================================================================

DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhooks;

COMMIT;