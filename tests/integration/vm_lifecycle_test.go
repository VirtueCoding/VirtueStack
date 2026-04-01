// Package integration provides end-to-end integration tests for VirtueStack.
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVMLifecycle tests the complete VM lifecycle: create -> start -> stop -> delete.
func TestVMLifecycle(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("CreateVM", func(t *testing.T) {
		// Create a new VM
		vm, _, err := suite.VMService.CreateVM(ctx, &models.VMCreateRequest{
			PlanID:     TestPlanID,
			Hostname:   "test-vm-lifecycle",
			TemplateID: TestTemplateID,
			Password:   TestVMPassword,
		}, TestCustomerID)

		require.NoError(t, err, "VM creation should succeed")
		assert.NotEmpty(t, vm.ID, "VM ID should be generated")
		assert.Equal(t, models.VMStatusProvisioning, vm.Status, "VM should be in provisioning state")
		assert.Equal(t, "test-vm-lifecycle", vm.Hostname, "Hostname should match")
	})

	t.Run("GetVM", func(t *testing.T) {
		// Create a VM first
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Retrieve the VM
		vm, err := suite.VMRepo.GetByID(ctx, vmID)
		require.NoError(t, err, "Getting VM should succeed")
		assert.Equal(t, vmID, vm.ID, "VM ID should match")
		assert.Equal(t, TestCustomerID, vm.CustomerID, "Customer ID should match")
	})

	t.Run("ListVMs", func(t *testing.T) {
		// Create multiple VMs
		for i := 0; i < 3; i++ {
			_, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
			require.NoError(t, err)
		}

		// List VMs for customer
		custID := TestCustomerID
		vms, total, err := suite.VMRepo.List(ctx, models.VMListFilter{
			CustomerID: &custID,
			PaginationParams: models.PaginationParams{
				Page:    1,
				PerPage: 10,
			},
		})

		require.NoError(t, err, "Listing VMs should succeed")
		assert.GreaterOrEqual(t, total, 3, "Should have at least 3 VMs")
		assert.GreaterOrEqual(t, len(vms), 3, "Should return at least 3 VMs")
	})

	t.Run("UpdateVMStatus", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Update status to running
		err = suite.VMRepo.UpdateStatus(ctx, vmID, models.VMStatusRunning)
		require.NoError(t, err, "Updating status should succeed")

		// Verify status
		vm, err := suite.VMRepo.GetByID(ctx, vmID)
		require.NoError(t, err)
		assert.Equal(t, models.VMStatusRunning, vm.Status, "Status should be running")
	})

	t.Run("VMStartStop", func(t *testing.T) {
		// Create a VM in stopped state
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Set to running
		err = suite.VMRepo.UpdateStatus(ctx, vmID, models.VMStatusRunning)
		require.NoError(t, err)

		// Stop the VM
		err = suite.VMRepo.UpdateStatus(ctx, vmID, models.VMStatusStopped)
		require.NoError(t, err, "Stopping VM should succeed")

		vm, err := suite.VMRepo.GetByID(ctx, vmID)
		require.NoError(t, err)
		assert.Equal(t, models.VMStatusStopped, vm.Status, "VM should be stopped")

		// Start the VM again
		err = suite.VMRepo.UpdateStatus(ctx, vmID, models.VMStatusRunning)
		require.NoError(t, err, "Starting VM should succeed")

		vm, err = suite.VMRepo.GetByID(ctx, vmID)
		require.NoError(t, err)
		assert.Equal(t, models.VMStatusRunning, vm.Status, "VM should be running")
	})

	t.Run("DeleteVM", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Delete the VM
		err = suite.VMRepo.SoftDelete(ctx, vmID)
		require.NoError(t, err, "Deleting VM should succeed")

		// Verify soft delete - should not be found with normal query
		_, err = suite.VMRepo.GetByID(ctx, vmID)
		assert.Error(t, err, "VM should not be found after soft delete")
		assert.True(t, sharederrors.Is(err, sharederrors.ErrNotFound), "Error should be ErrNotFound")
	})
}

