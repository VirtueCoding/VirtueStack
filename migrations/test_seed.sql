-- =============================================================================
-- VirtueStack E2E Test Seed Data
-- =============================================================================
-- Canonical seed data for Playwright E2E runs.
-- Apply after migrations:
--   docker exec -i virtuestack-postgres psql -U virtuestack -d virtuestack < migrations/test_seed.sql
-- =============================================================================

SET lock_timeout = '5s';

BEGIN;

-- Clean up existing E2E fixtures.
DELETE FROM backups
WHERE id IN (
  'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa01',
  'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa02',
  'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa03'
);

DELETE FROM snapshots
WHERE id IN (
  'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb01',
  'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb02',
  'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb03'
);

DELETE FROM ip_addresses
WHERE id IN (
  '55555555-5555-5555-5555-555555555001',
  '55555555-5555-5555-5555-555555555002',
  '55555555-5555-5555-5555-555555555003'
);

DELETE FROM vms
WHERE id IN (
  '99999999-9999-9999-9999-999999999001',
  '99999999-9999-9999-9999-999999999002',
  '99999999-9999-9999-9999-999999999003'
);

DELETE FROM node_storage
WHERE node_id IN (
  '33333333-3333-3333-3333-333333333001',
  '33333333-3333-3333-3333-333333333002',
  '33333333-3333-3333-3333-333333333003',
  '33333333-3333-3333-3333-333333333004',
  '33333333-3333-3333-3333-333333333005'
);

DELETE FROM storage_backends
WHERE id IN (
  '12121212-1212-1212-1212-121212121001',
  '12121212-1212-1212-1212-121212121002'
);

DELETE FROM templates
WHERE id IN (
  '66666666-6666-6666-6666-666666666001',
  '66666666-6666-6666-6666-666666666002',
  '66666666-6666-6666-6666-666666666003',
  '66666666-6666-6666-6666-666666666004',
  '66666666-6666-6666-6666-666666666005'
);

DELETE FROM ip_sets
WHERE id IN (
  '44444444-4444-4444-4444-444444444001',
  '44444444-4444-4444-4444-444444444002'
);

DELETE FROM nodes
WHERE id IN (
  '33333333-3333-3333-3333-333333333001',
  '33333333-3333-3333-3333-333333333002',
  '33333333-3333-3333-3333-333333333003',
  '33333333-3333-3333-3333-333333333004',
  '33333333-3333-3333-3333-333333333005'
);

DELETE FROM plans
WHERE id IN (
  '11111111-1111-1111-1111-111111111001',
  '11111111-1111-1111-1111-111111111002',
  '11111111-1111-1111-1111-111111111003',
  '11111111-1111-1111-1111-111111111004'
);

DELETE FROM customers
WHERE email IN (
  'customer@test.virtuestack.local',
  '2fa-customer@test.virtuestack.local',
  'suspended@test.virtuestack.local'
);

DELETE FROM failed_login_attempts
WHERE email IN (
  'admin:admin@test.virtuestack.local',
  'admin:2fa-admin@test.virtuestack.local',
  'admin:superadmin@test.virtuestack.local',
  'customer:customer@test.virtuestack.local',
  'customer:2fa-customer@test.virtuestack.local',
  'customer:suspended@test.virtuestack.local'
);

DELETE FROM admins
WHERE email IN (
  'admin@test.virtuestack.local',
  '2fa-admin@test.virtuestack.local',
  'superadmin@test.virtuestack.local'
);

DELETE FROM locations
WHERE id IN (
  '22222222-2222-2222-2222-222222222001',
  '22222222-2222-2222-2222-222222222002'
);

INSERT INTO locations (id, name, region, country, created_at)
VALUES
  ('22222222-2222-2222-2222-222222222001', 'Test US East', 'us-east', 'US', NOW()),
  ('22222222-2222-2222-2222-222222222002', 'Test EU West', 'eu-west', 'DE', NOW());

INSERT INTO storage_backends (
  id, name, type, ceph_pool, ceph_user, ceph_monitors, storage_path,
  total_gb, used_gb, available_gb, health_status, health_message,
  created_at, updated_at
)
VALUES
  (
    '12121212-1212-1212-1212-121212121001',
    'Test Ceph Cluster',
    'ceph',
    'vs-vms',
    'virtuestack',
    '10.10.10.10:6789',
    NULL,
    5000,
    500,
    4500,
    'healthy',
    'Ready for E2E workloads',
    NOW(),
    NOW()
  ),
  (
    '12121212-1212-1212-1212-121212121002',
    'Test QCOW Storage',
    'qcow',
    NULL,
    NULL,
    NULL,
    '/var/lib/virtuestack/storage/qcow',
    2000,
    150,
    1850,
    'healthy',
    'Ready for E2E workloads',
    NOW(),
    NOW()
  );

