// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/tasks"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
)

// MigrationService provides live migration operations for VirtueStack.
// It handles VM migration between hypervisor nodes, including pre-checks,
// target node selection, and coordination of the migration process.
type MigrationService struct {
	vmRepo        *repository.VMRepository
	nodeRepo      *repository.NodeRepository
	taskRepo      *repository.TaskRepository
	taskPublisher TaskPublisher
	nodeClient    NodeAgentClient
	logger        *slog.Logger
}

// NewMigrationService creates a new MigrationService with the given dependencies.
func NewMigrationService(
	vmRepo *repository.VMRepository,
	nodeRepo *repository.NodeRepository,
	taskRepo *repository.TaskRepository,
	taskPublisher TaskPublisher,
	nodeClient NodeAgentClient,
	logger *slog.Logger,
) *MigrationService {
	return &MigrationService{
		vmRepo:        vmRepo,
		nodeRepo:      nodeRepo,
		taskRepo:      taskRepo,
		taskPublisher: taskPublisher,
		nodeClient:    nodeClient,
		logger:        logger.With("component", "migration-service"),
	}
}

// MigrateVMRequest contains the parameters for a VM migration operation.
type MigrateVMRequest struct {
	// VMID is the unique identifier of the VM to migrate.
	VMID string `json:"vm_id" validate:"required,uuid"`
	// TargetNodeID is the optional target node for migration.
	// If not specified, the best available node will be selected automatically.
	TargetNodeID *string `json:"target_node_id,omitempty" validate:"omitempty,uuid"`
	// Live indicates whether to perform a live migration (without stopping the VM).
	// If false, the VM will be paused during migration.
	Live bool `json:"live"`
}

// MigrateVMResult contains the result of a migration initiation.
type MigrateVMResult struct {
	// TaskID is the ID of the async migration task for status polling.
	TaskID string `json:"task_id"`
	// SourceNodeID is the ID of the node the VM is migrating from.
	SourceNodeID string `json:"source_node_id"`
	// TargetNodeID is the ID of the node the VM is migrating to.
	TargetNodeID string `json:"target_node_id"`
}

