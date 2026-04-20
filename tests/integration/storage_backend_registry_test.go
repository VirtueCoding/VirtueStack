// Package integration provides end-to-end integration tests for VirtueStack.
// These tests use testcontainers to spin up PostgreSQL and NATS containers.
package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStorageBackendRegistryMigration tests the Phase 2 data migration
// (000055_storage_backend_registry.up.sql) which creates the storage_backends
// table, node_storage junction table, and migrates existing node config.
func TestStorageBackendRegistryMigration(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	ctx := context.Background()

	// =====================================================================
	// SETUP: Create test nodes with storage backend configurations
	// =====================================================================

	// Create a Ceph node
	cephNodeID := "10000000-0000-0000-0000-000000000001"
	_, err := suite.DBPool.Exec(ctx, `
		INSERT INTO nodes (id, hostname, grpc_address, management_ip, status, storage_backend, ceph_pool, ceph_user, ceph_monitors, total_vcpu, total_memory_mb, allocated_vcpu, allocated_memory_mb)
		VALUES ($1, 'ceph-node-1', '10.0.0.10:50051', '10.0.0.10', 'online', 'ceph', 'vms', 'admin', '10.0.0.10:6789', 8, 16384, 0, 0)
	`, cephNodeID)
	require.NoError(t, err, "Failed to create Ceph test node")

	// Create a QCOW node
	qcowNodeID := "10000000-0000-0000-0000-000000000002"
	_, err = suite.DBPool.Exec(ctx, `
		INSERT INTO nodes (id, hostname, grpc_address, management_ip, status, storage_backend, storage_path, total_vcpu, total_memory_mb, allocated_vcpu, allocated_memory_mb)
		VALUES ($1, 'qcow-node-1', '10.0.0.11:50051', '10.0.0.11', 'online', 'qcow', '/var/lib/virtuestack/vms', 8, 16384, 0, 0)
	`, qcowNodeID)
	require.NoError(t, err, "Failed to create QCOW test node")

	// Create an LVM node
	lvmNodeID := "10000000-0000-0000-0000-000000000003"
	_, err = suite.DBPool.Exec(ctx, `
		INSERT INTO nodes (id, hostname, grpc_address, management_ip, status, storage_backend, lvm_volume_group, lvm_thin_pool, total_vcpu, total_memory_mb, allocated_vcpu, allocated_memory_mb)
		VALUES ($1, 'lvm-node-1', '10.0.0.12:50051', '10.0.0.12', 'online', 'lvm', 'vgvs', 'thinpool', 8, 16384, 0, 0)
	`, lvmNodeID)
	require.NoError(t, err, "Failed to create LVM test node")

	// =====================================================================
	// Create test VMs associated with nodes
	// =====================================================================

	vm1ID := "20000000-0000-0000-0000-000000000001"
	_, err = suite.DBPool.Exec(ctx, `
		INSERT INTO vms (id, customer_id, node_id, plan_id, hostname, status, storage_backend)
		VALUES ($1, $2, $3, $4, 'test-vm-1', 'running', 'ceph')
	`, vm1ID, TestCustomerID, cephNodeID, TestPlanID)
	require.NoError(t, err, "Failed to create test VM 1")

	vm2ID := "20000000-0000-0000-0000-000000000002"
	_, err = suite.DBPool.Exec(ctx, `
		INSERT INTO vms (id, customer_id, node_id, plan_id, hostname, status, storage_backend)
		VALUES ($1, $2, $3, $4, 'test-vm-2', 'running', 'qcow')
	`, vm2ID, TestCustomerID, qcowNodeID, TestPlanID)
	require.NoError(t, err, "Failed to create test VM 2")

	// =====================================================================
	// EXECUTE: Run the storage backend migration
	// Note: Migration 000055 should have already run as part of TestMain
	// This test verifies the state after migration
	// =====================================================================

	// Verify storage_backends table exists and has correct entries
	var backendCount int
	err = suite.DBPool.QueryRow(ctx, "SELECT COUNT(*) FROM storage_backends").Scan(&backendCount)
	require.NoError(t, err, "storage_backends table should exist")
	require.GreaterOrEqual(t, backendCount, 3, "Should have at least 3 storage backends (ceph, qcow, lvm)")

	// Verify Ceph storage backend
	var cephPool, cephUser string
	err = suite.DBPool.QueryRow(ctx, `
		SELECT ceph_pool, ceph_user FROM storage_backends WHERE type = 'ceph' LIMIT 1
	`).Scan(&cephPool, &cephUser)
	require.NoError(t, err, "Should have a Ceph storage backend")
	require.Equal(t, "vms", cephPool)
	require.Equal(t, "admin", cephUser)

	// Verify QCOW storage backend
	var qcowPath string
	err = suite.DBPool.QueryRow(ctx, `
		SELECT storage_path FROM storage_backends WHERE type = 'qcow' LIMIT 1
	`).Scan(&qcowPath)
	require.NoError(t, err, "Should have a QCOW storage backend")
	require.Equal(t, "/var/lib/virtuestack/vms", qcowPath)

	// Verify LVM storage backend
	var lvmVG, lvmPool string
	err = suite.DBPool.QueryRow(ctx, `
		SELECT lvm_volume_group, lvm_thin_pool FROM storage_backends WHERE type = 'lvm' LIMIT 1
	`).Scan(&lvmVG, &lvmPool)
	require.NoError(t, err, "Should have an LVM storage backend")
	require.Equal(t, "vgvs", lvmVG)
	require.Equal(t, "thinpool", lvmPool)

	// =====================================================================
	// Verify node_storage junction table
	// =====================================================================

	var junctionCount int
	err = suite.DBPool.QueryRow(ctx, "SELECT COUNT(*) FROM node_storage").Scan(&junctionCount)
	require.NoError(t, err, "node_storage table should exist")
	require.GreaterOrEqual(t, junctionCount, 3, "Should have at least 3 junction records")

	// Verify junction records for each node
	var cephJunctionCount int
	err = suite.DBPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM node_storage ns
		JOIN storage_backends sb ON sb.id = ns.storage_backend_id
		WHERE sb.type = 'ceph'
	`).Scan(&cephJunctionCount)
	require.NoError(t, err)
	require.Equal(t, 1, cephJunctionCount, "Ceph node should have 1 junction record")

	// Verify VMs have storage_backend_id populated
	var vm1BackendID, vm2BackendID *string
	err = suite.DBPool.QueryRow(ctx, "SELECT storage_backend_id FROM vms WHERE id = $1", vm1ID).Scan(&vm1BackendID)
	require.NoError(t, err, "VM1 should have storage_backend_id")
	require.NotNil(t, vm1BackendID, "VM1 storage_backend_id should not be NULL")

	err = suite.DBPool.QueryRow(ctx, "SELECT storage_backend_id FROM vms WHERE id = $1", vm2ID).Scan(&vm2BackendID)
	require.NoError(t, err, "VM2 should have storage_backend_id")
	require.NotNil(t, vm2BackendID, "VM2 storage_backend_id should not be NULL")

	// =====================================================================
	// Verify storage_backend_id is NOT NULL (after 000057 migration)
	// =====================================================================

	var nullCount int
	err = suite.DBPool.QueryRow(ctx, "SELECT COUNT(*) FROM vms WHERE storage_backend_id IS NULL").Scan(&nullCount)
	require.NoError(t, err)
	require.Equal(t, 0, nullCount, "All VMs should have non-NULL storage_backend_id after migration")

	// =====================================================================
	// CLEANUP
	// =====================================================================

	_, err = suite.DBPool.Exec(ctx, "DELETE FROM vms WHERE id IN ($1, $2)", vm1ID, vm2ID)
	require.NoError(t, err, "Failed to cleanup test VMs")

	_, err = suite.DBPool.Exec(ctx, "DELETE FROM node_storage WHERE node_id IN ($1, $2, $3)", cephNodeID, qcowNodeID, lvmNodeID)
	require.NoError(t, err, "Failed to cleanup junction records")

	_, err = suite.DBPool.Exec(ctx, "DELETE FROM storage_backends WHERE type IN ('ceph', 'qcow', 'lvm')")
	require.NoError(t, err, "Failed to cleanup storage backends")

	_, err = suite.DBPool.Exec(ctx, "DELETE FROM nodes WHERE id IN ($1, $2, $3)", cephNodeID, qcowNodeID, lvmNodeID)
	require.NoError(t, err, "Failed to cleanup test nodes")
}

// TestStorageBackendRegistryDownMigration tests the rollback of the
// 000055_storage_backend_registry migration.
func TestStorageBackendRegistryDownMigration(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	// This test verifies that the down migration can be executed
	// In production, the down migration would be run to rollback the changes

	// Note: Running the actual down migration requires careful sequencing
	// and would affect other tests. This test documents the expected behavior.

	t.Skip("Down migration test requires isolated environment - manual verification recommended")

	// To test down migration manually:
	// 1. Run: migrate -database $DATABASE_URL -path ./migrations/down 55
	// 2. Verify: storage_backends table is dropped
	// 3. Verify: node_storage table is dropped
	// 4. Verify: vms.storage_backend_id column is dropped
	// 5. Run up migration to restore: migrate -database $DATABASE_URL -path ./migrations/up
}

// TestStorageBackendNotNullConstraint tests that the NOT NULL constraint
// is properly enforced on vms.storage_backend_id.
func TestStorageBackendNotNullConstraint(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	ctx := context.Background()

	// Create a VM without a node (and thus without storage_backend_id)
	vmID := "20000000-0000-0000-0000-000000000003"
	_, err := suite.DBPool.Exec(ctx, `
		INSERT INTO vms (id, customer_id, plan_id, hostname, status, storage_backend)
		VALUES ($1, $2, $3, 'test-vm-no-node', 'provisioning', 'ceph')
	`, vmID, TestCustomerID, TestPlanID)
	// This should succeed because storage_backend_id can be NULL before NOT NULL enforcement
	require.NoError(t, err, "VM creation without node should succeed before NOT NULL enforcement")

	// After migration 000057, this VM should have its storage_backend_id populated
	// via the backfill logic in the migration

	// Verify the VM has a storage_backend_id after migration
	var backendID *string
	err = suite.DBPool.QueryRow(ctx, "SELECT storage_backend_id FROM vms WHERE id = $1", vmID).Scan(&backendID)
	require.NoError(t, err)
	require.NotNil(t, backendID, "VM should have storage_backend_id after NOT NULL enforcement migration")

	// Cleanup
	_, _ = suite.DBPool.Exec(ctx, "DELETE FROM vms WHERE id = $1", vmID)
}
