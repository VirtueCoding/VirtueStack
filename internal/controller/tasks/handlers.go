// Package tasks provides async task handlers for VM operations.
package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"unicode"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/alexedwards/argon2id"
)

// HandlerDeps contains all dependencies required by task handlers.
type HandlerDeps struct {
	VMRepo      *repository.VMRepository
	NodeRepo    *repository.NodeRepository
	IPRepo      *repository.IPRepository
	BackupRepo  *repository.BackupRepository
	TaskRepo    *repository.TaskRepository
	TemplateRepo *repository.TemplateRepository
	IPAMService IPAMService
	NodeClient  NodeAgentClient
	Logger      *slog.Logger
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
}

// CreateVMRequest contains parameters for VM creation via node agent.
type CreateVMRequest struct {
	VMID               string
	Hostname           string
	VCPU               int
	MemoryMB           int
	DiskGB             int
	TemplateRBDImage   string
	TemplateRBDSnapshot string
	RootPasswordHash   string
	SSHPublicKeys      []string
	IPv4Address        string
	IPv4Gateway        string
	IPv6Address        string
	IPv6Gateway        string
	MACAddress         string
	PortSpeedMbps      int
	CephMonitors       []string
	CephUser           string
	CephSecretUUID     string
	CephPool           string
	Nameservers        []string
}

// CreateVMResponse contains the result of a VM creation operation.
type CreateVMResponse struct {
	DomainName string
	VNCPort    int32
}

// SnapshotResponse contains the result of a snapshot creation operation.
type SnapshotResponse struct {
	SnapshotID     string
	RBDSnapshotName string
	SizeBytes      int64
}

// CloudInitConfig contains parameters for cloud-init ISO generation.
type CloudInitConfig struct {
	VMID            string
	Hostname        string
	RootPasswordHash string
	SSHPublicKeys   []string
	IPv4Address     string
	IPv4Gateway     string
	IPv6Address     string
	IPv6Gateway     string
	Nameservers     []string
}

