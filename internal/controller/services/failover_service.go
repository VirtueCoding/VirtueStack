// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
)

// FailoverService provides high-availability failover operations for VirtueStack.
// It handles the complete failover workflow including STONITH, Ceph blocklisting,
// and VM migration to surviving nodes.
type FailoverService struct {
	nodeRepo      *repository.NodeRepository
	vmRepo        *repository.VMRepository
	nodeAgent     NodeAgentClient
	auditRepo     *repository.AuditRepository
	failoverRepo  *repository.FailoverRepository
	encryptionKey string
	logger        *slog.Logger
}

// NewFailoverService creates a new FailoverService with the given dependencies.
func NewFailoverService(
	nodeRepo *repository.NodeRepository,
	vmRepo *repository.VMRepository,
	nodeAgent NodeAgentClient,
	auditRepo *repository.AuditRepository,
	failoverRepo *repository.FailoverRepository,
	encryptionKey string,
	logger *slog.Logger,
) *FailoverService {
	return &FailoverService{
		nodeRepo:      nodeRepo,
		vmRepo:        vmRepo,
		nodeAgent:     nodeAgent,
		auditRepo:     auditRepo,
		failoverRepo:  failoverRepo,
		encryptionKey: encryptionKey,
		logger:        logger.With("component", "failover-service"),
	}
}

// FailoverResult represents the outcome of a failover operation.
type FailoverResult struct {
	NodeID           string            `json:"node_id"`
	NodeHostname     string            `json:"node_hostname"`
	TotalVMs         int               `json:"total_vms"`
	MigratedVMs      []MigratedVM      `json:"migrated_vms"`
	FailedMigrations []FailedMigration `json:"failed_migrations,omitempty"`
	STONITHExecuted  bool              `json:"stonith_executed"`
	BlocklistAdded   bool              `json:"blocklist_added"`
}

// MigratedVM represents a VM that was successfully migrated during failover.
type MigratedVM struct {
	VMID        string `json:"vm_id"`
	Hostname    string `json:"hostname"`
	OldNodeID   string `json:"old_node_id"`
	NewNodeID   string `json:"new_node_id"`
	NewNodeName string `json:"new_node_name"`
}

// FailedMigration represents a VM that failed to migrate during failover.
type FailedMigration struct {
	VMID     string `json:"vm_id"`
	Hostname string `json:"hostname"`
	Error    string `json:"error"`
}