INSERT INTO plans (
  id, name, slug, vcpu, memory_mb, disk_gb, port_speed_mbps, bandwidth_limit_gb,
  is_active, sort_order, created_at, updated_at, price_monthly, price_hourly,
  price_hourly_stopped, storage_backend, snapshot_limit, backup_limit, iso_upload_limit, currency
)
VALUES
  (
    '11111111-1111-1111-1111-111111111001',
    'Test Basic',
    'test-basic',
    1,
    1024,
    25,
    100,
    1000,
    TRUE,
    1,
    NOW(),
    NOW(),
    1000,
    2,
    0,
    'ceph',
    2,
    2,
    2,
    'USD'
  ),
  (
    '11111111-1111-1111-1111-111111111002',
    'Test Standard',
    'test-standard',
    2,
    2048,
    50,
    250,
    2000,
    TRUE,
    2,
    NOW(),
    NOW(),
    2000,
    3,
    0,
    'ceph',
    3,
    3,
    2,
    'USD'
  ),
  (
    '11111111-1111-1111-1111-111111111003',
    'Test Premium',
    'test-premium',
    4,
    4096,
    100,
    500,
    4000,
    TRUE,
    3,
    NOW(),
    NOW(),
    4000,
    6,
    0,
    'ceph',
    4,
    4,
    3,
    'USD'
  ),
  (
    '11111111-1111-1111-1111-111111111004',
    'Test Enterprise',
    'test-enterprise',
    8,
    8192,
    160,
    1000,
    8000,
    TRUE,
    4,
    NOW(),
    NOW(),
    8000,
    10,
    0,
    'qcow',
    6,
    6,
    4,
    'USD'
  );

INSERT INTO templates (
  id, name, os_family, os_version, rbd_image, rbd_snapshot,
  min_disk_gb, supports_cloudinit, is_active, sort_order,
  created_at, updated_at, version, description, storage_backend, file_path
)
VALUES
  (
    '66666666-6666-6666-6666-666666666001',
    'Ubuntu 22.04',
    'Linux',
    '22.04',
    'ubuntu-22.04',
    'v1',
    20,
    TRUE,
    TRUE,
    1,
    NOW(),
    NOW(),
    1,
    'Ubuntu 22.04 LTS image for E2E tests.',
    'ceph',
    '/templates/ubuntu-22.04.qcow2'
  ),
  (
    '66666666-6666-6666-6666-666666666002',
    'Ubuntu 24.04',
    'Linux',
    '24.04',
    'ubuntu-24.04',
    'v1',
    20,
    TRUE,
    TRUE,
    2,
    NOW(),
    NOW(),
    1,
    'Ubuntu 24.04 image for E2E tests.',
    'ceph',
    '/templates/ubuntu-24.04.qcow2'
  ),
  (
    '66666666-6666-6666-6666-666666666003',
    'Debian 12',
    'Linux',
    '12',
    'debian-12',
    'v1',
    20,
    TRUE,
    TRUE,
    3,
    NOW(),
    NOW(),
    1,
    'Debian 12 image for E2E tests.',
    'ceph',
    '/templates/debian-12.qcow2'
  ),
  (
    '66666666-6666-6666-6666-666666666004',
    'Rocky Linux 9',
    'Linux',
    '9',
    'rocky-9',
    'v1',
    20,
    TRUE,
    TRUE,
    4,
    NOW(),
    NOW(),
    1,
    'Rocky Linux 9 image for E2E tests.',
    'ceph',
    '/templates/rocky-9.qcow2'
  ),
  (
    '66666666-6666-6666-6666-666666666005',
    'AlmaLinux 9',
    'Linux',
    '9',
    'almalinux-9',
    'v1',
    20,
    TRUE,
    TRUE,
    5,
    NOW(),
    NOW(),
    1,
    'AlmaLinux 9 image for E2E tests.',
    'qcow',
    '/templates/almalinux-9.qcow2'
  );

