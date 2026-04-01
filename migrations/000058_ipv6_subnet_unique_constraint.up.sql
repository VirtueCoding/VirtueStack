-- Add UNIQUE constraint on (ipv6_prefix_id, subnet_index) to prevent duplicate subnet allocations.
-- This is a safety-net that prevents silent duplicate subnet_index values when concurrent
-- transactions both read the same max(subnet_index) before either inserts.
-- The application-level FOR UPDATE lock on the prefix row reduces but does not eliminate
-- the race condition window, so this constraint is necessary as a last line of defense.
ALTER TABLE vm_ipv6_subnets ADD CONSTRAINT uq_ipv6_subnets_prefix_index UNIQUE (ipv6_prefix_id, subnet_index);
