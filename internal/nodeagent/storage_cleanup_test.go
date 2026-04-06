package nodeagent

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateVMWithRollbackCleanup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		createErr          error
		cleanupErr         error
		wantCleanupCalls   int
		wantErrContains    string
		wantErrNotContains string
	}{
		{
			name:             "skips cleanup when create succeeds",
			wantCleanupCalls: 0,
		},
		{
			name:             "runs cleanup when create fails",
			createErr:        errors.New("define domain failed"),
			wantCleanupCalls: 1,
			wantErrContains:  "define domain failed",
		},
		{
			name:             "wraps cleanup failure when create and cleanup both fail",
			createErr:        errors.New("define domain failed"),
			cleanupErr:       errors.New("delete disk failed"),
			wantCleanupCalls: 1,
			wantErrContains:  "cleanup owned storage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanupCalls := 0

			err := createVMWithRollbackCleanup(
				context.Background(),
				func(context.Context) error {
					return tt.createErr
				},
				func(context.Context) error {
					cleanupCalls++
					return tt.cleanupErr
				},
			)

			assert.Equal(t, tt.wantCleanupCalls, cleanupCalls)
			if tt.wantErrContains == "" {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			assert.ErrorContains(t, err, tt.wantErrContains)
			if tt.wantErrNotContains != "" {
				assert.NotContains(t, err.Error(), tt.wantErrNotContains)
			}
		})
	}
}

func TestOwnedPrepareCleanupForBackend(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		storageType     string
		requestedDiskID string
		canonicalDiskID string
		ownedDiskID     string
		wantHasCleanup  bool
		wantCleanupDisk string
	}{
		{
			name:            "qcow cleans canonical migrated disk",
			storageType:     "qcow",
			requestedDiskID: "/var/lib/virtuestack/vms/vm-1-disk0.qcow2",
			canonicalDiskID: "/var/lib/virtuestack/vms/vm-1-disk0.qcow2",
			ownedDiskID:     "/var/lib/virtuestack/vms/vm-1-disk0.qcow2",
			wantHasCleanup:  true,
			wantCleanupDisk: "/var/lib/virtuestack/vms/vm-1-disk0.qcow2",
		},
		{
			name:            "qcow cleans normalized migrated disk path",
			storageType:     "qcow",
			requestedDiskID: "/var/lib/virtuestack/vms/./vm-1-disk0.qcow2",
			canonicalDiskID: "/var/lib/virtuestack/vms/vm-1-disk0.qcow2",
			ownedDiskID:     "/var/lib/virtuestack/vms/vm-1-disk0.qcow2",
			wantHasCleanup:  true,
			wantCleanupDisk: "/var/lib/virtuestack/vms/vm-1-disk0.qcow2",
		},
		{
			name:            "lvm cleans canonical migrated LV",
			storageType:     "lvm",
			requestedDiskID: "/dev/vg/vs-vm-1-disk0",
			canonicalDiskID: "/dev/vg/vs-vm-1-disk0",
			ownedDiskID:     "/dev/vg/vs-vm-1-disk0",
			wantHasCleanup:  true,
			wantCleanupDisk: "/dev/vg/vs-vm-1-disk0",
		},
		{
			name:            "qcow skips backup restore source path",
			storageType:     "qcow",
			requestedDiskID: "/var/backups/vm-1-backup.qcow2",
			canonicalDiskID: "/var/lib/virtuestack/vms/vm-1-disk0.qcow2",
			ownedDiskID:     "/var/backups/vm-1-backup.qcow2",
			wantHasCleanup:  false,
		},
		{
			name:            "ceph skips cleanup without explicit ownership",
			storageType:     "ceph",
			requestedDiskID: "vm-1-disk0",
			canonicalDiskID: "vs-vm-1-disk0",
			ownedDiskID:     "vm-1-disk0",
			wantHasCleanup:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := ownedPrepareCleanupForBackend(
				tt.storageType,
				tt.requestedDiskID,
				tt.canonicalDiskID,
				tt.ownedDiskID,
				func(_ context.Context, diskID string) error {
					assert.Equal(t, tt.wantCleanupDisk, diskID)
					return nil
				},
			)

			if !tt.wantHasCleanup {
				assert.Nil(t, cleanup)
				return
			}

			require.NotNil(t, cleanup)
			require.NoError(t, cleanup(context.Background(), "ignored-vm-id"))
		})
	}
}
