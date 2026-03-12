// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// MigrationService provides live migration operations for VirtueStack.
// It handles VM migration between hypervisor nodes, including pre-checks,
// target node selection, and coordination of the migration process.
type MigrationService struct {
	vmRepo        *repository.VMRepository
	nodeRepo      *repository.NodeRepository
	taskPublisher TaskPublisher
	logger        *slog.Logger
}

// NewMigrationService creates a new MigrationService with the given dependencies.
func NewMigrationService(
	vmRepo *repository.VMRepository,
	nodeRepo *repository.NodeRepository,
	taskPublisher TaskPublisher,
	logger *slog.Logger,
) *MigrationService {
	return &MigrationService{
		vmRepo:        vmRepo,
		nodeRepo:      nodeRepo,
		taskPublisher: taskPublisher,
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

	// 8. Update VM status to migrating before publishing task
	if err := s.vmRepo.UpdateStatus(ctx, vm.ID, models.VMStatusMigrating); err != nil {
		return nil, fmt.Errorf("updating VM status to migrating: %w", err)
	}

	// 9. Publish migration task
	taskPayload := map[string]any{
		"vm_id":           vm.ID,
		"source_node_id":  sourceNodeID,
		"target_node_id":  targetNode.ID,
		"hostname":        vm.Hostname,
		"vcpu":            vm.VCPU,
		"memory_mb":       vm.MemoryMB,
		"disk_gb":         vm.DiskGB,
		"mac_address":     vm.MACAddress,
		"live":            req.Live,
		"source_ceph_pool": sourceNode.CephPool,
		"target_ceph_pool": targetNode.CephPool,
		"initiated_by":    adminID,
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
		"admin_id", adminID)

	return &MigrateVMResult{
		TaskID:       taskID,
		SourceNodeID: sourceNodeID,
		TargetNodeID: targetNode.ID,
	}, nil
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
		Status: strPtr(models.NodeStatusOnline),
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
	// This would typically use the taskRepo, but we can use the task publisher's
	// underlying repository or inject a TaskRepository dependency
	// For now, we return an error indicating this needs to be implemented
	// with a proper task repository
	return nil, fmt.Errorf("GetMigrationStatus requires TaskRepository dependency")
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

	// 3. In a full implementation, this would:
	// - Cancel the migration task via NATS or task worker
	// - Wait for confirmation or timeout
	// - Revert VM status based on outcome

	// For now, just log the cancellation request
	s.logger.Warn("migration cancellation requested (not yet fully implemented)",
		"vm_id", vmID,
		"admin_id", adminID)

	return fmt.Errorf("migration cancellation not yet fully implemented")
}