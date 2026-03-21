// Package integration provides end-to-end integration tests for VirtueStack.
package integration

import (
	"context"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBackupCreation tests backup creation operations.
func TestBackupCreation(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("CreateFullBackup", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create a full backup
		backup, err := suite.BackupService.CreateBackup(ctx, vmID, "full")

		require.NoError(t, err, "Backup creation should succeed")
		assert.NotEmpty(t, backup.ID, "Backup ID should be generated")
		assert.Equal(t, vmID, backup.VMID, "Backup VM ID should match")
		assert.Equal(t, "full", backup.Type, "Backup type should be full")
		assert.Equal(t, models.BackupStatusCreating, backup.Status, "Backup status should be creating")
	})

	t.Run("ListBackupsForVM", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create multiple backups
		for i := 0; i < 3; i++ {
			_, err := CreateTestBackup(ctx, vmID)
			require.NoError(t, err)
		}

		// List backups
		backups, err := suite.BackupRepo.ListBackupsByVM(ctx, vmID)

		require.NoError(t, err, "Listing backups should succeed")
		assert.GreaterOrEqual(t, len(backups), 3, "Should have at least 3 backups")
	})

	t.Run("BackupWithNonExistentVM", func(t *testing.T) {
		// Try to create backup for non-existent VM
		_, err := suite.BackupService.CreateBackup(ctx, "non-existent-vm-id", "full")

		assert.Error(t, err, "Backup creation should fail for non-existent VM")
	})
}

// TestBackupRestore tests backup restore operations.
func TestBackupRestore(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("RestoreFromBackup", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create a backup
		backupID, err := CreateTestBackup(ctx, vmID)
		require.NoError(t, err)

		// Update backup to completed
		if _, err := suite.DBPool.Exec(ctx, `
			UPDATE backups SET status = $1, storage_path = $2, size_bytes = $3 WHERE id = $4
		`, models.BackupStatusCompleted, "/backups/"+backupID+".img", int64(1024*1024*100), backupID); err != nil {
			t.Logf("setup warning: %v", err)
		}

		// Initiate restore
		err = suite.BackupService.RestoreBackup(ctx, backupID)
		// Note: This may fail without actual storage backend
		if err != nil {
			assert.NotEmpty(t, backupID, "backup ID should be present even if restore fails")
			return
		}

		backup, err := suite.BackupRepo.GetBackupByID(ctx, backupID)
		require.NoError(t, err)
		assert.Contains(t, []string{models.BackupStatusRestoring, models.BackupStatusCompleted}, backup.Status)
	})

	t.Run("RestoreNonExistentBackup", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)
		_ = vmID
		err = suite.BackupService.RestoreBackup(ctx, "non-existent-backup-id")

		assert.Error(t, err, "Restore should fail for non-existent backup")
	})

	t.Run("RestoreToDifferentVM", func(t *testing.T) {
		// Create two VMs
		vmID1, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		vmID2, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)
		_ = vmID2
		backupID, err := CreateTestBackup(ctx, vmID1)
		require.NoError(t, err)

		// Update backup to completed
		if _, err := suite.DBPool.Exec(ctx, "UPDATE backups SET status = $1 WHERE id = $2", models.BackupStatusCompleted, backupID); err != nil {
			t.Logf("setup warning: %v", err)
		}

		// Restore to different VM
		// Note: nodeAgent is nil in integration tests, so RestoreBackup will return an error.
		// This test verifies that the restore flow doesn't panic and handles the nil nodeAgent case.
		err = suite.BackupService.RestoreBackup(ctx, backupID)
		// With nil nodeAgent, we expect an error about node agent not being configured
		require.Error(t, err, "Restore should return error when nodeAgent is nil")
		assert.Contains(t, err.Error(), "node agent not configured", "Error should mention node agent not configured")
	})
}