INSERT INTO nodes (
  id, hostname, grpc_address, management_ip, location_id, status,
  total_vcpu, total_memory_mb, allocated_vcpu, allocated_memory_mb,
  created_at, storage_backend, storage_path, ceph_pool, ceph_user, ceph_monitors
)
VALUES
  (
    '33333333-3333-3333-3333-333333333001',
    'test-node-1',
    '10.0.0.101:50051',
    '10.0.0.101',
    '22222222-2222-2222-2222-222222222001',
    'online',
    32,
    65536,
    6,
    6144,
    NOW(),
    'ceph',
    '/var/lib/virtuestack/node-1',
    'vs-vms',
    'virtuestack',
    '10.10.10.10:6789'
  ),
  (
    '33333333-3333-3333-3333-333333333002',
    'test-node-2',
    '10.0.0.102:50051',
    '10.0.0.102',
    '22222222-2222-2222-2222-222222222001',
    'online',
    32,
    65536,
    2,
    2048,
    NOW(),
    'ceph',
    '/var/lib/virtuestack/node-2',
    'vs-vms',
    'virtuestack',
    '10.10.10.10:6789'
  ),
  (
    '33333333-3333-3333-3333-333333333003',
    'test-node-3',
    '10.0.0.103:50051',
    '10.0.0.103',
    '22222222-2222-2222-2222-222222222002',
    'online',
    24,
    49152,
    8,
    8192,
    NOW(),
    'qcow',
    '/var/lib/virtuestack/node-3',
    NULL,
    NULL,
    NULL
  ),
  (
    '33333333-3333-3333-3333-333333333004',
    'test-node-4',
    '10.0.0.104:50051',
    '10.0.0.104',
    '22222222-2222-2222-2222-222222222001',
    'offline',
    24,
    49152,
    0,
    0,
    NOW(),
    'ceph',
    '/var/lib/virtuestack/node-4',
    'vs-vms',
    'virtuestack',
    '10.10.10.10:6789'
  ),
  (
    '33333333-3333-3333-3333-333333333005',
    'test-node-5',
    '10.0.0.105:50051',
    '10.0.0.105',
    '22222222-2222-2222-2222-222222222001',
    'draining',
    24,
    49152,
    0,
    0,
    NOW(),
    'ceph',
    '/var/lib/virtuestack/node-5',
    'vs-vms',
    'virtuestack',
    '10.10.10.10:6789'
  );

INSERT INTO node_storage (node_id, storage_backend_id, enabled, preferred, created_at)
VALUES
  ('33333333-3333-3333-3333-333333333001', '12121212-1212-1212-1212-121212121001', TRUE, TRUE, NOW()),
  ('33333333-3333-3333-3333-333333333002', '12121212-1212-1212-1212-121212121001', TRUE, TRUE, NOW()),
  ('33333333-3333-3333-3333-333333333003', '12121212-1212-1212-1212-121212121002', TRUE, TRUE, NOW()),
  ('33333333-3333-3333-3333-333333333004', '12121212-1212-1212-1212-121212121001', TRUE, TRUE, NOW()),
  ('33333333-3333-3333-3333-333333333005', '12121212-1212-1212-1212-121212121001', TRUE, TRUE, NOW());

INSERT INTO admins (
  id, email, password_hash, name, totp_secret_encrypted, totp_enabled,
  role, max_sessions, created_at, updated_at, permissions
)
VALUES
  (
    '77777777-7777-7777-7777-777777777001',
    'admin@test.virtuestack.local',
    '$argon2id$v=19$m=65536,t=3,p=4$S+4lP6LDPZgCKaERGc7yMQ$ZRaIllNMvmxRCle1paxjBWfyMIRkUaXrdgLdW73ThnU',
    'Test Admin',
    '',
    FALSE,
    'super_admin',
    10,
    NOW(),
    NOW(),
    '[]'::jsonb
  ),
  (
    '77777777-7777-7777-7777-777777777002',
    '2fa-admin@test.virtuestack.local',
    '$argon2id$v=19$m=65536,t=3,p=4$S+4lP6LDPZgCKaERGc7yMQ$ZRaIllNMvmxRCle1paxjBWfyMIRkUaXrdgLdW73ThnU',
    'Test Admin 2FA',
    'LSj2tuwJzbzTxqimms8+j+LH95O9qzb1fQaxFHSolD5FiEMFlcbFeKONGH0=',
    TRUE,
    'super_admin',
    10,
    NOW(),
    NOW(),
    '[]'::jsonb
  ),
  (
    '77777777-7777-7777-7777-777777777003',
    'superadmin@test.virtuestack.local',
    '$argon2id$v=19$m=65536,t=3,p=4$S+4lP6LDPZgCKaERGc7yMQ$ZRaIllNMvmxRCle1paxjBWfyMIRkUaXrdgLdW73ThnU',
    'Super Admin',
    '',
    FALSE,
    'super_admin',
    10,
    NOW(),
    NOW(),
    '[]'::jsonb
  );