// MigrateVM initiates a live migration of a VM to another node.
// It performs pre-flight checks, selects an optimal target node if not specified,
// publishes a migration task, and updates the VM status.
// This is an async operation - use the returned task ID to poll for status.
func (s *MigrationService) MigrateVM(ctx context.Context, req *MigrateVMRequest, adminID string) (*MigrateVMResult, error) {
	// 1. Retrieve and validate the VM
	vm, err := s.vmRepo.GetByID(ctx, req.VMID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("VM not found: %s", req.VMID)
		}
		return nil, fmt.Errorf("getting VM: %w", err)
	}

	// 2. Check if VM is deleted
	if vm.IsDeleted() {
		return nil, fmt.Errorf("VM has been deleted")
	}

	// 3. Verify VM has a node assigned
	if vm.NodeID == nil {
		return nil, fmt.Errorf("VM has no source node assigned")
	}
	sourceNodeID := *vm.NodeID

	// 4. Verify VM is in a state that allows migration
	// For live migration, VM must be running
	// For non-live migration, VM must be running or stopped
	switch vm.Status {
	case models.VMStatusRunning:
		// Running VMs can be migrated (live or paused)
	case models.VMStatusStopped:
		// Stopped VMs can be migrated (offline migration)
		if req.Live {
			return nil, fmt.Errorf("cannot perform live migration on a stopped VM")
		}
	case models.VMStatusMigrating:
		return nil, fmt.Errorf("VM is already migrating")
	case models.VMStatusProvisioning, models.VMStatusReinstalling:
		return nil, fmt.Errorf("cannot migrate VM in status %s", vm.Status)
	case models.VMStatusSuspended:
		// Suspended VMs can be migrated as offline migration
		if req.Live {
			return nil, fmt.Errorf("cannot perform live migration on a suspended VM")
		}
	default:
		return nil, fmt.Errorf("VM status %s does not support migration", vm.Status)
	}

	// 5. Get the source node for validation
	sourceNode, err := s.nodeRepo.GetByID(ctx, sourceNodeID)
	if err != nil {
		return nil, fmt.Errorf("getting source node: %w", err)
	}

	// 6. Determine target node
	var targetNode *models.Node
	if req.TargetNodeID != nil {
		// Validate explicit target node
		targetNode, err = s.validateTargetNode(ctx, *req.TargetNodeID, sourceNodeID, vm)
		if err != nil {
			return nil, err
		}
	} else {
		// Find best available target node
		targetNode, err = s.findBestTargetNode(ctx, sourceNodeID, vm)
		if err != nil {
			return nil, fmt.Errorf("finding target node: %w", err)
		}
	}

	// 7. Check target node has sufficient resources
	if err := s.checkNodeCapacity(targetNode, vm); err != nil {
		return nil, fmt.Errorf("target node capacity check failed: %w", err)
	}

	// 8. Determine VM's storage backend (immutable, set at creation time)
	vmStorageBackend := s.getNodeStorageBackend(vm)

	// 9. Determine migration strategy based on VM's storage backend
	migrationStrategy := s.determineMigrationStrategy(vmStorageBackend, req.Live)

	// 10. Validate storage backend compatibility
	// A VM's storage backend is immutable — it cannot be migrated to a different backend
	if err := s.validateStorageBackend(vmStorageBackend); err != nil {
		return nil, fmt.Errorf("storage backend validation failed: %w", err)
	}

	// 10. Get storage paths for disk copy operations
	sourceDiskPath := s.getVMDiskPath(vm, sourceNode)
	targetDiskPath := s.getVMDiskPathForTarget(vm, targetNode)

	s.logger.Info("migration strategy determined",
		"vm_id", vm.ID,
		"storage_backend", vmStorageBackend,
		"strategy", migrationStrategy,
		"source_disk", sourceDiskPath,
		"target_disk", targetDiskPath)

	// 11. Update VM status to migrating before publishing task
	if err := s.vmRepo.UpdateStatus(ctx, vm.ID, models.VMStatusMigrating); err != nil {
		return nil, fmt.Errorf("updating VM status to migrating: %w", err)
	}

	// 12. Publish migration task with storage-aware payload
	taskPayload := map[string]any{
		"vm_id":               vm.ID,
		"source_node_id":      sourceNodeID,
		"target_node_id":      targetNode.ID,
		"hostname":            vm.Hostname,
		"vcpu":                vm.VCPU,
		"memory_mb":           vm.MemoryMB,
		"disk_gb":             vm.DiskGB,
		"mac_address":         vm.MACAddress,
		"live":                req.Live,
		"source_ceph_pool":    sourceNode.CephPool,
		"target_ceph_pool":    targetNode.CephPool,
		"initiated_by":        adminID,
		"pre_migration_state": vm.Status,
		"storage_backend":     vmStorageBackend,
		"source_storage_path": sourceNode.StoragePath,
		"target_storage_path": targetNode.StoragePath,
		"migration_strategy":  string(migrationStrategy),
		"source_disk_path":    sourceDiskPath,
		"target_disk_path":    targetDiskPath,
		"disk_size_gb":        vm.DiskGB,
	}

	taskID, err := s.taskPublisher.PublishTask(ctx, models.TaskTypeVMMigrate, taskPayload)
	if err != nil {
		// Revert status on failure
		_ = s.vmRepo.UpdateStatus(ctx, vm.ID, vm.Status)
		return nil, fmt.Errorf("publishing migration task: %w", err)
	}

	s.logger.Info("VM migration initiated",
		"vm_id", vm.ID,
		"task_id", taskID,
		"source_node_id", sourceNodeID,
		"target_node_id", targetNode.ID,
		"live", req.Live,
		"strategy", migrationStrategy,
		"admin_id", adminID)

	return &MigrateVMResult{
		TaskID:       taskID,
		SourceNodeID: sourceNodeID,
		TargetNodeID: targetNode.ID,
	}, nil
}

// getNodeStorageBackend returns the storage backend for a VM.
// The VM's storage_backend is immutable once set at creation time.
func (s *MigrationService) getNodeStorageBackend(vm *models.VM) string {
	if vm.StorageBackend != "" {
		return vm.StorageBackend
	}
	return models.StorageBackendCeph
}

// determineMigrationStrategy determines the migration strategy based on the VM's storage backend.
func (s *MigrationService) determineMigrationStrategy(storageBackend string, live bool) tasks.MigrationStrategy {
	if storageBackend == models.StorageBackendCeph {
		// Ceph: shared storage, can do live migration
		return tasks.MigrationStrategyLiveSharedStorage
	}

	// QCOW: need disk copy between nodes
	return tasks.MigrationStrategyDiskCopy
}