// ApproveFailover executes the failover workflow for a failed node.
// This is a destructive operation that should only be called after admin approval.
//
// The workflow:
// 1. Verify the node is in a failed state (consecutive_heartbeat_misses >= 3)
// 2. If IPMI credentials exist, execute STONITH (power off)
// 3. Blocklist the failed node's management IP in Ceph
// 4. Release RBD locks for VMs on the failed node
// 5. Find surviving nodes
// 6. Migrate each VM to a suitable surviving node
func (s *FailoverService) ApproveFailover(ctx context.Context, adminID, targetNodeID string) (*FailoverResult, error) {
	s.logger.Info("failover initiated",
		"admin_id", adminID,
		"target_node_id", targetNodeID)

	node, err := s.verifyFailedNode(ctx, targetNodeID)
	if err != nil {
		return nil, err
	}

	result := &FailoverResult{
		NodeID:       targetNodeID,
		NodeHostname: node.Hostname,
	}

	var failoverReq *models.FailoverRequest
	if s.failoverRepo != nil {
		failoverReq = &models.FailoverRequest{
			NodeID:      targetNodeID,
			RequestedBy: adminID,
			Status:      models.FailoverStatusApproved,
		}
		if err := s.failoverRepo.Create(ctx, failoverReq); err != nil {
			s.logger.Warn("failed to create failover request record", "error", err)
		}
	}

	finalizeRequest := func(status string, resultData any) {
		if s.failoverRepo != nil && failoverReq != nil {
			if err := s.failoverRepo.UpdateStatus(ctx, failoverReq.ID, status, resultData); err != nil {
				s.logger.Warn("failed to update failover request status", "error", err)
			}
		}
	}

	if s.failoverRepo != nil && failoverReq != nil {
		_ = s.failoverRepo.UpdateStatus(ctx, failoverReq.ID, models.FailoverStatusInProgress, nil)
	}

	if node.IPMIAddress != nil && *node.IPMIAddress != "" {
		if err := s.executeSTONITH(ctx, node); err != nil {
			s.logger.Error("STONITH failed, continuing with failover",
				"node_id", targetNodeID,
				"error", err)
		} else {
			result.STONITHExecuted = true
			s.logger.Info("STONITH completed successfully",
				"node_id", targetNodeID,
				"ipmi_address", *node.IPMIAddress)
		}
	} else {
		s.logger.Info("no IPMI configured, assuming manual power-off confirmed",
			"node_id", targetNodeID)
	}

	if err := s.blocklistNodeInCeph(ctx, node); err != nil {
		s.logger.Error("failed to blocklist node in Ceph",
			"node_id", targetNodeID,
			"management_ip", node.ManagementIP,
			"error", err)
	} else {
		result.BlocklistAdded = true
		s.logger.Info("node blocklisted in Ceph",
			"node_id", targetNodeID,
			"management_ip", node.ManagementIP)
	}

	if err := s.releaseRBDLocks(ctx, node); err != nil {
		s.logger.Error("failed to release RBD locks, continuing with failover",
			"node_id", targetNodeID,
			"error", err)
	}

	vms, err := s.getVMsOnNode(ctx, targetNodeID)
	if err != nil {
		finalizeRequest(models.FailoverStatusFailed, map[string]string{"error": err.Error()})
		return nil, fmt.Errorf("getting VMs on failed node: %w", err)
	}
	result.TotalVMs = len(vms)

	if len(vms) == 0 {
		s.logger.Info("no VMs to migrate on failed node",
			"node_id", targetNodeID)
		finalizeRequest(models.FailoverStatusCompleted, result)
		return result, nil
	}

	survivingNodes, err := s.findSurvivingNodes(ctx, node)
	if err != nil {
		finalizeRequest(models.FailoverStatusFailed, map[string]string{"error": err.Error()})
		return nil, fmt.Errorf("finding surviving nodes: %w", err)
	}

	if len(survivingNodes) == 0 {
		finalizeRequest(models.FailoverStatusFailed, map[string]string{"error": "no surviving nodes available"})
		return nil, fmt.Errorf("no surviving nodes available for VM migration")
	}

	result.MigratedVMs = make([]MigratedVM, 0)
	result.FailedMigrations = make([]FailedMigration, 0)

	for _, vm := range vms {
		migrated, failed := s.migrateVM(ctx, &vm, survivingNodes)
		if migrated != nil {
			result.MigratedVMs = append(result.MigratedVMs, *migrated)
		} else if failed != nil {
			result.FailedMigrations = append(result.FailedMigrations, *failed)
		}
	}

	s.logFailoverAudit(ctx, adminID, result)

	s.logger.Info("failover completed",
		"node_id", targetNodeID,
		"total_vms", result.TotalVMs,
		"migrated", len(result.MigratedVMs),
		"failed", len(result.FailedMigrations))

	finalizeRequest(models.FailoverStatusCompleted, result)

	return result, nil
}

// verifyFailedNode checks that the node is in a proper failed state for failover.
// A node is considered failed if consecutive_heartbeat_misses >= 3.
func (s *FailoverService) verifyFailedNode(ctx context.Context, nodeID string) (*models.Node, error) {
	node, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("node not found: %s", nodeID)
		}
		return nil, fmt.Errorf("getting node: %w", err)
	}

	// Check if node is in failed state
	if node.ConsecutiveHeartbeatMisses < 3 {
		return nil, fmt.Errorf("node %s is not in failed state (consecutive_heartbeat_misses: %d, required: >= 3)",
			nodeID, node.ConsecutiveHeartbeatMisses)
	}

	// Update node status to failed if not already
	if node.Status != models.NodeStatusFailed {
		if err := s.nodeRepo.UpdateStatus(ctx, nodeID, models.NodeStatusFailed); err != nil {
			s.logger.Warn("failed to update node status to failed",
				"node_id", nodeID,
				"error", err)
		}
	}

	s.logger.Info("node verified as failed",
		"node_id", nodeID,
		"hostname", node.Hostname,
		"consecutive_heartbeat_misses", node.ConsecutiveHeartbeatMisses)

	return node, nil
}

