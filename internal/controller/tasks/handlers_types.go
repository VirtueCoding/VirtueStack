// Package tasks provides shared types and interfaces for task handlers.
// This file contains all the core types, interfaces, and payload structures
// used across different task handlers in the package.
package tasks

import (
	"context"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"log/slog"
)

// MACPrefix is the OUI prefix for VirtueStack VM MAC addresses.
// Uses QEMU's default prefix (52:54:00) for compatibility.
const MACPrefix = "52:54:00"

// HandlerDeps contains all dependencies required by task handlers.
type HandlerDeps struct {
	VMRepo         *repository.VMRepository
	NodeRepo       *repository.NodeRepository
	IPRepo         *repository.IPRepository
	BackupRepo     *repository.BackupRepository
	TaskRepo       *repository.TaskRepository
	TemplateRepo   *repository.TemplateRepository
	IPAMService    IPAMService
	NodeClient     NodeAgentClient
	DNSNameservers []string
	CephUser       string
	CephSecretUUID string
	CephMonitors   []string
	Logger         *slog.Logger
}

// IPAMService defines the interface for IP address management operations.
type IPAMService interface {
	AllocateIPv4(ctx context.Context, vmID, customerID, locationID string) (*models.IPAddress, error)
	AllocateIPv6(ctx context.Context, vmID, customerID, nodeID string) (*models.VMIPv6Subnet, error)
	ReleaseIPsByVM(ctx context.Context, vmID string) error
	GetPrimaryIPv4(ctx context.Context, vmID string) (*models.IPAddress, error)
	GetIPv6SubnetsByVM(ctx context.Context, vmID string) ([]models.VMIPv6Subnet, error)
}

// NodeAgentClient defines the interface for communicating with node agents via gRPC.
type NodeAgentClient interface {
	// CreateVM provisions a new VM on the specified node.
	CreateVM(ctx context.Context, nodeID string, req *CreateVMRequest) (*CreateVMResponse, error)
	// StartVM starts a stopped VM on the specified node.
	StartVM(ctx context.Context, nodeID, vmID string) error
	// StopVM gracefully stops a running VM on the specified node.
	StopVM(ctx context.Context, nodeID, vmID string, timeoutSec int) error
	// ForceStopVM immediately terminates a VM on the specified node.
	ForceStopVM(ctx context.Context, nodeID, vmID string) error
	// DeleteVM removes a VM definition from the specified node.
	DeleteVM(ctx context.Context, nodeID, vmID string) error
	// CreateSnapshot creates a disk snapshot for a VM.
	CreateSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) (*SnapshotResponse, error)
	// DeleteSnapshot removes a disk snapshot for a VM.
	DeleteSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error
	// RestoreSnapshot restores a VM disk from a snapshot.
	RestoreSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error
	// CloneFromBackup clones a VM disk from a backup snapshot.
	CloneFromBackup(ctx context.Context, nodeID, vmID, backupSnapshot string, diskGB int) error
	// DeleteDisk removes the RBD disk for a VM.
	DeleteDisk(ctx context.Context, nodeID, vmID string) error
	// CloneFromTemplate clones a disk from a template for a VM.
	CloneFromTemplate(ctx context.Context, nodeID, vmID, templateImage, templateSnapshot string, diskGB int) error
	// GenerateCloudInit generates a cloud-init ISO for a VM.
	GenerateCloudInit(ctx context.Context, nodeID string, cfg *CloudInitConfig) (string, error)
	// MigrateVM initiates a live migration of a VM to a target node.
	MigrateVM(ctx context.Context, sourceNodeID, targetNodeID, vmID string, opts *MigrateVMOptions) error
	// AbortMigration aborts an in-progress migration on the specified node.
	AbortMigration(ctx context.Context, nodeID, vmID string) error
	// PostMigrateSetup re-applies tc throttling and nwfilter on the target node after migration.
	PostMigrateSetup(ctx context.Context, nodeID, vmID string, bandwidthMbps int) error
	// GuestFreezeFilesystems freezes all filesystems in the VM via QEMU guest agent.
	// Used for consistent backup operations. Returns the number of frozen filesystems.
	GuestFreezeFilesystems(ctx context.Context, nodeID, vmID string) (int, error)
	// GuestThawFilesystems unfreezes all filesystems in the VM via QEMU guest agent.
	// Used after backup operations are complete. Returns the number of thawed filesystems.
	GuestThawFilesystems(ctx context.Context, nodeID, vmID string) (int, error)
	// ProtectSnapshot protects a snapshot from deletion, required before cloning.
	ProtectSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error
	// UnprotectSnapshot removes protection from a snapshot, allowing deletion.
	UnprotectSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error
	// CloneSnapshot clones a protected snapshot to a target pool.
	// Returns the name of the cloned image in the target pool.
	CloneSnapshot(ctx context.Context, nodeID, vmID, snapshotName, targetPool string) (string, error)
	// CreateQCOWSnapshot creates a qemu-img internal snapshot for QCOW-backed VMs.
	CreateQCOWSnapshot(ctx context.Context, nodeID, vmID, diskPath, snapshotName string) error
	// DeleteQCOWSnapshot deletes a qemu-img internal snapshot for QCOW-backed VMs.
	DeleteQCOWSnapshot(ctx context.Context, nodeID, vmID, diskPath, snapshotName string) error
	// CreateQCOWBackup creates a backup file from a QCOW disk using qemu-img convert.
	// If snapshotName is provided, it exports from that specific snapshot.
	// Returns the size of the backup file in bytes.
	CreateQCOWBackup(ctx context.Context, nodeID, vmID, diskPath, snapshotName, backupPath string, compress bool) (int64, error)
	// RestoreQCOWBackup restores a VM from a QCOW backup file.
	RestoreQCOWBackup(ctx context.Context, nodeID, vmID, backupPath, targetPath string) error
	// DeleteQCOWBackupFile deletes a QCOW backup file from the backup storage.
	DeleteQCOWBackupFile(ctx context.Context, nodeID, backupPath string) error
	// GetQCOWDiskInfo returns information about a QCOW disk including size.
	GetQCOWDiskInfo(ctx context.Context, nodeID, diskPath string) (*QCOWDiskInfo, error)
	// ResizeVM modifies the resource allocation (vCPU, memory, disk) for a VM on the specified node.
	ResizeVM(ctx context.Context, nodeID, vmID string, vcpu, memoryMB, diskGB int) error
	// TransferDisk transfers a disk from a source node to a target node via streaming gRPC.
	TransferDisk(ctx context.Context, opts *DiskTransferOptions) error
	// PrepareMigratedVM creates a VM definition on the target node using a transferred disk.
	PrepareMigratedVM(ctx context.Context, targetNodeID, vmID, diskPath string, vm *models.VM) error
}