// validateStorageBackend validates that the VM's storage backend is supported for migration.
// The VM's storage backend is immutable — cross-backend migration is not allowed.
func (s *MigrationService) validateStorageBackend(storageBackend string) error {
	switch storageBackend {
	case models.StorageBackendCeph, models.StorageBackendQcow:
		return nil
	default:
		return fmt.Errorf("unknown storage backend %q: VM must be migrated from a node with the same backend", storageBackend)
	}
}

// getVMDiskPath returns the disk path for a VM on its current node.
func (s *MigrationService) getVMDiskPath(vm *models.VM, node *models.Node) string {
	if vm.DiskPath != "" {
		return vm.DiskPath
	}

	storageBackend := s.getNodeStorageBackend(vm)
	if storageBackend == models.StorageBackendQcow {
		if node.StoragePath != "" {
			return fmt.Sprintf("%s/%s-disk0.qcow2", node.StoragePath, vm.ID)
		}
		return fmt.Sprintf("/var/lib/virtuestack/vms/%s-disk0.qcow2", vm.ID)
	}

	if vm.RBDImage != "" {
		return vm.RBDImage
	}
	return fmt.Sprintf("vm-%s-disk0", vm.ID)
}

// getVMDiskPathForTarget returns the disk path for a VM on the target node.
func (s *MigrationService) getVMDiskPathForTarget(vm *models.VM, targetNode *models.Node) string {
	storageBackend := s.getNodeStorageBackend(vm)

	if storageBackend == models.StorageBackendQcow {
		if targetNode.StoragePath != "" {
			return fmt.Sprintf("%s/%s-disk0.qcow2", targetNode.StoragePath, vm.ID)
		}
		return fmt.Sprintf("/var/lib/virtuestack/vms/%s-disk0.qcow2", vm.ID)
	}

	if vm.RBDImage != "" {
		return vm.RBDImage
	}
	return fmt.Sprintf("vm-%s-disk0", vm.ID)
}

// validateTargetNode validates that a specified target node is suitable for migration.
func (s *MigrationService) validateTargetNode(ctx context.Context, targetNodeID, sourceNodeID string, vm *models.VM) (*models.Node, error) {
	// Cannot migrate to the same node
	if targetNodeID == sourceNodeID {
		return nil, fmt.Errorf("target node cannot be the same as source node")
	}

	// Get the target node
	targetNode, err := s.nodeRepo.GetByID(ctx, targetNodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("target node not found: %s", targetNodeID)
		}
		return nil, fmt.Errorf("getting target node: %w", err)
	}

	// Check target node is online and accepting placements
	if targetNode.Status != models.NodeStatusOnline {
		return nil, fmt.Errorf("target node %s is not online (status: %s)", targetNodeID, targetNode.Status)
	}

	return targetNode, nil
}

// findBestTargetNode finds the optimal target node for migration.
// It selects the node with the most available capacity that is online
// and in the same location as the source node (if applicable).
func (s *MigrationService) findBestTargetNode(ctx context.Context, sourceNodeID string, vm *models.VM) (*models.Node, error) {
	// Get source node to determine location
	sourceNode, err := s.nodeRepo.GetByID(ctx, sourceNodeID)
	if err != nil {
		return nil, fmt.Errorf("getting source node: %w", err)
	}

	// Get all online nodes
	filter := models.NodeListFilter{
		Status: util.StringPtr(models.NodeStatusOnline),
		PaginationParams: models.PaginationParams{
			Page:    1,
			PerPage: models.MaxPerPage,
		},
	}

	nodes, _, err := s.nodeRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("listing online nodes: %w", err)
	}

	// Filter out the source node and nodes without capacity
	var candidates []models.Node
	for i := range nodes {
		node := &nodes[i]

		// Skip source node
		if node.ID == sourceNodeID {
			continue
		}

		// Prefer nodes in the same location for reduced network latency
		// But don't require it - cross-location migration is possible
		// Check if node has sufficient capacity
		availableVCPU := node.TotalVCPU - node.AllocatedVCPU
		availableMemory := node.TotalMemoryMB - node.AllocatedMemoryMB

		if availableVCPU >= vm.VCPU && availableMemory >= vm.MemoryMB {
			candidates = append(candidates, *node)
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no suitable target nodes available with sufficient capacity (need %d vCPU, %d MB RAM)",
			vm.VCPU, vm.MemoryMB)
	}

	// Score candidates: prefer same location, then most available capacity
	type scoredNode struct {
		node  models.Node
		score int
	}

	scored := make([]scoredNode, 0, len(candidates))
	for _, node := range candidates {
		score := 0

		// Bonus for same location
		if sourceNode.LocationID != nil && node.LocationID != nil && *sourceNode.LocationID == *node.LocationID {
			score += 1000
		}

		// Score by available memory (normalized to 0-999 range)
		availableMemory := node.TotalMemoryMB - node.AllocatedMemoryMB
		score += (availableMemory * 999) / node.TotalMemoryMB

		// Prefer nodes with fewer VMs for better load distribution
		scored = append(scored, scoredNode{node: node, score: score})
	}

	// Find best candidate
	best := scored[0]
	for i := 1; i < len(scored); i++ {
		if scored[i].score > best.score {
			best = scored[i]
		}
	}

	return &best.node, nil
}

