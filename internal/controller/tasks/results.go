// Package tasks provides async task handlers for VM operations.
package tasks

// VMCreateResult represents the result of a vm.create task.
// This struct is serialized to JSON and stored in task.Result field.
type VMCreateResult struct {
	VMID         string `json:"vm_id"`
	DomainName   string `json:"domain_name"`
	VNCPort      int32  `json:"vnc_port"`
	IPv4Address  string `json:"ipv4_address"`
	IPv6Subnet   string `json:"ipv6_subnet"`
	CloudInitPath string `json:"cloud_init_path"`
}

// VMDeleteResult represents the result of a vm.delete task.
// This struct is serialized to JSON and stored in task.Result field.
type VMDeleteResult struct {
	VMID   string `json:"vm_id"`
	Status string `json:"status"`
}

// BackupCreateResult represents the result of a backup.create task.
// This struct is serialized to JSON and stored in task.Result field.
// Fields Filepath and StoragePath are mutually exclusive:
// - Filepath is used for QCOW backups (file_path key)
// - StoragePath is used for Ceph backups (storage_path key)
type BackupCreateResult struct {
	BackupID         string `json:"backup_id"`
	VMID             string `json:"vm_id"`
	SnapshotName     string `json:"snapshot_name"`
	Filepath         string `json:"file_path,omitempty"`
	StoragePath      string `json:"storage_path,omitempty"`
	SizeBytes        int64  `json:"size_bytes"`
	Consistency      string `json:"consistency"`
	FrozenFilesystems int   `json:"frozen_filesystems"`
	StorageBackend   string `json:"storage_backend"`
}

// BackupRestoreResult represents the result of a backup.restore task.
// This struct is serialized to JSON and stored in task.Result field.
type BackupRestoreResult struct {
	BackupID string `json:"backup_id"`
	VMID     string `json:"vm_id"`
	Status   string `json:"status"`
}

// VMMigrateResult represents the result of a vm.migrate task.
// This struct is serialized to JSON and stored in task.Result field.
// Some fields are optional and only populated for completed migrations:
// - SourceNodeAddress, TargetNodeAddress: populated for completed migrations
// - MigrationStrategy, SourceStorageBackend, TargetStorageBackend: populated for completed migrations
type VMMigrateResult struct {
	VMID                 string `json:"vm_id"`
	SourceNodeID         string `json:"source_node_id"`
	TargetNodeID         string `json:"target_node_id"`
	Status               string `json:"status"`
	SourceNodeAddress    string `json:"source_node_address,omitempty"`
	TargetNodeAddress    string `json:"target_node_address,omitempty"`
	MigrationStrategy    string `json:"migration_strategy,omitempty"`
	SourceStorageBackend string `json:"source_storage_backend,omitempty"`
	TargetStorageBackend string `json:"target_storage_backend,omitempty"`
}