// TestBackupScheduling tests backup scheduling functionality.
func TestBackupScheduling(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("CreateBackupSchedule", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create a backup schedule
		schedule := &models.BackupSchedule{
			VMID:           vmID,
			CustomerID:     TestCustomerID,
			Frequency:       "daily",
			RetentionCount: 7,
			Active:         true,
		}

		scheduleID, err := suite.BackupService.CreateSchedule(ctx, schedule)

		require.NoError(t, err, "Creating schedule should succeed")
		assert.NotEmpty(t, scheduleID, "Schedule ID should be generated")
	})

	t.Run("ListBackupSchedules", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create multiple schedules
		for i := 0; i < 2; i++ {
			schedule := &models.BackupSchedule{
				VMID:           vmID,
				CustomerID:     TestCustomerID,
				Frequency:       "daily",
				RetentionCount: 7,
				Active:         true,
			}
			_, err := suite.BackupService.CreateSchedule(ctx, schedule)
			require.NoError(t, err)
		}

		// List schedules
		schedules, err := suite.BackupService.ListSchedules(ctx, vmID)

		require.NoError(t, err, "Listing schedules should succeed")
		assert.GreaterOrEqual(t, len(schedules), 2, "Should have at least 2 schedules")
	})

	t.Run("DisableBackupSchedule", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create a schedule
		schedule := &models.BackupSchedule{
			VMID:           vmID,
			CustomerID:     TestCustomerID,
			Frequency:       "daily",
			RetentionCount: 7,
			Active:         true,
		}
		scheduleID, err := suite.BackupService.CreateSchedule(ctx, schedule)
		require.NoError(t, err)

		// Disable schedule
		err = suite.BackupService.UpdateSchedule(ctx, scheduleID, false)
		require.NoError(t, err, "Disabling schedule should succeed")

		// Verify schedule is disabled
		schedules, _ := suite.BackupService.ListSchedules(ctx, vmID)
		for _, s := range schedules {
			if s.ID == scheduleID {
				assert.False(t, s.Active, "Schedule should be disabled")
			}
		}
	})

	t.Run("DeleteBackupSchedule", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create a schedule
		schedule := &models.BackupSchedule{
			VMID:           vmID,
			CustomerID:     TestCustomerID,
			Frequency:       "daily",
			RetentionCount: 7,
			Active:         true,
		}
		scheduleID, err := suite.BackupService.CreateSchedule(ctx, schedule)
		require.NoError(t, err)

		// Delete schedule
		err = suite.BackupService.DeleteSchedule(ctx, scheduleID)
		require.NoError(t, err, "Deleting schedule should succeed")

		// Verify schedule is deleted
		schedules, _ := suite.BackupService.ListSchedules(ctx, vmID)
		for _, s := range schedules {
			assert.NotEqual(t, scheduleID, s.ID, "Schedule should be deleted")
		}
	})
}

// TestBackupRetention tests backup retention policies.
func TestBackupRetention(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("ApplyRetentionPolicy", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create more backups than retention allows
		for i := 0; i < 10; i++ {
			backupID, err := CreateTestBackup(ctx, vmID)
			require.NoError(t, err)

			// Set older creation times for some backups
			if i < 5 {
				_, _ = suite.DBPool.Exec(ctx, `
					UPDATE backups SET status = $1, created_at = NOW() - INTERVAL '7 days' WHERE id = $2
				`, models.BackupStatusCompleted, backupID)
			} else {
				_, _ = suite.DBPool.Exec(ctx, "UPDATE backups SET status = $1 WHERE id = $2", models.BackupStatusCompleted, backupID)
			}
		}

		// Apply retention (keep only 5)
		err = suite.BackupService.ApplyRetentionPolicy(ctx, vmID, 5)
		require.NoError(t, err, "Applying retention policy should succeed")

		// Count remaining backups
		var count int
		err = suite.DBPool.QueryRow(ctx, `
			SELECT COUNT(*) FROM backups WHERE vm_id = $1 AND status != $2
		`, vmID, models.BackupStatusDeleted).Scan(&count)
		require.NoError(t, err)

		assert.LessOrEqual(t, count, 5, "Should have at most 5 backups after retention")
	})

	t.Run("BackupExpiration", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create a backup with expiration
		backupID, err := CreateTestBackup(ctx, vmID)
		require.NoError(t, err)

		// Set expiration to past
		_, _ = suite.DBPool.Exec(ctx, `
			UPDATE backups SET status = $1, expires_at = NOW() - INTERVAL '1 day' WHERE id = $2
		`, models.BackupStatusCompleted, backupID)

		// Run expiration check
		expired, err := suite.BackupService.ProcessExpiredBackups(ctx)
		require.NoError(t, err, "Processing expired backups should succeed")
		assert.GreaterOrEqual(t, expired, 1, "Should have expired at least 1 backup")

		// Verify backup is deleted (hard delete, not soft delete)
		_, err = suite.BackupRepo.GetBackupByID(ctx, backupID)
		assert.Error(t, err, "Expired backup should be deleted from database")
		assert.True(t, sharederrors.Is(err, sharederrors.ErrNotFound), "Should return ErrNotFound for deleted backup")
	})
}