// checkNodeCapacity verifies that a node has sufficient resources for a VM.
func (s *MigrationService) checkNodeCapacity(node *models.Node, vm *models.VM) error {
	availableVCPU := node.TotalVCPU - node.AllocatedVCPU
	if availableVCPU < vm.VCPU {
		return fmt.Errorf("insufficient vCPU on target node (available: %d, required: %d)",
			availableVCPU, vm.VCPU)
	}

	availableMemory := node.TotalMemoryMB - node.AllocatedMemoryMB
	if availableMemory < vm.MemoryMB {
		return fmt.Errorf("insufficient memory on target node (available: %d MB, required: %d MB)",
			availableMemory, vm.MemoryMB)
	}

	return nil
}

// GetMigrationStatus retrieves the status of a migration task.
func (s *MigrationService) GetMigrationStatus(ctx context.Context, taskID string) (*models.Task, error) {
	task, err := s.taskRepo.GetByID(ctx, taskID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("migration task not found: %s", taskID)
		}
		return nil, fmt.Errorf("getting migration task: %w", err)
	}
	return task, nil
}

// CancelMigration attempts to cancel an in-progress migration.
// This is a best-effort operation and may not succeed if migration is too far along.
func (s *MigrationService) CancelMigration(ctx context.Context, vmID, adminID string) error {
	// 1. Get the VM
	vm, err := s.vmRepo.GetByID(ctx, vmID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("VM not found: %s", vmID)
		}
		return fmt.Errorf("getting VM: %w", err)
	}

	// 2. Verify VM is migrating
	if vm.Status != models.VMStatusMigrating {
		return fmt.Errorf("VM is not in migrating status (current: %s)", vm.Status)
	}

	// 3. If migration is in progress, call AbortMigration via gRPC
	if vm.NodeID != nil && s.nodeClient != nil {
		if err := s.nodeClient.AbortMigration(ctx, *vm.NodeID, vmID); err != nil {
			s.logger.Warn("failed to abort migration via node client",
				"vm_id", vmID,
				"node_id", *vm.NodeID,
				"error", err)
			// Continue anyway to revert status
		}
	}

	restoreStatus := models.VMStatusRunning
	filter := repository.TaskListFilter{
		Type: util.StringPtr(models.TaskTypeVMMigrate),
		PaginationParams: models.PaginationParams{
			Page:    1,
			PerPage: 20,
		},
	}
	if tasks, _, listErr := s.taskRepo.List(ctx, filter); listErr == nil {
		for _, task := range tasks {
			var payload struct {
				VMID              string `json:"vm_id"`
				PreMigrationState string `json:"pre_migration_state"`
			}
			if err := json.Unmarshal(task.Payload, &payload); err != nil {
				continue
			}
			if payload.VMID != vmID {
				continue
			}
			if payload.PreMigrationState != "" {
				restoreStatus = payload.PreMigrationState
			}
			break
		}
	}

	if err := s.vmRepo.UpdateStatus(ctx, vmID, restoreStatus); err != nil {
		return fmt.Errorf("reverting VM status: %w", err)
	}

	s.logger.Info("migration cancelled",
		"vm_id", vmID,
		"admin_id", adminID)

	return nil
}
