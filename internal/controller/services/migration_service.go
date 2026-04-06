// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path"
	"path/filepath"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/tasks"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
)

// StorageBackendGetter is defined in vm_service.go.
// This interface is used for getting storage backends for a node.

// MigrationService provides live migration operations for VirtueStack.
// It handles VM migration between hypervisor nodes, including pre-checks,
// target node selection, and coordination of the migration process.
type MigrationService struct {
	vmRepo            *repository.VMRepository
	nodeRepo          *repository.NodeRepository
	taskRepo          *repository.TaskRepository
	taskPublisher     TaskPublisher
	nodeClient        NodeAgentClient
	storageBackendSvc StorageBackendGetter
	logger            *slog.Logger
}

// NewMigrationService creates a new MigrationService with the given dependencies.
func NewMigrationService(
	vmRepo *repository.VMRepository,
	nodeRepo *repository.NodeRepository,
	taskRepo *repository.TaskRepository,
	taskPublisher TaskPublisher,
	nodeClient NodeAgentClient,
	storageBackendSvc StorageBackendGetter,
	logger *slog.Logger,
) *MigrationService {
	return &MigrationService{
		vmRepo:            vmRepo,
		nodeRepo:          nodeRepo,
		taskRepo:          taskRepo,
		taskPublisher:     taskPublisher,
		nodeClient:        nodeClient,
		storageBackendSvc: storageBackendSvc,
		logger:            logger.With("component", "migration-service"),
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
	// 1. Validate VM state for migration
	vm, sourceNodeID, err := s.validateVMState(ctx, req.VMID, req.Live)
	if err != nil {
		return nil, err
	}

	// 2. Get the source node for validation
	sourceNode, err := s.nodeRepo.GetByID(ctx, sourceNodeID)
	if err != nil {
		return nil, fmt.Errorf("getting source node: %w", err)
	}

	// 3. Determine target node
	targetNode, err := s.resolveTargetNode(ctx, req, sourceNodeID, vm)
	if err != nil {
		return nil, err
	}

	// 4. Prepare and execute migration
	return s.executeMigration(ctx, vm, sourceNode, targetNode, req.Live, adminID)
}

// validateVMState retrieves a VM and validates it's in a state suitable for migration.
// Returns the VM, source node ID, and any validation error.
func (s *MigrationService) validateVMState(ctx context.Context, vmID string, live bool) (*models.VM, string, error) {
	vm, err := s.vmRepo.GetByID(ctx, vmID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, "", fmt.Errorf("VM not found: %s", vmID)
		}
		return nil, "", fmt.Errorf("getting VM: %w", err)
	}

	if vm.IsDeleted() {
		return nil, "", fmt.Errorf("VM has been deleted")
	}

	if vm.NodeID == nil {
		return nil, "", fmt.Errorf("VM has no source node assigned")
	}

	if err := s.validateVMStatusForMigration(vm.Status, live); err != nil {
		return nil, "", err
	}

	return vm, *vm.NodeID, nil
}

// validateVMStatusForMigration checks if a VM status allows migration.
func (s *MigrationService) validateVMStatusForMigration(status string, live bool) error {
	switch status {
	case models.VMStatusRunning:
		return nil
	case models.VMStatusStopped:
		if live {
			return fmt.Errorf("cannot perform live migration on a stopped VM")
		}
		return nil
	case models.VMStatusMigrating:
		return fmt.Errorf("VM is already migrating")
	case models.VMStatusProvisioning, models.VMStatusReinstalling:
		return fmt.Errorf("cannot migrate VM in status %s", status)
	case models.VMStatusSuspended:
		if live {
			return fmt.Errorf("cannot perform live migration on a suspended VM")
		}
		return nil
	default:
		return fmt.Errorf("VM status %s does not support migration", status)
	}
}

