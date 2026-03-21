package services

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// strPtr is a helper to create string pointers for test data.
func strPtr(s string) *string { return &s }

func testMigrationLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestCheckNodeCapacity_SufficientResources(t *testing.T) {
	logger := testMigrationLogger()

	svc := NewMigrationService(nil, nil, nil, nil, nil, logger)

	node := &models.Node{
		ID:                "node-1",
		TotalVCPU:         32,
		AllocatedVCPU:     10,
		TotalMemoryMB:     65536,
		AllocatedMemoryMB: 16384,
	}

	vm := &models.VM{
		ID:       "vm-123",
		VCPU:     4,
		MemoryMB: 8192,
	}

	err := svc.checkNodeCapacity(node, vm)
	assert.NoError(t, err)
}

func TestCheckNodeCapacity_InsufficientCPU(t *testing.T) {
	logger := testMigrationLogger()

	svc := NewMigrationService(nil, nil, nil, nil, nil, logger)

	node := &models.Node{
		ID:                "node-1",
		TotalVCPU:         32,
		AllocatedVCPU:     30,
		TotalMemoryMB:     65536,
		AllocatedMemoryMB: 16384,
	}

	vm := &models.VM{
		ID:       "vm-123",
		VCPU:     4,
		MemoryMB: 8192,
	}

	err := svc.checkNodeCapacity(node, vm)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient vCPU")
}

func TestCheckNodeCapacity_InsufficientMemory(t *testing.T) {
	logger := testMigrationLogger()

	svc := NewMigrationService(nil, nil, nil, nil, nil, logger)

	node := &models.Node{
		ID:                "node-1",
		TotalVCPU:         32,
		AllocatedVCPU:     10,
		TotalMemoryMB:     65536,
		AllocatedMemoryMB: 60000,
	}

	vm := &models.VM{
		ID:       "vm-123",
		VCPU:     4,
		MemoryMB: 8192,
	}

	err := svc.checkNodeCapacity(node, vm)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient memory")
}

func TestGetNodeStorageBackend(t *testing.T) {
	logger := testMigrationLogger()

	svc := NewMigrationService(nil, nil, nil, nil, nil, logger)

	tests := []struct {
		name     string
		vm       *models.VM
		expected string
	}{
		{
			name:     "explicit ceph backend",
			vm:       &models.VM{StorageBackend: models.StorageBackendCeph},
			expected: models.StorageBackendCeph,
		},
		{
			name:     "explicit qcow backend",
			vm:       &models.VM{StorageBackend: models.StorageBackendQcow},
			expected: models.StorageBackendQcow,
		},
		{
			name:     "default to ceph",
			vm:       &models.VM{StorageBackend: ""},
			expected: models.StorageBackendCeph,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.getNodeStorageBackend(tt.vm)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetermineMigrationStrategy(t *testing.T) {
	logger := testMigrationLogger()

	svc := NewMigrationService(nil, nil, nil, nil, nil, logger)

	tests := []struct {
		name           string
		storageBackend string
		live           bool
		expected       string
	}{
		{
			name:           "ceph live migration",
			storageBackend: models.StorageBackendCeph,
			live:           true,
			expected:       "live_shared",
		},
		{
			name:           "ceph offline migration",
			storageBackend: models.StorageBackendCeph,
			live:           false,
			expected:       "live_shared",
		},
		{
			name:           "qcow migration",
			storageBackend: models.StorageBackendQcow,
			live:           true,
			expected:       "disk_copy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.determineMigrationStrategy(tt.storageBackend, tt.live)
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestValidateStorageBackend(t *testing.T) {
	logger := testMigrationLogger()

	svc := NewMigrationService(nil, nil, nil, nil, nil, logger)

	tests := []struct {
		name           string
		storageBackend string
		wantErr        bool
	}{
		{"valid ceph", models.StorageBackendCeph, false},
		{"valid qcow", models.StorageBackendQcow, false},
		{"invalid backend", "unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.validateStorageBackend(tt.storageBackend)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unknown storage backend")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetVMDiskPath(t *testing.T) {
	logger := testMigrationLogger()

	svc := NewMigrationService(nil, nil, nil, nil, nil, logger)

	tests := []struct {
		name     string
		vm       *models.VM
		node     *models.Node
		expected string
	}{
		{
			name:     "explicit disk path",
			vm:       &models.VM{ID: "vm-123", DiskPath: strPtr("/custom/path/disk.qcow2")},
			node:     &models.Node{ID: "node-1"},
			expected: "/custom/path/disk.qcow2",
		},
		{
			name:     "ceph with rbd image",
			vm:       &models.VM{ID: "vm-123", StorageBackend: models.StorageBackendCeph, RBDImage: strPtr("vm-123-disk0")},
			node:     &models.Node{ID: "node-1"},
			expected: "vm-123-disk0",
		},
		{
			name:     "ceph default rbd image",
			vm:       &models.VM{ID: "vm-123", StorageBackend: models.StorageBackendCeph},
			node:     &models.Node{ID: "node-1"},
			expected: "vm-vm-123-disk0",
		},
		{
			name:     "qcow with storage path",
			vm:       &models.VM{ID: "vm-123", StorageBackend: models.StorageBackendQcow},
			node:     &models.Node{ID: "node-1", StoragePath: "/data/vms"},
			expected: "/data/vms/vm-123-disk0.qcow2",
		},
		{
			name:     "qcow default path",
			vm:       &models.VM{ID: "vm-123", StorageBackend: models.StorageBackendQcow},
			node:     &models.Node{ID: "node-1"},
			expected: "/var/lib/virtuestack/vms/vm-123-disk0.qcow2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := svc.getVMDiskPath(tt.vm, tt.node)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateTargetNode_SameAsSource(t *testing.T) {
	logger := testMigrationLogger()
	ctx := context.Background()

	svc := NewMigrationService(nil, nil, nil, nil, nil, logger)

	vm := &models.VM{ID: "vm-123"}
	node, err := svc.validateTargetNode(ctx, "node-1", "node-1", vm)
	require.Error(t, err)
	assert.Nil(t, node)
	assert.Contains(t, err.Error(), "cannot be the same as source")
}