INSERT INTO customers (
  id, email, password_hash, name, totp_secret_encrypted, totp_enabled,
  status, created_at, updated_at, totp_backup_codes_shown,
  billing_provider, balance, auth_provider
)
VALUES
  (
    '88888888-8888-8888-8888-888888888001',
    'customer@test.virtuestack.local',
    '$argon2id$v=19$m=65536,t=3,p=4$UckSPc1laEy1U6CPKSY6JQ$oKfM23Fl2O8AHupuNCQwiquIRmdfb2GXzVP0w9P4tIE',
    'Test Customer',
    '',
    FALSE,
    'active',
    NOW(),
    NOW(),
    FALSE,
    'native',
    5000,
    'local'
  ),
  (
    '88888888-8888-8888-8888-888888888002',
    '2fa-customer@test.virtuestack.local',
    '$argon2id$v=19$m=65536,t=3,p=4$lP8KMMeCZBzY9SlR7OOKJw$5J9Mnp5bWaCLkayfl72cUBgf5pc7JHYXDDUBFmuRg6w',
    'Test Customer 2FA',
    'C572A12EWu8TUDzhpuFKLnOX6Kg19wJDq6dt11Lxn1jgSFVeUHWilejBFSY=',
    TRUE,
    'active',
    NOW(),
    NOW(),
    FALSE,
    'native',
    2500,
    'local'
  ),
  (
    '88888888-8888-8888-8888-888888888003',
    'suspended@test.virtuestack.local',
    '$argon2id$v=19$m=65536,t=3,p=4$UckSPc1laEy1U6CPKSY6JQ$oKfM23Fl2O8AHupuNCQwiquIRmdfb2GXzVP0w9P4tIE',
    'Suspended Customer',
    '',
    FALSE,
    'suspended',
    NOW(),
    NOW(),
    FALSE,
    'native',
    0,
    'local'
  );

INSERT INTO ip_sets (
  id, name, location_id, network, gateway, vlan_id, ip_version, node_ids, created_at
)
VALUES
  (
    '44444444-4444-4444-4444-444444444001',
    'Test Public IPv4',
    '22222222-2222-2222-2222-222222222001',
    '192.0.2.0/24',
    '192.0.2.1',
    100,
    4,
    ARRAY[
      '33333333-3333-3333-3333-333333333001'::uuid,
      '33333333-3333-3333-3333-333333333002'::uuid,
      '33333333-3333-3333-3333-333333333003'::uuid
    ],
    NOW()
  ),
  (
    '44444444-4444-4444-4444-444444444002',
    'Test Private IPv4',
    '22222222-2222-2222-2222-222222222001',
    '10.10.0.0/24',
    '10.10.0.1',
    200,
    4,
    ARRAY[
      '33333333-3333-3333-3333-333333333001'::uuid,
      '33333333-3333-3333-3333-333333333002'::uuid
    ],
    NOW()
  );

INSERT INTO vms (
  id, customer_id, node_id, plan_id, hostname, status, vcpu, memory_mb, disk_gb,
  port_speed_mbps, bandwidth_limit_gb, mac_address, template_id, created_at, updated_at,
  storage_backend, disk_path, ceph_pool, rbd_image, storage_backend_id, external_service_id
)
VALUES
  (
    '99999999-9999-9999-9999-999999999001',
    '88888888-8888-8888-8888-888888888001',
    '33333333-3333-3333-3333-333333333001',
    '11111111-1111-1111-1111-111111111002',
    'test-vm-running',
    'running',
    2,
    2048,
    50,
    250,
    2000,
    '52:54:00:12:34:01',
    '66666666-6666-6666-6666-666666666001',
    NOW(),
    NOW(),
    'ceph',
    '/var/lib/libvirt/images/test-vm-running.qcow2',
    'vs-vms',
    'vm-test-running',
    '12121212-1212-1212-1212-121212121001',
    1001
  ),
  (
    '99999999-9999-9999-9999-999999999002',
    '88888888-8888-8888-8888-888888888001',
    '33333333-3333-3333-3333-333333333002',
    '11111111-1111-1111-1111-111111111001',
    'test-vm-stopped',
    'stopped',
    1,
    1024,
    25,
    100,
    1000,
    '52:54:00:12:34:02',
    '66666666-6666-6666-6666-666666666003',
    NOW(),
    NOW(),
    'ceph',
    '/var/lib/libvirt/images/test-vm-stopped.qcow2',
    'vs-vms',
    'vm-test-stopped',
    '12121212-1212-1212-1212-121212121001',
    1002
  ),
  (
    '99999999-9999-9999-9999-999999999003',
    '88888888-8888-8888-8888-888888888002',
    '33333333-3333-3333-3333-333333333003',
    '11111111-1111-1111-1111-111111111004',
    'test-vm-suspended',
    'suspended',
    8,
    8192,
    160,
    1000,
    8000,
    '52:54:00:12:34:03',
    '66666666-6666-6666-6666-666666666005',
    NOW(),
    NOW(),
    'qcow',
    '/var/lib/libvirt/images/test-vm-suspended.qcow2',
    NULL,
    NULL,
    '12121212-1212-1212-1212-121212121002',
    1003
  );