// resolveTargetNode determines the target node for migration.
func (s *MigrationService) resolveTargetNode(ctx context.Context, req *MigrateVMRequest, sourceNodeID string, vm *models.VM) (*models.Node, error) {
	var targetNode *models.Node
	var err error

	if req.TargetNodeID != nil {
		targetNode, err = s.validateTargetNode(ctx, *req.TargetNodeID, sourceNodeID, vm)
		if err != nil {
			return nil, err
		}
	} else {
		targetNode, err = s.findBestTargetNode(ctx, sourceNodeID, vm)
		if err != nil {
			return nil, fmt.Errorf("finding target node: %w", err)
		}
	}

	if err := s.checkNodeCapacity(targetNode, vm); err != nil {
		return nil, fmt.Errorf("target node capacity check failed: %w", err)
	}

	return targetNode, nil
}

// executeMigration prepares and publishes the migration task.
func (s *MigrationService) executeMigration(ctx context.Context, vm *models.VM, sourceNode, targetNode *models.Node, live bool, adminID string) (*MigrateVMResult, error) {
	vmStorageBackend := s.getNodeStorageBackend(vm)
	migrationStrategy := s.determineMigrationStrategy(vmStorageBackend, live)

	if err := s.validateStorageBackend(vmStorageBackend); err != nil {
		return nil, fmt.Errorf("storage backend validation failed: %w", err)
	}

	preMigrationStatus := vm.Status
	taskPayload, err := s.buildMigrationPayload(ctx, vm, sourceNode, targetNode, vmStorageBackend, migrationStrategy, live, adminID, preMigrationStatus)
	if err != nil {
		return nil, fmt.Errorf("building migration payload: %w", err)
	}

	s.logger.Info("migration strategy determined",
		"vm_id", vm.ID,
		"storage_backend", vmStorageBackend,
		"strategy", migrationStrategy,
		"source_disk", taskPayload["source_disk_path"],
		"target_disk", taskPayload["target_disk_path"])

	taskID, err := s.taskPublisher.PublishTask(ctx, models.TaskTypeVMMigrate, taskPayload)
	if err != nil {
		return nil, fmt.Errorf("publishing migration task: %w", err)
	}

	s.logger.Info("VM migration initiated", "vm_id", vm.ID, "task_id", taskID, "source_node_id", *vm.NodeID, "target_node_id", targetNode.ID, "strategy", migrationStrategy, "admin_id", adminID)

	return &MigrateVMResult{TaskID: taskID, SourceNodeID: *vm.NodeID, TargetNodeID: targetNode.ID}, nil
}

// buildMigrationPayload constructs the task payload for a migration operation.
func (s *MigrationService) buildMigrationPayload(ctx context.Context, vm *models.VM, sourceNode, targetNode *models.Node, storageBackend string, strategy tasks.MigrationStrategy, live bool, _, preMigrationStatus string) (map[string]any, error) {
	payload := tasks.VMMigratePayload{
		VMID:                 vm.ID,
		SourceNodeID:         *vm.NodeID,
		TargetNodeID:         targetNode.ID,
		PreMigrationState:    preMigrationStatus,
		SourceStorageBackend: storageBackend,
		TargetStorageBackend: storageBackend,
		SourceStoragePath:    sourceNode.StoragePath,
		TargetStoragePath:    targetNode.StoragePath,
		SourceCephPool:       sourceNode.CephPool,
		TargetCephPool:       targetNode.CephPool,
		MigrationStrategy:    strategy,
		Live:                 live,
		DiskSizeGB:           vm.DiskGB,
	}

	sourceBackend, err := s.resolveMigrationBackend(ctx, sourceNode.ID, storageBackend, vm.StorageBackendID)
	if err != nil {
		return nil, fmt.Errorf("resolving source storage backend: %w", err)
	}
	if sourceBackend != nil {
		applyStorageBackendMetadata(&payload, sourceBackend, true)
	}

	targetPreferredBackendID := targetMigrationBackendID(storageBackend, vm.StorageBackendID)
	targetBackend, err := s.resolveMigrationBackend(ctx, targetNode.ID, storageBackend, targetPreferredBackendID)
	if err != nil {
		return nil, fmt.Errorf("resolving target storage backend: %w", err)
	}
	if targetBackend != nil {
		applyStorageBackendMetadata(&payload, targetBackend, false)
		payload.TargetStorageBackendID = targetBackend.ID
	}
	payload.SourceDiskPath = s.getVMDiskPath(vm, sourceNode, sourceBackend)
	payload.TargetDiskPath = s.getVMDiskPathForTarget(vm, targetNode, targetBackend)

	return map[string]any{
		"vm_id":                     payload.VMID,
		"source_node_id":            payload.SourceNodeID,
		"target_node_id":            payload.TargetNodeID,
		"pre_migration_state":       payload.PreMigrationState,
		"source_storage_backend":    payload.SourceStorageBackend,
		"target_storage_backend":    payload.TargetStorageBackend,
		"target_storage_backend_id": payload.TargetStorageBackendID,
		"source_storage_path":       payload.SourceStoragePath,
		"target_storage_path":       payload.TargetStoragePath,
		"source_ceph_pool":          payload.SourceCephPool,
		"target_ceph_pool":          payload.TargetCephPool,
		"source_lvm_volume_group":   payload.SourceLVMVolumeGroup,
		"source_lvm_thin_pool":      payload.SourceLVMThinPool,
		"target_lvm_volume_group":   payload.TargetLVMVolumeGroup,
		"target_lvm_thin_pool":      payload.TargetLVMThinPool,
		"migration_strategy":        string(payload.MigrationStrategy),
		"live":                      payload.Live,
		"source_disk_path":          payload.SourceDiskPath,
		"target_disk_path":          payload.TargetDiskPath,
		"disk_size_gb":              payload.DiskSizeGB,
	}, nil
}