// MigrateVMOptions contains options for VM live migration.
type MigrateVMOptions struct {
	TargetNodeAddress string // gRPC address of the target node
	BandwidthLimitMbps int    // Bandwidth limit for migration traffic
	Compression       bool   // Enable compression during migration
	AutoConverge      bool   // Force convergence if migration stalls
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

// VMMigratePayload represents the payload for vm.migrate tasks.
type VMMigratePayload struct {
	VMID         string `json:"vm_id"`
	SourceNodeID string `json:"source_node_id"`
	TargetNodeID string `json:"target_node_id"`
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

// RegisterAllHandlers registers all task handlers with the worker.
func RegisterAllHandlers(worker *Worker, deps *HandlerDeps) {
	worker.RegisterHandler(models.TaskTypeVMCreate, func(ctx context.Context, task *models.Task) error {
		return handleVMCreate(ctx, task, deps)
	})
	worker.RegisterHandler(models.TaskTypeVMReinstall, func(ctx context.Context, task *models.Task) error {
		return handleVMReinstall(ctx, task, deps)
	})
	worker.RegisterHandler(models.TaskTypeVMMigrate, func(ctx context.Context, task *models.Task) error {
		return handleVMMigrate(ctx, task, deps)
	})
	worker.RegisterHandler(models.TaskTypeBackupCreate, func(ctx context.Context, task *models.Task) error {
		return handleBackupCreate(ctx, task, deps)
	})
	worker.RegisterHandler("vm.delete", func(ctx context.Context, task *models.Task) error {
		return handleVMDelete(ctx, task, deps)
	})
	worker.RegisterHandler(models.TaskTypeBackupRestore, func(ctx context.Context, task *models.Task) error {
		return handleBackupRestore(ctx, task, deps)
	})
	worker.RegisterHandler(models.TaskTypeSnapshotCreate, func(ctx context.Context, task *models.Task) error {
		return handleSnapshotCreate(ctx, task, deps)
	})
	worker.RegisterHandler(models.TaskTypeSnapshotRevert, func(ctx context.Context, task *models.Task) error {
		return handleSnapshotRevert(ctx, task, deps)
	})
	worker.RegisterHandler(models.TaskTypeSnapshotDelete, func(ctx context.Context, task *models.Task) error {
		return handleSnapshotDelete(ctx, task, deps)
	})

	deps.Logger.Info("all task handlers registered",
		"handlers", []string{
			models.TaskTypeVMCreate,
			models.TaskTypeVMReinstall,
			models.TaskTypeVMMigrate,
			models.TaskTypeBackupCreate,
			"vm.delete",
			models.TaskTypeBackupRestore,
			models.TaskTypeSnapshotCreate,
			models.TaskTypeSnapshotRevert,
			models.TaskTypeSnapshotDelete,
		})
}

// handleVMCreate handles the full VM provisioning flow.
// Steps:
//  1. Parse payload
//  2. Clone RBD from template
//  3. Generate cloud-init ISO
//  4. Define and start VM via gRPC
//  5. Allocate IP addresses
//  6. Update VM status
func handleVMCreate(ctx context.Context, task *models.Task, deps *HandlerDeps) error {
	logger := deps.Logger.With("task_id", task.ID, "task_type", models.TaskTypeVMCreate)

	// Parse payload
	var payload VMCreatePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse vm.create payload", "error", err)
		return fmt.Errorf("parsing vm.create payload: %w", err)
	}

	logger = logger.With("vm_id", payload.VMID)
	logger.Info("vm.create task started",
		"node_id", payload.NodeID,
		"hostname", payload.Hostname,
		"template_id", payload.TemplateID)

	// Update task progress: Starting
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 5, "Starting VM provisioning..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Get node information
	node, err := deps.NodeRepo.GetByID(ctx, payload.NodeID)
	if err != nil {
		logger.Error("failed to get node", "node_id", payload.NodeID, "error", err)
		return fmt.Errorf("getting node %s: %w", payload.NodeID, err)
	}

	// Get template information
	template, err := deps.TemplateRepo.GetByID(ctx, payload.TemplateID)
	if err != nil {
		logger.Error("failed to get template", "template_id", payload.TemplateID, "error", err)
		return fmt.Errorf("getting template %s: %w", payload.TemplateID, err)
	}

	// Get VM record to retrieve customer and location info
	vm, err := deps.VMRepo.GetByID(ctx, payload.VMID)
	if err != nil {
		logger.Error("failed to get VM record", "vm_id", payload.VMID, "error", err)
		return fmt.Errorf("getting VM %s: %w", payload.VMID, err)
	}

	// Generate MAC address if not set
	macAddress := vm.MACAddress
	if macAddress == "" {
		macAddress = generateMACAddress(payload.VMID)
	}

	// Update task progress: Cloning disk
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 15, "Cloning disk image from template..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Clone disk from template via node agent
	err = deps.NodeClient.CloneFromTemplate(ctx, payload.NodeID, payload.VMID,
		template.RBDImage, template.RBDSnapshot, payload.DiskGB)
	if err != nil {
		logger.Error("failed to clone template", "error", err)
		return fmt.Errorf("cloning template for VM %s: %w", payload.VMID, err)
	}

	// Update task progress: Generating cloud-init
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 30, "Generating cloud-init configuration..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Allocate IPv4 address if needed
	var ipv4Addr, ipv4Gateway string
	locationID := ""
	if node.LocationID != nil {
		locationID = *node.LocationID
	}

	ip, err := deps.IPAMService.AllocateIPv4(ctx, payload.VMID, payload.CustomerID, locationID)
	if err != nil {
		logger.Warn("failed to allocate IPv4, using DHCP", "error", err)
	} else if ip != nil {
		ipv4Addr = ip.Address
		// Gateway would come from the IP set configuration
	}

	// Allocate IPv6 subnet
	var ipv6Addr, ipv6Gateway string
	ipv6Subnet, err := deps.IPAMService.AllocateIPv6(ctx, payload.VMID, payload.CustomerID, payload.NodeID)
	if err != nil {
		logger.Warn("failed to allocate IPv6 subnet", "error", err)
	} else if ipv6Subnet != nil {
		ipv6Addr = ipv6Subnet.Subnet
		ipv6Gateway = ipv6Subnet.Gateway
	}

	// Hash the password using Argon2id
	passwordHash, err := hashPassword(payload.Password)
	if err != nil {
		return fmt.Errorf("hashing password: %w", err)
	}

	// Generate cloud-init ISO
	cloudInitCfg := &CloudInitConfig{
		VMID:             payload.VMID,
		Hostname:         payload.Hostname,
		RootPasswordHash: passwordHash,
		SSHPublicKeys:    payload.SSHKeys,
		IPv4Address:      ipv4Addr,
		IPv4Gateway:      ipv4Gateway,
		IPv6Address:      ipv6Addr,
		IPv6Gateway:      ipv6Gateway,
		Nameservers:      []string{"8.8.8.8", "8.8.4.4"}, // Default nameservers
	}

	cloudInitPath, err := deps.NodeClient.GenerateCloudInit(ctx, payload.NodeID, cloudInitCfg)
	if err != nil {
		logger.Error("failed to generate cloud-init", "error", err)
		// Cleanup: try to delete the cloned disk
		_ = deps.NodeClient.DeleteDisk(ctx, payload.NodeID, payload.VMID)
		return fmt.Errorf("generating cloud-init for VM %s: %w", payload.VMID, err)
	}

	// Update task progress: Creating VM
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 50, "Creating virtual machine..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Create VM via node agent gRPC
	createReq := &CreateVMRequest{
		VMID:                payload.VMID,
		Hostname:            payload.Hostname,
		VCPU:                payload.VCPU,
		MemoryMB:            payload.MemoryMB,
		DiskGB:              payload.DiskGB,
		TemplateRBDImage:    template.RBDImage,
		TemplateRBDSnapshot: template.RBDSnapshot,
		RootPasswordHash:    passwordHash,
		SSHPublicKeys:       payload.SSHKeys,
		IPv4Address:         ipv4Addr,
		IPv4Gateway:         ipv4Gateway,
		IPv6Address:         ipv6Addr,
		IPv6Gateway:         ipv6Gateway,
		MACAddress:          macAddress,
		PortSpeedMbps:       vm.PortSpeedMbps,
		CephPool:            node.CephPool,
		CephUser:            "virtuestack", // Default Ceph user
		CephSecretUUID:      "",            // Would be configured per-cluster
		CephMonitors:        []string{},    // Would be configured per-cluster
		Nameservers:         cloudInitCfg.Nameservers,
	}

	createResp, err := deps.NodeClient.CreateVM(ctx, payload.NodeID, createReq)
	if err != nil {
		logger.Error("failed to create VM via node agent", "error", err)
		// Cleanup
		_ = deps.NodeClient.DeleteDisk(ctx, payload.NodeID, payload.VMID)
		_ = deps.IPAMService.ReleaseIPsByVM(ctx, payload.VMID)
		return fmt.Errorf("creating VM %s via node agent: %w", payload.VMID, err)
	}

	// Update task progress: Starting VM
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 80, "Starting virtual machine..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Start VM via node agent
	if err := deps.NodeClient.StartVM(ctx, payload.NodeID, payload.VMID); err != nil {
		logger.Error("failed to start VM", "error", err)
		return fmt.Errorf("starting VM %s: %w", payload.VMID, err)
	}

	// Update VM status to running
	if err := deps.VMRepo.UpdateStatus(ctx, payload.VMID, models.VMStatusRunning); err != nil {
		logger.Warn("failed to update VM status", "error", err)
	}

	// Update VM with template ID
	// Note: In production, you'd have an Update method for other fields

	// Update task progress: Complete
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "VM provisioned successfully"); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Set task result
	result := map[string]any{
		"vm_id":            payload.VMID,
		"domain_name":      createResp.DomainName,
		"vnc_port":         createResp.VNCPort,
		"ipv4_address":     ipv4Addr,
		"ipv6_subnet":      ipv6Addr,
		"cloud_init_path":  cloudInitPath,
	}
	resultJSON, _ := json.Marshal(result)
	if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
		logger.Warn("failed to set task completed", "error", err)
	}

	logger.Info("vm.create task completed successfully",
		"vm_id", payload.VMID,
		"domain_name", createResp.DomainName,
		"ipv4", ipv4Addr)

	return nil
}