INSERT INTO ip_addresses (
  id, ip_set_id, address, ip_version, vm_id, customer_id,
  is_primary, rdns_hostname, status, assigned_at, created_at
)
VALUES
  (
    '55555555-5555-5555-5555-555555555001',
    '44444444-4444-4444-4444-444444444001',
    '192.0.2.10'::inet,
    4,
    '99999999-9999-9999-9999-999999999001',
    '88888888-8888-8888-8888-888888888001',
    TRUE,
    'test-vm-running.example.test',
    'assigned',
    NOW(),
    NOW()
  ),
  (
    '55555555-5555-5555-5555-555555555002',
    '44444444-4444-4444-4444-444444444001',
    '192.0.2.11'::inet,
    4,
    '99999999-9999-9999-9999-999999999002',
    '88888888-8888-8888-8888-888888888001',
    TRUE,
    'test-vm-stopped.example.test',
    'assigned',
    NOW(),
    NOW()
  ),
  (
    '55555555-5555-5555-5555-555555555003',
    '44444444-4444-4444-4444-444444444001',
    '192.0.2.12'::inet,
    4,
    '99999999-9999-9999-9999-999999999003',
    '88888888-8888-8888-8888-888888888002',
    TRUE,
    'test-vm-suspended.example.test',
    'assigned',
    NOW(),
    NOW()
  );

INSERT INTO backups (
  id, vm_id, rbd_snapshot, storage_path, size_bytes, status, created_at,
  storage_backend, file_path, source, snapshot_name, method, name
)
VALUES
  (
    'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa01',
    '99999999-9999-9999-9999-999999999001',
    'backup-full-001',
    '/backups/test-vm-running-full-001',
    2147483648,
    'completed',
    NOW() - INTERVAL '2 days',
    'ceph',
    '/backups/test-vm-running-full-001.tar.zst',
    'manual',
    NULL,
    'full',
    'Daily Full Backup'
  ),
  (
    'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa02',
    '99999999-9999-9999-9999-999999999001',
    'snapshot-pre-upgrade',
    '/snapshots/test-vm-running-pre-upgrade',
    536870912,
    'completed',
    NOW() - INTERVAL '1 day',
    'ceph',
    '/snapshots/test-vm-running-pre-upgrade',
    'manual',
    'pre-upgrade',
    'snapshot',
    'Pre-upgrade Snapshot'
  ),
  (
    'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaa03',
    '99999999-9999-9999-9999-999999999002',
    'backup-full-002',
    '/backups/test-vm-stopped-full-002',
    1073741824,
    'failed',
    NOW() - INTERVAL '12 hours',
    'ceph',
    '/backups/test-vm-stopped-full-002.tar.zst',
    'manual',
    NULL,
    'full',
    'Failed Full Backup'
  );

INSERT INTO snapshots (id, vm_id, name, rbd_snapshot, size_bytes, created_at)
VALUES
  (
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb01',
    '99999999-9999-9999-9999-999999999001',
    'pre-upgrade',
    'snapshot-pre-upgrade',
    536870912,
    NOW() - INTERVAL '1 day'
  ),
  (
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb02',
    '99999999-9999-9999-9999-999999999002',
    'before-maintenance',
    'snapshot-before-maintenance',
    268435456,
    NOW() - INTERVAL '3 days'
  ),
  (
    'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbb03',
    '99999999-9999-9999-9999-999999999003',
    'pre-suspend',
    'snapshot-pre-suspend',
    134217728,
    NOW() - INTERVAL '5 days'
  );

COMMIT;

SELECT 'E2E Test Seed Data Loaded' AS status;
SELECT
  (SELECT COUNT(*) FROM plans WHERE slug LIKE 'test-%') AS test_plans,
  (SELECT COUNT(*) FROM nodes WHERE hostname LIKE 'test-node-%') AS test_nodes,
  (SELECT COUNT(*) FROM vms WHERE hostname LIKE 'test-vm-%') AS test_vms,
  (SELECT COUNT(*) FROM customers WHERE email LIKE '%@test.virtuestack.local') AS test_customers,
  (SELECT COUNT(*) FROM admins WHERE email LIKE '%@test.virtuestack.local') AS test_admins;