func (s *MigrationService) resolveMigrationBackend(ctx context.Context, nodeID, backendType string, preferredBackendID *string) (*models.StorageBackend, error) {
	if s.storageBackendSvc == nil {
		return nil, nil
	}

	backends, err := s.storageBackendSvc.GetBackendsForNodeByType(ctx, nodeID, backendType)
	if err != nil {
		return nil, fmt.Errorf("getting storage backends for node %s: %w", nodeID, err)
	}
	if len(backends) == 0 {
		return nil, nil
	}

	if preferredBackendID != nil {
		for i := range backends {
			if backends[i].ID == *preferredBackendID {
				return &backends[i], nil
			}
		}
		return nil, fmt.Errorf("storage backend %s is not assigned to node %s", *preferredBackendID, nodeID)
	}

	if len(backends) > 1 {
		return nil, fmt.Errorf("node %s has multiple %s storage backends assigned; migration target selection is ambiguous", nodeID, backendType)
	}

	return &backends[0], nil
}

func applyStorageBackendMetadata(payload *tasks.VMMigratePayload, backend *models.StorageBackend, isSource bool) {
	if backend == nil {
		return
	}

	switch string(backend.Type) {
	case models.StorageBackendQcow:
		if backend.StoragePath != nil {
			if isSource {
				payload.SourceStoragePath = *backend.StoragePath
			} else {
				payload.TargetStoragePath = *backend.StoragePath
			}
		}
	case models.StorageBackendLvm:
		if backend.LVMVolumeGroup != nil {
			if isSource {
				payload.SourceLVMVolumeGroup = *backend.LVMVolumeGroup
			} else {
				payload.TargetLVMVolumeGroup = *backend.LVMVolumeGroup
			}
		}
		if backend.LVMThinPool != nil {
			if isSource {
				payload.SourceLVMThinPool = *backend.LVMThinPool
			} else {
				payload.TargetLVMThinPool = *backend.LVMThinPool
			}
		}
	case models.StorageBackendCeph:
		if backend.CephPool != nil {
			if isSource {
				payload.SourceCephPool = *backend.CephPool
			} else {
				payload.TargetCephPool = *backend.CephPool
			}
		}
	}
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

	// QCOW and LVM: need disk copy between nodes
	return tasks.MigrationStrategyDiskCopy
}

// validateStorageBackend validates that the VM's storage backend is supported for migration.
// The VM's storage backend is immutable — cross-backend migration is not allowed.
func (s *MigrationService) validateStorageBackend(storageBackend string) error {
	switch storageBackend {
	case models.StorageBackendCeph, models.StorageBackendQcow, models.StorageBackendLvm:
		return nil
	default:
		return fmt.Errorf("unknown storage backend %q: VM must be migrated from a node with the same backend", storageBackend)
	}
}