// handleVMDelete handles the VM deletion flow.
// Steps:
//  1. Parse payload
//  2. Get VM record
//  3. Stop VM (if running)
//  4. Delete VM definition
//  5. Delete RBD volume
//  6. Release IP addresses
//  7. Soft delete VM record
func handleVMDelete(ctx context.Context, task *models.Task, deps *HandlerDeps) error {
	logger := deps.Logger.With("task_id", task.ID, "task_type", "vm.delete")

	// Parse payload
	var payload VMDeletePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse vm.delete payload", "error", err)
		return fmt.Errorf("parsing vm.delete payload: %w", err)
	}

	logger = logger.With("vm_id", payload.VMID)
	logger.Info("vm.delete task started")

	// Update task progress
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 5, "Starting VM deletion..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Get VM record
	vm, err := deps.VMRepo.GetByID(ctx, payload.VMID)
	if err != nil {
		logger.Error("failed to get VM record", "error", err)
		// If VM doesn't exist, consider deletion successful (idempotent)
		if err := deps.TaskRepo.SetCompleted(ctx, task.ID, []byte(`{"vm_id":"`+payload.VMID+`","status":"deleted"}`)); err != nil {
			logger.Warn("failed to set task completed", "error", err)
		}
		return nil
	}

	// Update task progress: Stopping VM
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 15, "Stopping virtual machine..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Delete VM definition and disk if node is assigned
	if vm.NodeID != nil {
		nodeID := *vm.NodeID

		// Stop VM if running
		if vm.Status == models.VMStatusRunning {
			if err := deps.NodeClient.StopVM(ctx, nodeID, payload.VMID, 60); err != nil {
				logger.Warn("failed to stop VM gracefully, forcing", "error", err)
				if err := deps.NodeClient.ForceStopVM(ctx, nodeID, payload.VMID); err != nil {
					logger.Warn("failed to force stop VM", "error", err)
				}
			}
		}

		// Update task progress: Deleting VM definition
		if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 30, "Deleting VM definition..."); err != nil {
			logger.Warn("failed to update task progress", "error", err)
		}

		// Delete VM definition from libvirt
		if err := deps.NodeClient.DeleteVM(ctx, nodeID, payload.VMID); err != nil {
			logger.Warn("failed to delete VM definition", "error", err)
			// Continue with disk deletion
		}

		// Update task progress: Deleting disk
		if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 50, "Deleting disk image..."); err != nil {
			logger.Warn("failed to update task progress", "error", err)
		}

		// Delete RBD disk
		if err := deps.NodeClient.DeleteDisk(ctx, nodeID, payload.VMID); err != nil {
			logger.Warn("failed to delete disk", "error", err)
			// Continue with IP release
		}
	}

	// Update task progress: Releasing IPs
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 70, "Releasing IP addresses..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Release IP addresses
	if err := deps.IPAMService.ReleaseIPsByVM(ctx, payload.VMID); err != nil {
		logger.Warn("failed to release IPs", "error", err)
		// Continue with VM record deletion
	}

	// Update task progress: Soft deleting record
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 90, "Removing VM record..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Soft delete VM record
	if err := deps.VMRepo.SoftDelete(ctx, payload.VMID); err != nil {
		logger.Error("failed to soft delete VM record", "error", err)
		return fmt.Errorf("soft deleting VM %s: %w", payload.VMID, err)
	}

	// Update task progress: Complete
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "VM deleted successfully"); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Set task result
	result := map[string]any{
		"vm_id":  payload.VMID,
		"status": "deleted",
	}
	resultJSON, _ := json.Marshal(result)
	if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
		logger.Warn("failed to set task completed", "error", err)
	}

	logger.Info("vm.delete task completed successfully", "vm_id", payload.VMID)

	return nil
}