// CreateVMRequest contains parameters for VM creation via node agent.
type CreateVMRequest struct {
	VMID                string
	Hostname            string
	VCPU                int
	MemoryMB            int
	DiskGB              int
	TemplateRBDImage    string
	TemplateRBDSnapshot string
	RootPasswordHash    string
	SSHPublicKeys       []string
	IPv4Address         string
	IPv4Gateway         string
	IPv6Address         string
	IPv6Gateway         string
	MACAddress          string
	PortSpeedMbps       int
	CephMonitors        []string
	CephUser            string
	CephSecretUUID      string
	CephPool            string
	Nameservers         []string
}

// CreateVMResponse contains the result of a VM creation operation.
type CreateVMResponse struct {
	DomainName string
	VNCPort    int32
}

// SnapshotResponse contains the result of a snapshot creation operation.
type SnapshotResponse struct {
	SnapshotID      string
	RBDSnapshotName string
	SizeBytes       int64
}

// QCOWDiskInfo holds information about a QCOW disk returned by GetQCOWDiskInfo.
type QCOWDiskInfo struct {
	DiskPath    string
	TotalDiskGB uint64
	UsedDiskGB  uint64
}

// CloudInitConfig contains parameters for cloud-init ISO generation.
type CloudInitConfig struct {
	VMID             string
	Hostname         string
	RootPasswordHash string
	SSHPublicKeys    []string
	IPv4Address      string
	IPv4Gateway      string
	IPv6Address      string
	IPv6Gateway      string
	Nameservers      []string
}

// MigrateVMOptions contains options for VM live migration.
type MigrateVMOptions struct {
	TargetNodeAddress  string // gRPC address of the target node
	BandwidthLimitMbps int    // Bandwidth limit for migration traffic
	Compression        bool   // Enable compression during migration
	AutoConverge       bool   // Force convergence if migration stalls
}

