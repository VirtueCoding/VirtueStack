-- Remove the UNIQUE constraint on (ipv6_prefix_id, subnet_index).
-- This was added as a safety-net to prevent duplicate subnet allocations.
ALTER TABLE vm_ipv6_subnets DROP CONSTRAINT uq_ipv6_subnets_prefix_index;