// getVMDiskPath returns the disk path for a VM on its current node.
func (s *MigrationService) getVMDiskPath(vm *models.VM, node *models.Node, backend *models.StorageBackend) string {
	storageBackend := s.getNodeStorageBackend(vm)
	if storageBackend == models.StorageBackendQcow {
		if vm.DiskPath != nil && *vm.DiskPath != "" {
			return *vm.DiskPath
		}
		return qcowDiskIdentifier(qcowBasePath(node, backend), vm.ID)
	}
	if storageBackend == models.StorageBackendLvm {
		return lvmDiskIdentifier(vm.ID, lvmVolumeGroup(backend))
	}

	if vm.RBDImage != nil && *vm.RBDImage != "" {
		return *vm.RBDImage
	}
	return fmt.Sprintf("vm-%s-disk0", vm.ID)
}

// getVMDiskPathForTarget returns the disk path for a VM on the target node.
func (s *MigrationService) getVMDiskPathForTarget(vm *models.VM, targetNode *models.Node, backend *models.StorageBackend) string {
	storageBackend := s.getNodeStorageBackend(vm)

	if storageBackend == models.StorageBackendQcow {
		return qcowDiskIdentifier(qcowBasePath(targetNode, backend), vm.ID)
	}
	if storageBackend == models.StorageBackendLvm {
		return lvmDiskIdentifier(vm.ID, lvmVolumeGroup(backend))
	}

	if vm.RBDImage != nil && *vm.RBDImage != "" {
		return *vm.RBDImage
	}
	return fmt.Sprintf("vm-%s-disk0", vm.ID)
}

func qcowBasePath(node *models.Node, backend *models.StorageBackend) string {
	if backend != nil && backend.StoragePath != nil && *backend.StoragePath != "" {
		return *backend.StoragePath
	}
	if node != nil && node.StoragePath != "" {
		return node.StoragePath
	}
	return "/var/lib/virtuestack/vms"
}

func qcowDiskIdentifier(basePath, vmID string) string {
	return filepath.Join(basePath, fmt.Sprintf("%s-disk0.qcow2", vmID))
}

func lvmVolumeGroup(backend *models.StorageBackend) string {
	if backend != nil && backend.LVMVolumeGroup != nil && *backend.LVMVolumeGroup != "" {
		return *backend.LVMVolumeGroup
	}
	return "vgvs"
}

func lvmDiskIdentifier(vmID, volumeGroup string) string {
	return path.Join("/dev", volumeGroup, fmt.Sprintf("vs-%s-disk0", vmID))
}

func targetMigrationBackendID(storageBackend string, vmStorageBackendID *string) *string {
	if storageBackend == models.StorageBackendCeph {
		return vmStorageBackendID
	}
	return nil
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

	if err := s.validateTargetNodeStorageCompatibility(ctx, targetNode, vm); err != nil {
		return nil, err
	}

	return targetNode, nil
}

// findBestTargetNode finds the optimal target node for migration.
// It selects the node with the most available capacity that is online
// and in the same location as the source node (if applicable).
func (s *MigrationService) findBestTargetNode(ctx context.Context, sourceNodeID string, vm *models.VM) (*models.Node, error) {
	sourceNode, err := s.nodeRepo.GetByID(ctx, sourceNodeID)
	if err != nil {
		return nil, fmt.Errorf("getting source node: %w", err)
	}

	nodes, _, _, err := s.nodeRepo.List(ctx, models.NodeListFilter{
		Status:           util.StringPtr(models.NodeStatusOnline),
		PaginationParams: models.PaginationParams{PerPage: models.MaxPerPage},
	})
	if err != nil {
		return nil, fmt.Errorf("listing online nodes: %w", err)
	}

	candidates := s.filterCandidateNodes(ctx, nodes, sourceNodeID, vm)

	// Provide clear error message based on storage backend type
	if len(candidates) == 0 {
		vmStorageBackend := s.getNodeStorageBackend(vm)
		if vmStorageBackend != models.StorageBackendCeph {
			return nil, fmt.Errorf("VMs with %s storage backend cannot be migrated between nodes (disk is local to source node); only Ceph VMs support cross-node migration", vmStorageBackend)
		}
		return nil, fmt.Errorf("no suitable target nodes available with sufficient capacity (need %d vCPU, %d MB RAM)", vm.VCPU, vm.MemoryMB)
	}

	return s.selectBestCandidate(candidates, sourceNode), nil
}

