package tasks

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVMCreateResult_JSONSerialization(t *testing.T) {
	result := VMCreateResult{
		VMID:          "vm-123",
		DomainName:    "vs-vm-123",
		VNCPort:       5900,
		IPv4Address:   "10.0.0.5",
		IPv6Subnet:    "2001:db8::/64",
		CloudInitPath: "/var/lib/virtuestack/cloud-init/vm-123.iso",
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var decoded VMCreateResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, result, decoded)
}

func TestVMDeleteResult_JSONSerialization(t *testing.T) {
	result := VMDeleteResult{
		VMID:   "vm-123",
		Status: "deleted",
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var decoded VMDeleteResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, result, decoded)
}

func TestBackupCreateResult_JSONSerialization(t *testing.T) {
	result := BackupCreateResult{
		BackupID:          "backup-123",
		VMID:              "vm-456",
		SnapshotName:      "snap-2026-03-15",
		StoragePath:       "vs-backups/vm-456/snap-2026-03-15",
		SizeBytes:         5368709120,
		Consistency:       "crash-consistent",
		FrozenFilesystems: 2,
		StorageBackend:    "ceph",
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var decoded BackupCreateResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, result, decoded)
}

func TestBackupCreateResult_QCOW_JSONSerialization(t *testing.T) {
	result := BackupCreateResult{
		BackupID:       "backup-123",
		VMID:           "vm-456",
		SnapshotName:   "snap-2026-03-15",
		Filepath:       "/var/lib/virtuestack/backups/vm-456/snap.qcow2",
		SizeBytes:      2147483648,
		StorageBackend: "qcow",
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	// Verify StoragePath is omitted for QCOW
	var raw map[string]interface{}
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)
	assert.NotEmpty(t, raw["file_path"])
	assert.Empty(t, raw["storage_path"])

	var decoded BackupCreateResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, result, decoded)
}

func TestBackupRestoreResult_JSONSerialization(t *testing.T) {
	result := BackupRestoreResult{
		BackupID: "backup-123",
		VMID:     "vm-456",
		Status:   "completed",
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var decoded BackupRestoreResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, result, decoded)
}

func TestVMMigrateResult_JSONSerialization(t *testing.T) {
	result := VMMigrateResult{
		VMID:                 "vm-123",
		SourceNodeID:         "node-1",
		TargetNodeID:         "node-2",
		Status:               "completed",
		SourceNodeAddress:    "10.0.0.1:50051",
		TargetNodeAddress:    "10.0.0.2:50051",
		MigrationStrategy:    "live",
		SourceStorageBackend: "ceph",
		TargetStorageBackend: "ceph",
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var decoded VMMigrateResult
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, result, decoded)
}

func TestVMMigrateResult_OptionalFields_Omitted(t *testing.T) {
	result := VMMigrateResult{
		VMID:         "vm-123",
		SourceNodeID: "node-1",
		TargetNodeID: "node-2",
		Status:       "in_progress",
	}

	data, err := json.Marshal(result)
	require.NoError(t, err)

	var raw map[string]interface{}
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)

	// Optional fields should be omitted when empty
	assert.Empty(t, raw["source_node_address"])
	assert.Empty(t, raw["target_node_address"])
	assert.Empty(t, raw["migration_strategy"])
}
