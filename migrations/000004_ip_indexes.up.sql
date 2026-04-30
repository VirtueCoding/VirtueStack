BEGIN;

SET lock_timeout = '5s';

CREATE INDEX IF NOT EXISTS idx_ip_sets_location_ip_version
    ON ip_sets(location_id, ip_version);

CREATE INDEX IF NOT EXISTS idx_ip_sets_network
    ON ip_sets(network);

CREATE INDEX IF NOT EXISTS idx_ip_addresses_set_status
    ON ip_addresses(ip_set_id, status);

CREATE INDEX IF NOT EXISTS idx_ip_addresses_address_status
    ON ip_addresses(address, status);

CREATE INDEX IF NOT EXISTS idx_ipv6_prefixes_node_id
    ON ipv6_prefixes(node_id);

CREATE INDEX IF NOT EXISTS idx_vm_ipv6_subnets_vm_id
    ON vm_ipv6_subnets(vm_id);

CREATE INDEX IF NOT EXISTS idx_vm_ipv6_subnets_prefix_id
    ON vm_ipv6_subnets(ipv6_prefix_id);

COMMIT;
