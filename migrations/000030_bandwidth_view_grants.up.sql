-- Grant SELECT on bandwidth views to app roles.
-- Views do not automatically inherit table grants; explicit GRANT is required
-- so app_user (internal operations) and app_customer (customer-facing reads)
-- can query v_bandwidth_current and v_bandwidth_throttled.
BEGIN;

GRANT SELECT ON v_bandwidth_current TO app_user;
GRANT SELECT ON v_bandwidth_throttled TO app_user;

-- Customers may view their own bandwidth usage via the view (RLS on underlying
-- tables already restricts rows to the customer's own VMs).
GRANT SELECT ON v_bandwidth_current TO app_customer;

COMMIT;