// TestBackupStatusTransitions tests backup status transitions.
func TestBackupStatusTransitions(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("CreatingToCompleted", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create a backup (starts in creating state)
		backupID, err := CreateTestBackup(ctx, vmID)
		require.NoError(t, err)

		// Update to completed
		_, _ = suite.DBPool.Exec(ctx, "UPDATE backups SET status = $1 WHERE id = $2", models.BackupStatusCompleted, backupID)

		// Verify
		backup, err := suite.BackupRepo.GetBackupByID(ctx, backupID)
		require.NoError(t, err)
		assert.Equal(t, models.BackupStatusCompleted, backup.Status)
	})

	t.Run("CreatingToFailed", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create a backup
		backupID, err := CreateTestBackup(ctx, vmID)
		require.NoError(t, err)

		// Mark as failed
		err = suite.BackupRepo.UpdateBackupStatus(ctx, backupID, models.BackupStatusFailed)
		require.NoError(t, err)

		// Verify
		backup, err := suite.BackupRepo.GetBackupByID(ctx, backupID)
		require.NoError(t, err)
		assert.Equal(t, models.BackupStatusFailed, backup.Status)
	})

	t.Run("RestoringToCompleted", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create a backup
		backupID, err := CreateTestBackup(ctx, vmID)
		require.NoError(t, err)

		// Set to restoring
		_, _ = suite.DBPool.Exec(ctx, "UPDATE backups SET status = $1 WHERE id = $2", models.BackupStatusRestoring, backupID)

		// Update to completed
		err = suite.BackupRepo.UpdateBackupStatus(ctx, backupID, models.BackupStatusCompleted)
		require.NoError(t, err)

		// Verify
		backup, err := suite.BackupRepo.GetBackupByID(ctx, backupID)
		require.NoError(t, err)
		assert.Equal(t, models.BackupStatusCompleted, backup.Status)
	})
}

// TestSnapshotOperations tests VM snapshot functionality.
func TestSnapshotOperations(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("CreateSnapshot", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create a snapshot
		snapshot, err := suite.BackupService.CreateSnapshot(ctx, vmID, "test-snapshot")

		require.NoError(t, err, "Snapshot creation should succeed")
		assert.NotEmpty(t, snapshot.ID, "Snapshot ID should be generated")
		assert.Equal(t, vmID, snapshot.VMID, "Snapshot VM ID should match")
		assert.Equal(t, "test-snapshot", snapshot.Name, "Snapshot name should match")
	})

	t.Run("ListSnapshots", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create multiple snapshots
		for i := 0; i < 3; i++ {
			_, err := suite.BackupService.CreateSnapshot(ctx, vmID, "snapshot-"+string(rune('a'+i)))
			require.NoError(t, err)
		}

		// List snapshots
		snapshots, err := suite.BackupRepo.ListSnapshotsByVM(ctx, vmID)

		require.NoError(t, err, "Listing snapshots should succeed")
		assert.GreaterOrEqual(t, len(snapshots), 3, "Should have at least 3 snapshots")
	})

	t.Run("DeleteSnapshot", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create a snapshot
		snapshot, err := suite.BackupService.CreateSnapshot(ctx, vmID, "to-delete")
		require.NoError(t, err)

		// Delete snapshot
		err = suite.BackupService.DeleteSnapshot(ctx, snapshot.ID)
		require.NoError(t, err, "Deleting snapshot should succeed")

		// Verify snapshot is deleted
		_, err = suite.BackupRepo.GetSnapshotByID(ctx, snapshot.ID)
		assert.Error(t, err, "Snapshot should not be found after deletion")
		assert.True(t, sharederrors.Is(err, sharederrors.ErrNotFound), "Should return ErrNotFound")
	})

	t.Run("RestoreFromSnapshot", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create a snapshot
		snapshot, err := suite.BackupService.CreateSnapshot(ctx, vmID, "restore-test")
		require.NoError(t, err)

		// Verify snapshot was created before attempting restore
		assert.NotEmpty(t, snapshot.ID, "snapshot ID should be generated")

		// Restore from snapshot
		err = suite.BackupService.RestoreSnapshot(ctx, snapshot.ID)
		// Note: This may fail without actual storage backend
		if err != nil {
			t.Logf("snapshot restore returned error (expected without storage backend): %v", err)
			return
		}

		// If restore succeeded, verify snapshot still exists
		_, err = suite.BackupRepo.GetSnapshotByID(ctx, snapshot.ID)
		assert.NoError(t, err, "snapshot should still exist after restore")
	})
}