// executeSTONITH performs IPMI power-off on the failed node.
// This ensures the node is truly down before proceeding with failover.
func (s *FailoverService) executeSTONITH(ctx context.Context, node *models.Node) error {
	// Decrypt IPMI credentials
	if node.IPMIUsernameEncrypted == nil || node.IPMIPasswordEncrypted == nil {
		return sharederrors.ErrNoIPMIConfigured
	}

	username, err := crypto.Decrypt(*node.IPMIUsernameEncrypted, s.encryptionKey)
	if err != nil {
		return fmt.Errorf("decrypting IPMI username: %w", err)
	}

	password, err := crypto.Decrypt(*node.IPMIPasswordEncrypted, s.encryptionKey)
	if err != nil {
		return fmt.Errorf("decrypting IPMI password: %w", err)
	}

	ipmiAddress := *node.IPMIAddress

	s.logger.Info("executing STONITH via IPMI",
		"node_id", node.ID,
		"ipmi_address", ipmiAddress)

	// Execute IPMI power off command using environment variable to avoid exposing password in process list
	cmd := exec.CommandContext(ctx, "ipmitool",
		"-H", ipmiAddress,
		"-U", username,
		"-E", // Use IPMITOOL_PASSWORD environment variable
		"power", "off")
	cmd.Env = append(cmd.Environ(), "IPMITOOL_PASSWORD="+password)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("IPMI power off failed: %w, output: %s", err, string(output))
	}

	// Wait 10 seconds for power-off to complete
	select {
	case <-time.After(10 * time.Second):
		// Expected path
	case <-ctx.Done():
		return fmt.Errorf("context cancelled during STONITH wait: %w", ctx.Err())
	}

	// Verify power state using environment variable to avoid exposing password
	verifyCmd := exec.CommandContext(ctx, "ipmitool",
		"-H", ipmiAddress,
		"-U", username,
		"-E", // Use IPMITOOL_PASSWORD environment variable
		"power", "status")
	verifyCmd.Env = append(verifyCmd.Environ(), "IPMITOOL_PASSWORD="+password)

	verifyOutput, err := verifyCmd.CombinedOutput()
	if err != nil {
		s.logger.Warn("could not verify power status after STONITH",
			"node_id", node.ID,
			"error", err)
	} else {
		s.logger.Debug("power status after STONITH",
			"node_id", node.ID,
			"status", string(verifyOutput))
	}

	return nil
}

// blocklistNodeInCeph adds the failed node's management IP to the Ceph OSD blocklist.
// This prevents the failed node from accessing Ceph storage, ensuring data integrity.
func (s *FailoverService) blocklistNodeInCeph(ctx context.Context, node *models.Node) error {
	// Validate the management IP to prevent command injection
	ip := net.ParseIP(node.ManagementIP)
	if ip == nil {
		return fmt.Errorf("invalid management IP: %s", node.ManagementIP)
	}

	// Use the string representation of the parsed IP for safety
	safeIP := ip.String()

	s.logger.Info("adding node to Ceph blocklist",
		"node_id", node.ID,
		"management_ip", safeIP)

	// Ensure a timeout is applied to prevent the command from hanging indefinitely
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Execute ceph osd blocklist add command
	cmd := exec.CommandContext(ctx, "ceph",
		"osd", "blocklist", "add",
		safeIP)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ceph blocklist add failed: %w, output: %s", err, string(output))
	}

	s.logger.Info("node added to Ceph blocklist",
		"node_id", node.ID,
		"management_ip", safeIP,
		"output", string(output))

	return nil
}