// handleBackupRestore handles the backup restoration flow.
// Steps:
//  1. Parse payload
//  2. Get backup and VM records
//  3. Stop target VM
//  4. Delete current RBD volume
//  5. Clone from backup snapshot
//  6. Start VM
func handleBackupRestore(ctx context.Context, task *models.Task, deps *HandlerDeps) error {
	logger := deps.Logger.With("task_id", task.ID, "task_type", models.TaskTypeBackupRestore)

	// Parse payload
	var payload BackupRestorePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		logger.Error("failed to parse backup.restore payload", "error", err)
		return fmt.Errorf("parsing backup.restore payload: %w", err)
	}

	logger = logger.With("backup_id", payload.BackupID, "vm_id", payload.VMID)
	logger.Info("backup.restore task started")

	// Update task progress
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 5, "Starting backup restoration..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Get backup record
	backup, err := deps.BackupRepo.GetBackupByID(ctx, payload.BackupID)
	if err != nil {
		logger.Error("failed to get backup record", "error", err)
		return fmt.Errorf("getting backup %s: %w", payload.BackupID, err)
	}

	// Get VM record
	vm, err := deps.VMRepo.GetByID(ctx, payload.VMID)
	if err != nil {
		logger.Error("failed to get VM record", "error", err)
		return fmt.Errorf("getting VM %s: %w", payload.VMID, err)
	}

	if vm.NodeID == nil {
		return fmt.Errorf("VM %s has no node assigned", payload.VMID)
	}
	nodeID := *vm.NodeID

	// Update task progress: Stopping VM
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 15, "Stopping virtual machine..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Stop VM
	if vm.Status == models.VMStatusRunning {
		if err := deps.NodeClient.StopVM(ctx, nodeID, payload.VMID, 120); err != nil {
			logger.Warn("failed to stop VM gracefully, forcing", "error", err)
			if err := deps.NodeClient.ForceStopVM(ctx, nodeID, payload.VMID); err != nil {
				logger.Error("failed to force stop VM", "error", err)
				return fmt.Errorf("stopping VM %s: %w", payload.VMID, err)
			}
		}
	}

	// Update task progress: Deleting current disk
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 30, "Removing current disk..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Delete current disk
	if err := deps.NodeClient.DeleteDisk(ctx, nodeID, payload.VMID); err != nil {
		logger.Warn("failed to delete current disk", "error", err)
		// Continue anyway
	}

	// Update task progress: Cloning from backup
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 50, "Restoring from backup..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Clone from backup snapshot
	if backup.RBDSnapshot == nil {
		return fmt.Errorf("backup %s has no RBD snapshot", payload.BackupID)
	}

	if err := deps.NodeClient.CloneFromBackup(ctx, nodeID, payload.VMID, *backup.RBDSnapshot, vm.DiskGB); err != nil {
		logger.Error("failed to clone from backup", "error", err)
		return fmt.Errorf("cloning from backup %s: %w", payload.BackupID, err)
	}

	// Update task progress: Starting VM
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 80, "Starting virtual machine..."); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Start VM
	if err := deps.NodeClient.StartVM(ctx, nodeID, payload.VMID); err != nil {
		logger.Error("failed to start VM", "error", err)
		return fmt.Errorf("starting VM %s: %w", payload.VMID, err)
	}

	// Update VM status
	if err := deps.VMRepo.UpdateStatus(ctx, payload.VMID, models.VMStatusRunning); err != nil {
		logger.Warn("failed to update VM status", "error", err)
	}

	// Update backup status
	if err := deps.BackupRepo.UpdateBackupStatus(ctx, payload.BackupID, models.BackupStatusCompleted); err != nil {
		logger.Warn("failed to update backup status", "error", err)
	}

	// Update task progress: Complete
	if err := deps.TaskRepo.UpdateProgress(ctx, task.ID, 100, "Backup restored successfully"); err != nil {
		logger.Warn("failed to update task progress", "error", err)
	}

	// Set task result
	result := map[string]any{
		"backup_id": payload.BackupID,
		"vm_id":     payload.VMID,
		"status":    "restored",
	}
	resultJSON, _ := json.Marshal(result)
	if err := deps.TaskRepo.SetCompleted(ctx, task.ID, resultJSON); err != nil {
		logger.Warn("failed to set task completed", "error", err)
	}

	logger.Info("backup.restore task completed successfully",
		"backup_id", payload.BackupID,
		"vm_id", payload.VMID)

	return nil
}