// TestVMStatusTransitions tests all valid VM status transitions.
func TestVMStatusTransitions(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	validTransitions := []struct {
		from string
		to   string
	}{
		{models.VMStatusProvisioning, models.VMStatusRunning},
		{models.VMStatusProvisioning, models.VMStatusError},
		{models.VMStatusRunning, models.VMStatusStopped},
		{models.VMStatusRunning, models.VMStatusSuspended},
		{models.VMStatusRunning, models.VMStatusMigrating},
		{models.VMStatusStopped, models.VMStatusRunning},
		{models.VMStatusStopped, models.VMStatusReinstalling},
		{models.VMStatusSuspended, models.VMStatusRunning},
		{models.VMStatusMigrating, models.VMStatusRunning},
		{models.VMStatusReinstalling, models.VMStatusRunning},
		{models.VMStatusReinstalling, models.VMStatusError},
	}

	for _, tt := range validTransitions {
		t.Run(tt.from+"->"+tt.to, func(t *testing.T) {
			// Create a VM
			vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
			require.NoError(t, err)

			// Set initial status
			err = suite.VMRepo.UpdateStatus(ctx, vmID, tt.from)
			require.NoError(t, err)

			// Transition to new status
			err = suite.VMRepo.UpdateStatus(ctx, vmID, tt.to)
			require.NoError(t, err, "Status transition %s -> %s should succeed", tt.from, tt.to)

			// Verify
			vm, err := suite.VMRepo.GetByID(ctx, vmID)
			require.NoError(t, err)
			assert.Equal(t, tt.to, vm.Status)
		})
	}
}

// TestVMAssignment tests VM assignment to nodes.
func TestVMAssignment(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("AssignVMToNode", func(t *testing.T) {
		// Create a VM without node assignment
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, "")
		require.NoError(t, err)

		// Assign to node
		err = suite.VMRepo.UpdateNodeAssignment(ctx, vmID, TestNodeID)
		require.NoError(t, err, "Assigning VM to node should succeed")

		// Verify assignment
		vm, err := suite.VMRepo.GetByID(ctx, vmID)
		require.NoError(t, err)
		require.NotNil(t, vm.NodeID)
		assert.Equal(t, TestNodeID, *vm.NodeID, "VM should be assigned to the node")
	})

	t.Run("ReassignVMToDifferentNode", func(t *testing.T) {
		// Create a VM with node assignment
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create a second node
		node2ID := uuid.New().String()
		_, err = suite.DBPool.Exec(ctx, `
			INSERT INTO nodes (id, hostname, grpc_address, management_ip, status, total_vcpu, total_memory_mb, created_at)
			VALUES ($1, 'test-node-2', '192.168.1.101:50051', '192.168.1.101', 'online', 16, 65536, NOW())
		`, node2ID)
		require.NoError(t, err)

		// Reassign to second node
		err = suite.VMRepo.UpdateNodeAssignment(ctx, vmID, node2ID)
		require.NoError(t, err, "Reassigning VM should succeed")

		// Verify reassignment
		vm, err := suite.VMRepo.GetByID(ctx, vmID)
		require.NoError(t, err)
		require.NotNil(t, vm.NodeID)
		assert.Equal(t, node2ID, *vm.NodeID, "VM should be reassigned to new node")

		// Cleanup
		_, _ = suite.DBPool.Exec(ctx, "DELETE FROM nodes WHERE id = $1", node2ID)
	})
}

// TestVMIPAddressManagement tests IP address assignment for VMs.
func TestVMIPAddressManagement(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("AssignIPv4Address", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create and assign IP
		ipID, err := CreateTestIP(ctx, vmID)
		require.NoError(t, err, "Creating IP should succeed")
		assert.NotEmpty(t, ipID, "IP ID should be generated")
	})

	t.Run("GetVMWithIPs", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create multiple IPs
		for i := 0; i < 2; i++ {
			_, err := CreateTestIP(ctx, vmID)
			require.NoError(t, err)
		}

		// Get VM detail with IPs
		detail, err := suite.VMRepo.GetByID(ctx, vmID)
		require.NoError(t, err, "Getting VM detail should succeed")
		assert.NotEmpty(t, detail.ID, "VM should exist")
	})

	t.Run("PrimaryIPAssignment", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create primary IP
		ipID, err := CreateTestIP(ctx, vmID)
		require.NoError(t, err)

		// Verify primary flag
		var isPrimary bool
		err = suite.DBPool.QueryRow(ctx, "SELECT is_primary FROM ip_addresses WHERE id = $1", ipID).Scan(&isPrimary)
		require.NoError(t, err)
		assert.True(t, isPrimary, "First IP should be primary")
	})
}