// releaseRBDLocks releases RBD locks for VMs on the failed node.
// This is done after blocklisting to ensure clean VM recovery.
func (s *FailoverService) releaseRBDLocks(ctx context.Context, node *models.Node) error {
	s.logger.Info("releasing RBD locks for failed node",
		"node_id", node.ID,
		"ceph_pool", node.CephPool,
		"storage_backend", node.StorageBackend)

	vms, err := s.getVMsOnNode(ctx, node.ID)
	if err != nil {
		return fmt.Errorf("listing VMs on failed node: %w", err)
	}

	var firstErr error
	for _, vm := range vms {
		if vm.StorageBackend != "" && vm.StorageBackend != "ceph" {
			s.logger.Debug("skipping RBD lock release for non-Ceph VM",
				"vm_id", vm.ID,
				"backend", vm.StorageBackend)
			continue
		}

		image := fmt.Sprintf("%s/vm-%s", node.CephPool, vm.ID)

		listCmd := exec.CommandContext(ctx, "rbd", "lock", "list", image, "--format", "json")
		listOut, err := listCmd.CombinedOutput()
		if err != nil {
			s.logger.Warn("failed to list RBD locks, skipping VM",
				"image", image,
				"error", err,
				"output", string(listOut))
			if firstErr == nil {
				firstErr = fmt.Errorf("listing RBD locks for %s: %w", image, err)
			}
			continue
		}

		locks, err := parseRBDLocks(listOut)
		if err != nil {
			s.logger.Warn("failed to parse RBD lock list, skipping VM",
				"image", image,
				"error", err)
			if firstErr == nil {
				firstErr = fmt.Errorf("parsing RBD locks for %s: %w", image, err)
			}
			continue
		}

		for _, lock := range locks {
			if lock.ID == "" || lock.Locker == "" {
				continue
			}

			removeCmd := exec.CommandContext(ctx, "rbd", "lock", "remove", image, lock.ID, lock.Locker)
			removeOut, err := removeCmd.CombinedOutput()
			if err != nil {
				s.logger.Warn("failed to remove RBD lock",
					"image", image,
					"lock_id", lock.ID,
					"locker", lock.Locker,
					"error", err,
					"output", string(removeOut))
				continue
			}

			s.logger.Info("released RBD lock",
				"node_id", node.ID,
				"vm_id", vm.ID,
				"image", image,
				"lock_id", lock.ID,
				"locker", lock.Locker)
		}
	}

	s.logger.Info("RBD lock release completed", "node_id", node.ID, "vm_count", len(vms))
	return firstErr
}

type rbdLock struct {
	ID     string
	Locker string
}