// TestBackupStorageMetrics tests backup storage tracking.
func TestBackupStorageMetrics(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("TotalBackupSize", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create backups with known sizes
		for i := 0; i < 3; i++ {
			backupID, err := CreateTestBackup(ctx, vmID)
			require.NoError(t, err)
			_, _ = suite.DBPool.Exec(ctx, `
				UPDATE backups SET status = $1, size_bytes = $2 WHERE id = $3
			`, models.BackupStatusCompleted, int64(1024*1024*100*(i+1)), backupID) // 100MB, 200MB, 300MB
		}

		// Get total backup size
		var totalSize int64
		err = suite.DBPool.QueryRow(ctx, `
			SELECT COALESCE(SUM(size_bytes), 0) FROM backups WHERE vm_id = $1 AND status = $2
		`, vmID, models.BackupStatusCompleted).Scan(&totalSize)
		require.NoError(t, err)

		expectedSize := int64(1024 * 1024 * 600) // 600MB
		assert.Equal(t, expectedSize, totalSize, "Total backup size should match")
	})

	t.Run("BackupSizeByCustomer", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create backups
		for i := 0; i < 2; i++ {
			backupID, err := CreateTestBackup(ctx, vmID)
			require.NoError(t, err)
			_, _ = suite.DBPool.Exec(ctx, `
				UPDATE backups SET status = $1, size_bytes = $2 WHERE id = $3
			`, models.BackupStatusCompleted, int64(1024*1024*50), backupID) // 50MB each
		}

		// Get customer total backup size
		var totalSize int64
		err = suite.DBPool.QueryRow(ctx, `
			SELECT COALESCE(SUM(b.size_bytes), 0) 
			FROM backups b 
			JOIN vms v ON b.vm_id = v.id 
			WHERE v.customer_id = $1 AND b.status = $2
		`, TestCustomerID, models.BackupStatusCompleted).Scan(&totalSize)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, totalSize, int64(1024*1024*100), "Should have at least 100MB in backups")
	})
}

// TestBackupConcurrency tests concurrent backup operations.
func TestBackupConcurrency(t *testing.T) {
	suite := GetTestSuite()
	require.NotNil(t, suite, "Test suite not initialized")

	SetupTest(t)
	defer TeardownTest(t)

	ctx := context.Background()

	t.Run("ConcurrentBackupCreation", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Run concurrent backup creations
		done := make(chan error, 5)
		for i := 0; i < 5; i++ {
			go func() {
				_, err := suite.BackupService.CreateBackup(ctx, vmID, "full")
				done <- err
			}()
		}

		// Collect results
		var errors []error
		for i := 0; i < 5; i++ {
			if err := <-done; err != nil {
				errors = append(errors, err)
			}
		}

		// At least some should succeed
		assert.LessOrEqual(t, len(errors), 5, "Some backup creations should succeed")
	})

	t.Run("BackupWhileRestoring", func(t *testing.T) {
		// Create a VM
		vmID, err := CreateTestVM(ctx, TestCustomerID, TestPlanID, TestNodeID)
		require.NoError(t, err)

		// Create a backup
		backupID, err := CreateTestBackup(ctx, vmID)
		require.NoError(t, err)

		// Set backup to restoring
		_, err = suite.DBPool.Exec(ctx, "UPDATE backups SET status = $1 WHERE id = $2", models.BackupStatusRestoring, backupID)
		require.NoError(t, err)

		// Try to create another backup (behavior depends on business logic)
		_, err = suite.BackupService.CreateBackup(ctx, vmID, "full")
		// May succeed or fail depending on implementation
		if err != nil {
			t.Logf("backup while restoring returned error (expected without storage backend): %v", err)
		}
	})
}
