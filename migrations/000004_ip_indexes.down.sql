BEGIN;

DROP INDEX IF EXISTS idx_vm_ipv6_subnets_prefix_id;
DROP INDEX IF EXISTS idx_vm_ipv6_subnets_vm_id;
DROP INDEX IF EXISTS idx_ipv6_prefixes_node_id;
DROP INDEX IF EXISTS idx_ip_addresses_address_status;
DROP INDEX IF EXISTS idx_ip_addresses_set_status;
DROP INDEX IF EXISTS idx_ip_sets_network;
DROP INDEX IF EXISTS idx_ip_sets_location_ip_version;

COMMIT;