func parseRBDLocks(raw []byte) ([]rbdLock, error) {
	var wrapped struct {
		Locks []struct {
			ID     string `json:"id"`
			Locker string `json:"locker"`
			Client string `json:"client"`
		} `json:"locks"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && len(wrapped.Locks) > 0 {
		locks := make([]rbdLock, 0, len(wrapped.Locks))
		for _, l := range wrapped.Locks {
			locker := l.Locker
			if locker == "" {
				locker = l.Client
			}
			locks = append(locks, rbdLock{ID: l.ID, Locker: locker})
		}
		return locks, nil
	}

	var arr []struct {
		ID     string `json:"id"`
		Locker string `json:"locker"`
		Client string `json:"client"`
	}
	if err := json.Unmarshal(raw, &arr); err == nil {
		locks := make([]rbdLock, 0, len(arr))
		for _, l := range arr {
			locker := l.Locker
			if locker == "" {
				locker = l.Client
			}
			locks = append(locks, rbdLock{ID: l.ID, Locker: locker})
		}
		return locks, nil
	}

	return nil, fmt.Errorf("unexpected lock list format")
}

// getVMsOnNode retrieves all VMs assigned to a specific node.
func (s *FailoverService) getVMsOnNode(ctx context.Context, nodeID string) ([]models.VM, error) {
	filter := models.VMListFilter{
		NodeID: &nodeID,
		PaginationParams: models.PaginationParams{
			Page:    1,
			PerPage: models.MaxPerPage,
		},
	}

	vms, _, err := s.vmRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("listing VMs on node %s: %w", nodeID, err)
	}

	return vms, nil
}

// findSurvivingNodes returns all online nodes that can accept VM migrations.
// Nodes in draining, offline, or failed status are excluded.
func (s *FailoverService) findSurvivingNodes(ctx context.Context, failedNode *models.Node) ([]models.Node, error) {
	// Get all online nodes
	filter := models.NodeListFilter{
		Status: util.StringPtr(models.NodeStatusOnline),
	}

	nodes, _, err := s.nodeRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("listing online nodes: %w", err)
	}

	// Filter out the failed node (shouldn't be in the list, but be safe)
	surviving := make([]models.Node, 0, len(nodes))
	for _, n := range nodes {
		if n.ID != failedNode.ID {
			surviving = append(surviving, n)
		}
	}

	// If we have location information, prefer nodes in the same location
	if failedNode.LocationID != nil {
		surviving = s.sortNodesByLocation(surviving, *failedNode.LocationID)
	}

	return surviving, nil
}

// sortNodesByLocation sorts nodes with preference for the same location.
func (s *FailoverService) sortNodesByLocation(nodes []models.Node, preferredLocationID string) []models.Node {
	// Simple sort: nodes in preferred location first
	sameLocation := make([]models.Node, 0)
	otherLocations := make([]models.Node, 0)

	for _, n := range nodes {
		if n.LocationID != nil && *n.LocationID == preferredLocationID {
			sameLocation = append(sameLocation, n)
		} else {
			otherLocations = append(otherLocations, n)
		}
	}

	return append(sameLocation, otherLocations...)
}

// migrateVM migrates a single VM to one of the surviving nodes.
// Returns either a successful migration or a failure reason.
func (s *FailoverService) migrateVM(ctx context.Context, vm *models.VM, survivingNodes []models.Node) (*MigratedVM, *FailedMigration) {
	// Select the best node for this VM
	targetNode := s.selectBestNode(vm, survivingNodes)
	if targetNode == nil {
		return nil, &FailedMigration{
			VMID:     vm.ID,
			Hostname: vm.Hostname,
			Error:    "no suitable node found for migration",
		}
	}

	s.logger.Info("migrating VM",
		"vm_id", vm.ID,
		"hostname", vm.Hostname,
		"old_node_id", vm.NodeID,
		"new_node_id", targetNode.ID)

	// Update VM's node assignment in the database
	if err := s.vmRepo.UpdateNodeAssignment(ctx, vm.ID, targetNode.ID); err != nil {
		return nil, &FailedMigration{
			VMID:     vm.ID,
			Hostname: vm.Hostname,
			Error:    fmt.Sprintf("failed to update node assignment: %v", err),
		}
	}

	// Update VM status to indicate it's being recovered
	if err := s.vmRepo.UpdateStatus(ctx, vm.ID, models.VMStatusProvisioning); err != nil {
		s.logger.Warn("failed to update VM status during migration",
			"vm_id", vm.ID,
			"error", err)
	}

	// Attempt to start the VM on the new node
	if s.nodeAgent != nil {
		if err := s.nodeAgent.StartVM(ctx, targetNode.ID, vm.ID); err != nil {
			s.logger.Warn("failed to start VM on new node",
				"vm_id", vm.ID,
				"new_node_id", targetNode.ID,
				"error", err)
			// Don't fail the entire migration - the VM is reassigned and can be started manually
		} else {
			// Update status to running - failure not critical as VM is already migrated
			_ = s.vmRepo.UpdateStatus(ctx, vm.ID, models.VMStatusRunning)
		}
	} else {
		s.logger.Warn("node agent not available, VM will need manual start",
			"vm_id", vm.ID,
			"new_node_id", targetNode.ID)
	}

	// Update allocated resources on the new node
	newAllocatedVCPU := targetNode.AllocatedVCPU + vm.VCPU
	newAllocatedMemory := targetNode.AllocatedMemoryMB + vm.MemoryMB
	if err := s.nodeRepo.UpdateAllocatedResources(ctx, targetNode.ID, newAllocatedVCPU, newAllocatedMemory); err != nil {
		s.logger.Warn("failed to update allocated resources on target node",
			"node_id", targetNode.ID,
			"error", err)
	}

	// Decrement allocated resources from the old (failed) node
	if vm.NodeID != nil {
		oldNode, err := s.nodeRepo.GetByID(ctx, *vm.NodeID)
		if err == nil {
			oldVCPU := oldNode.AllocatedVCPU - vm.VCPU
			oldMemory := oldNode.AllocatedMemoryMB - vm.MemoryMB
			if oldVCPU < 0 {
				oldVCPU = 0
			}
			if oldMemory < 0 {
				oldMemory = 0
			}
			if err := s.nodeRepo.UpdateAllocatedResources(ctx, *vm.NodeID, oldVCPU, oldMemory); err != nil {
				s.logger.Warn("failed to decrement allocated resources on failed node",
					"node_id", *vm.NodeID,
					"error", err)
			}
		}
	}

	return &MigratedVM{
		VMID:        vm.ID,
		Hostname:    vm.Hostname,
		OldNodeID:   *vm.NodeID,
		NewNodeID:   targetNode.ID,
		NewNodeName: targetNode.Hostname,
	}, nil
}

// selectBestNode selects the best surviving node for a VM based on available capacity.
func (s *FailoverService) selectBestNode(vm *models.VM, nodes []models.Node) *models.Node {
	var bestNode *models.Node
	bestScore := -1

	for i := range nodes {
		node := &nodes[i]

		// Check if node has enough capacity
		availableVCPU := node.TotalVCPU - node.AllocatedVCPU
		availableMemory := node.TotalMemoryMB - node.AllocatedMemoryMB

		if availableVCPU < vm.VCPU || availableMemory < vm.MemoryMB {
			continue // Not enough capacity
		}

		// Score based on available memory (prefer nodes with more free resources)
		score := availableVCPU + (availableMemory / 1024) // Normalize memory to roughly match CPU weight

		if score > bestScore {
			bestScore = score
			bestNode = node
		}
	}

	return bestNode
}

// logFailoverAudit logs the failover operation to the audit trail.
func (s *FailoverService) logFailoverAudit(ctx context.Context, adminID string, result *FailoverResult) {
	s.logger.Info("failover audit",
		"actor_type", models.AuditActorAdmin,
		"actor_id", adminID,
		"action", "node.failover",
		"resource_type", "node",
		"resource_id", result.NodeID,
		"stonith_executed", result.STONITHExecuted,
		"blocklist_added", result.BlocklistAdded,
		"total_vms", result.TotalVMs,
		"migrated_count", len(result.MigratedVMs),
		"failed_count", len(result.FailedMigrations))

	// If auditRepo is available, write to audit log
	if s.auditRepo != nil {
		success := len(result.FailedMigrations) == 0
		var errorMsg *string
		if !success {
			msg := fmt.Sprintf("%d VMs failed to migrate", len(result.FailedMigrations))
			errorMsg = &msg
		}

		audit := &models.AuditLog{
			ActorID:      &adminID,
			ActorType:    models.AuditActorAdmin,
			Action:       "node.failover",
			ResourceType: "node",
			ResourceID:   &result.NodeID,
			Success:      success,
			ErrorMessage: errorMsg,
		}

		if err := s.auditRepo.Append(ctx, audit); err != nil {
			s.logger.Warn("failed to write audit log for failover",
				"node_id", result.NodeID,
				"error", err)
		}
	}
}

const systemActorID = "system:auto-failover"

func (s *FailoverService) AutoFailover(ctx context.Context, nodeID, correlationID string) (*FailoverResult, error) {
	s.logger.Info("auto-failover triggered",
		"node_id", nodeID,
		"correlation_id", correlationID)

	return s.ApproveFailover(ctx, systemActorID, nodeID)
}