// Helper functions

// generateMACAddress generates a MAC address from a VM ID.
// Uses a consistent algorithm to generate reproducible MAC addresses.
func generateMACAddress(vmID string) string {
	// Use a fixed prefix for VirtueStack VMs
	prefix := "52:54:00" // QEMU default prefix

	// Generate the last 3 octets from the VM ID hash
	// This is a simple deterministic approach
	hash := 0
	for _, c := range vmID {
		hash = hash*31 + int(c)
	}

	octet4 := (hash >> 16) & 0xFF
	octet5 := (hash >> 8) & 0xFF
	octet6 := hash & 0xFF

	return fmt.Sprintf("%s:%02x:%02x:%02x", prefix, octet4, octet5, octet6)
}

// hashPasswordParams holds the parameters for Argon2id password hashing.
// These parameters are tuned for security (memory=65536, iterations=3, parallelism=4).
var hashPasswordParams = &argon2id.Params{
	Memory:      65536, // 64MB
	Iterations:  3,
	Parallelism: 4,
	SaltLength:  16,
	KeyLength:   32,
}

// hashPassword creates a secure password hash using Argon2id.
// Returns an empty string if the password is empty or fails validation.
func hashPassword(password string) (string, error) {
	if password == "" {
		return "", nil
	}

	if err := validatePasswordStrength(password); err != nil {
		return "", err
	}

	hash, err := argon2id.CreateHash(password, hashPasswordParams)
	if err != nil {
		return "", fmt.Errorf("creating password hash: %w", err)
	}
	return hash, nil
}

// verifyPassword verifies a password against an Argon2id hash.
// Returns true if the password matches the hash.
func verifyPassword(password, hash string) (bool, error) {
	if password == "" || hash == "" {
		return false, fmt.Errorf("password and hash cannot be empty")
	}

	match, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return false, fmt.Errorf("comparing password: %w", err)
	}
	return match, nil
}

// validatePasswordStrength validates that a password meets minimum security requirements.
// Minimum 8 characters with at least one uppercase, one lowercase, one digit, and one special character.
func validatePasswordStrength(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters long")
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return fmt.Errorf("password must contain at least one uppercase letter")
	}
	if !hasLower {
		return fmt.Errorf("password must contain at least one lowercase letter")
	}
	if !hasDigit {
		return fmt.Errorf("password must contain at least one digit")
	}
	if !hasSpecial {
		return fmt.Errorf("password must contain at least one special character")
	}

	return nil
}