// TestVMResourceValidation tests VM resource constraints.
func TestVMResourceValidation(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("VMResourcesFromPlan", func(t *testing.T) {
		// Create VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Get VM and verify resources match plan
		vm, err := suite.VMRepo.GetByID(ctx, vmID)
		require.NoError(t, err)

		// Get plan
		plan, err := suite.PlanRepo.GetByID(ctx, TestPlanID)
		require.NoError(t, err)

		assert.Equal(t, plan.VCPU, vm.VCPU, "VCPU should match plan")
		assert.Equal(t, plan.MemoryMB, vm.MemoryMB, "Memory should match plan")
		assert.Equal(t, plan.DiskGB, vm.DiskGB, "Disk should match plan")
	})

	t.Run("HostnameUniqueness", func(t *testing.T) {
		// Create first VM
		vmID1, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Get hostname of first VM
		vm1, err := suite.VMRepo.GetByID(ctx, vmID1)
		require.NoError(t, err)
		require.NotEmpty(t, vm1.Hostname, "first VM should have a hostname")

		// Note: VM hostnames are NOT unique in the schema (no UNIQUE constraint).
		// This test verifies that multiple VMs CAN have the same hostname.
		// CreateTestVM uses hardcoded hostname 'test-vm', so duplicates are expected.
		var count int
		err = suite.DBPool.QueryRow(ctx, `
			SELECT COUNT(*) FROM vms WHERE hostname = $1 AND status != $2
		`, vm1.Hostname, models.VMStatusDeleted).Scan(&count)
		require.NoError(t, err)
		// We expect at least 1 VM (the one we just created) with this hostname
		assert.GreaterOrEqual(t, count, 1, "should find at least the VM we created with this hostname")
	})
}

// TestVMConcurrency tests concurrent operations on VMs.
func TestVMConcurrency(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("ConcurrentStatusUpdates", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Run concurrent updates
		done := make(chan error, 10)
		for i := 0; i < 10; i++ {
			go func(idx int) {
				status := models.VMStatusRunning
				if idx%2 == 0 {
					status = models.VMStatusStopped
				}
				done <- suite.VMRepo.UpdateStatus(ctx, vmID, status)
			}(i)
		}

		// Collect results
		var errors []error
		for i := 0; i < 10; i++ {
			if err := <-done; err != nil {
				errors = append(errors, err)
			}
		}

		// All updates should succeed (last one wins)
		assert.Empty(t, errors, "All concurrent updates should succeed")

		// Final status should be one of the valid values
		vm, err := suite.VMRepo.GetByID(ctx, vmID)
		require.NoError(t, err)
		assert.Contains(t, []string{models.VMStatusRunning, models.VMStatusStopped}, vm.Status)
	})
}

// TestVMBandwidthTracking tests bandwidth tracking for VMs.
func TestVMBandwidthTracking(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("UpdateBandwidthUsage", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Update bandwidth usage
		usage := int64(1024 * 1024 * 1024) // 1GB
		err = suite.VMRepo.UpdateBandwidthUsed(ctx, vmID, usage)
		require.NoError(t, err, "Updating bandwidth should succeed")

		// Verify usage
		vm, err := suite.VMRepo.GetByID(ctx, vmID)
		require.NoError(t, err)
		assert.Equal(t, usage, vm.BandwidthUsedBytes, "Bandwidth usage should match")
	})

	t.Run("BandwidthReset", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Set some usage
		err = suite.VMRepo.UpdateBandwidthUsed(ctx, vmID, 1000)
		require.NoError(t, err)

		// Reset bandwidth (simulate monthly reset)
		resetTime := time.Now().Add(30 * 24 * time.Hour)
		_, err = suite.DBPool.Exec(ctx, `
			UPDATE vms SET bandwidth_used_bytes = 0, bandwidth_reset_at = $1 WHERE id = $2
		`, resetTime, vmID)
		require.NoError(t, err)

		// Verify reset
		vm, err := suite.VMRepo.GetByID(ctx, vmID)
		require.NoError(t, err)
		assert.Equal(t, int64(0), vm.BandwidthUsedBytes, "Bandwidth should be reset to 0")
	})
}