// DiskTransferOptions contains options for disk transfer between nodes.
type DiskTransferOptions struct {
	SourceNodeID         string    // ID of the source node
	TargetNodeID         string    // ID of the target node
	SourceDiskPath       string    // Path to the source disk file
	TargetDiskPath       string    // Path where disk will be stored on target
	SnapshotName         string    // Optional snapshot name for consistent copy
	DiskSizeGB           int       // Size of the disk in GB
	SourceStorageBackend string    // Storage backend of source (ceph/qcow)
	TargetStorageBackend string    // Storage backend of target (ceph/qcow)
	Compress             bool      // Enable compression during transfer
	ConvertFormat        bool      // Convert disk format (for mixed storage)
	ProgressCallback     func(int) // Callback for progress updates (0-100)
}

// VMCreatePayload represents the payload for vm.create tasks.
type VMCreatePayload struct {
	VMID       string   `json:"vm_id"`
	NodeID     string   `json:"node_id"`
	Hostname   string   `json:"hostname"`
	VCPU       int      `json:"vcpu"`
	MemoryMB   int      `json:"memory_mb"`
	DiskGB     int      `json:"disk_gb"`
	TemplateID string   `json:"template_id"`
	CustomerID string   `json:"customer_id"`
	SSHKeys    []string `json:"ssh_keys"`
	UserData   string   `json:"user_data"`
	Password   string   `json:"password"` // Plain password, will be hashed
}

// VMReinstallPayload represents the payload for vm.reinstall tasks.
type VMReinstallPayload struct {
	VMID       string   `json:"vm_id"`
	TemplateID string   `json:"template_id"`
	SSHKeys    []string `json:"ssh_keys"`
	Password   string   `json:"password"`
}

// VMDeletePayload represents the payload for vm.delete tasks.
type VMDeletePayload struct {
	VMID string `json:"vm_id"`
}

// MigrationStrategy defines the type of migration to perform.
type MigrationStrategy string

const (
	// MigrationStrategyLiveSharedStorage indicates live migration with shared storage (Ceph).
	// No disk copy needed, VM remains running during migration.
	MigrationStrategyLiveSharedStorage MigrationStrategy = "live_shared"
	// MigrationStrategyDiskCopy indicates migration requiring disk copy between nodes (QCOW).
	// For running VMs: copy disk, sync delta, switchover.
	// For stopped VMs: simple disk copy.
	MigrationStrategyDiskCopy MigrationStrategy = "disk_copy"
	// MigrationStrategyCold indicates cold migration with format conversion.
	// Used for mixed storage (Ceph↔QCOW) migrations.
	MigrationStrategyCold MigrationStrategy = "cold"
)

// VMMigratePayload represents the payload for vm.migrate tasks.
type VMMigratePayload struct {
	VMID                 string            `json:"vm_id"`
	SourceNodeID         string            `json:"source_node_id"`
	TargetNodeID         string            `json:"target_node_id"`
	PreMigrationState    string            `json:"pre_migration_state,omitempty"`
	SourceStorageBackend string            `json:"source_storage_backend,omitempty"`
	TargetStorageBackend string            `json:"target_storage_backend,omitempty"`
	SourceStoragePath    string            `json:"source_storage_path,omitempty"`
	TargetStoragePath    string            `json:"target_storage_path,omitempty"`
	SourceCephPool       string            `json:"source_ceph_pool,omitempty"`
	TargetCephPool       string            `json:"target_ceph_pool,omitempty"`
	MigrationStrategy    MigrationStrategy `json:"migration_strategy"`
	Live                 bool              `json:"live"`
	SourceDiskPath       string            `json:"source_disk_path,omitempty"`
	TargetDiskPath       string            `json:"target_disk_path,omitempty"`
	DiskSizeGB           int               `json:"disk_size_gb,omitempty"`
	// DiskTransferProgress tracks the progress of disk copy (0-100).
	DiskTransferProgress int `json:"disk_transfer_progress,omitempty"`
}

// BackupCreatePayload represents the payload for backup.create tasks.
type BackupCreatePayload struct {
	VMID       string `json:"vm_id"`
	BackupName string `json:"backup_name"`
	BackupType string `json:"backup_type"` // "full" or "incremental"
}

// BackupRestorePayload represents the payload for backup.restore tasks.
type BackupRestorePayload struct {
	BackupID string `json:"backup_id"`
	VMID     string `json:"vm_id"`
}