// filterCandidateNodes filters nodes with sufficient capacity for the VM.
// It also ensures storage backend compatibility - Ceph VMs can only migrate to
// nodes with access to the same Ceph cluster; QCOW/LVM VMs cannot migrate
// (disk is local to the source node).
func (s *MigrationService) filterCandidateNodes(
	ctx context.Context,
	nodes []models.Node,
	sourceNodeID string,
	vm *models.VM,
) []models.Node {
	var candidates []models.Node
	vmStorageBackend := s.getNodeStorageBackend(vm)

	for i := range nodes {
		node := &nodes[i]
		if node.ID == sourceNodeID {
			continue
		}

		if err := s.validateTargetNodeStorageCompatibility(ctx, node, vm); err != nil {
			if vmStorageBackend == models.StorageBackendCeph && s.logger != nil {
				s.logger.Debug("skipping incompatible target node during migration",
					"node_id", node.ID,
					"error", err)
			}
			continue
		}

		// Check CPU and memory capacity
		availableVCPU := node.TotalVCPU - node.AllocatedVCPU
		availableMemory := node.TotalMemoryMB - node.AllocatedMemoryMB
		if availableVCPU >= vm.VCPU && availableMemory >= vm.MemoryMB {
			candidates = append(candidates, *node)
		}
	}
	return candidates
}

func (s *MigrationService) validateTargetNodeStorageCompatibility(ctx context.Context, targetNode *models.Node, vm *models.VM) error {
	vmStorageBackend := s.getNodeStorageBackend(vm)
	if vmStorageBackend != models.StorageBackendCeph {
		return fmt.Errorf("VMs with %s storage backend cannot be migrated between nodes (disk is local to source node); only Ceph VMs support cross-node migration", vmStorageBackend)
	}

	if s.storageBackendSvc != nil && vm.StorageBackendID != nil {
		backends, err := s.storageBackendSvc.GetBackendsForNodeByType(ctx, targetNode.ID, models.StorageBackendCeph)
		if err != nil {
			return fmt.Errorf("checking storage backends for target node %s: %w", targetNode.ID, err)
		}

		for i := range backends {
			if backends[i].ID == *vm.StorageBackendID {
				return nil
			}
		}

		return fmt.Errorf("target node %s does not have access to the same storage backend as VM %s", targetNode.ID, vm.ID)
	}

	targetNodeStorageBackend := targetNode.StorageBackend
	if targetNodeStorageBackend == "" {
		targetNodeStorageBackend = models.StorageBackendCeph
	}
	if targetNodeStorageBackend != models.StorageBackendCeph {
		return fmt.Errorf("target node %s does not support Ceph storage for VM %s", targetNode.ID, vm.ID)
	}

	return nil
}

// selectBestCandidate selects the best node from candidates based on scoring.
func (s *MigrationService) selectBestCandidate(candidates []models.Node, sourceNode *models.Node) *models.Node {
	var bestNode *models.Node
	bestScore := -1

	for i := range candidates {
		node := &candidates[i]
		score := s.scoreNode(node, sourceNode)
		if score > bestScore {
			bestScore = score
			bestNode = node
		}
	}
	return bestNode
}

// scoreNode calculates a score for a node based on capacity and location.
func (s *MigrationService) scoreNode(node, sourceNode *models.Node) int {
	score := 0
	if sourceNode.LocationID != nil && node.LocationID != nil && *sourceNode.LocationID == *node.LocationID {
		score += 1000
	}
	availableMemory := node.TotalMemoryMB - node.AllocatedMemoryMB
	score += (availableMemory * 999) / node.TotalMemoryMB
	return score
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
			PerPage: 20,
		},
	}
	if tasks, _, _, listErr := s.taskRepo.List(ctx, filter); listErr == nil {
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
