// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// NodeAgentClient abstracts gRPC communication with node agents for node management.
// This interface allows the NodeService to call node agent methods without
// depending directly on generated protobuf code.
type NodeAgentClient interface {
	// GetNodeMetrics retrieves real-time metrics from a node.
	GetNodeMetrics(ctx context.Context, nodeID string) (*models.NodeHeartbeat, error)
	// PingNode checks if a node is reachable.
	PingNode(ctx context.Context, nodeID string) error
	// EvacuateNode evacuates all VMs from a node (for maintenance).
	EvacuateNode(ctx context.Context, nodeID string) error
}

// NodeService provides node management operations for VirtueStack.
// It handles node registration, heartbeat processing, status management,
// and node selection for VM placement.
type NodeService struct {
	nodeRepo    *repository.NodeRepository
	vmRepo      *repository.VMRepository
	nodeAgent   NodeAgentClient
	encryptionKey string // For encrypting IPMI credentials
	logger      *slog.Logger
}

// NewNodeService creates a new NodeService with the given dependencies.
func NewNodeService(
	nodeRepo *repository.NodeRepository,
	vmRepo *repository.VMRepository,
	nodeAgent NodeAgentClient,
	encryptionKey string,
	logger *slog.Logger,
) *NodeService {
	return &NodeService{
		nodeRepo:      nodeRepo,
		vmRepo:        vmRepo,
		nodeAgent:     nodeAgent,
		encryptionKey: encryptionKey,
		logger:        logger.With("component", "node-service"),
	}
}

// RegisterNode registers a new hypervisor node.
// It validates the request, encrypts sensitive credentials, and creates the node record.
func (s *NodeService) RegisterNode(ctx context.Context, req *models.NodeCreateRequest) (*models.Node, error) {
	// Check if node with same hostname already exists
	existing, err := s.nodeRepo.GetByHostname(ctx, req.Hostname)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("node with hostname %s already exists", req.Hostname)
	}
	if err != nil && !sharederrors.Is(err, sharederrors.ErrNotFound) {
		return nil, fmt.Errorf("checking existing node: %w", err)
	}

	// Prepare node record
	node := &models.Node{
		Hostname:      req.Hostname,
		GRPCAddress:   req.GRPCAddress,
		ManagementIP:  req.ManagementIP,
		LocationID:    req.LocationID,
		Status:        models.NodeStatusOnline,
		TotalVCPU:     req.TotalVCPU,
		TotalMemoryMB: req.TotalMemoryMB,
		CephPool:      req.CephPool,
		IPMIAddress:   req.IPMIAddress,
	}

	// Encrypt IPMI credentials if provided
	if req.IPMIUsername != nil && req.IPMIPassword != nil {
		encryptedUsername, err := crypto.Encrypt(*req.IPMIUsername, s.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("encrypting IPMI username: %w", err)
		}
		encryptedPassword, err := crypto.Encrypt(*req.IPMIPassword, s.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("encrypting IPMI password: %w", err)
		}
		node.IPMIUsernameEncrypted = &encryptedUsername
		node.IPMIPasswordEncrypted = &encryptedPassword
	}

	// Create node in database
	if err := s.nodeRepo.Create(ctx, node); err != nil {
		return nil, fmt.Errorf("creating node: %w", err)
	}

	s.logger.Info("node registered",
		"node_id", node.ID,
		"hostname", node.Hostname,
		"location_id", node.LocationID)

	return node, nil
}

// UpdateHeartbeat records a heartbeat from a node agent.
// It updates the node's last_heartbeat_at and resets the consecutive miss counter.
func (s *NodeService) UpdateHeartbeat(ctx context.Context, nodeID string, hb *models.NodeHeartbeat) error {
	// Verify node exists
	node, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("node not found: %s", nodeID)
		}
		return fmt.Errorf("getting node: %w", err)
	}

	// Set node ID in heartbeat
	hb.NodeID = nodeID

	// Record the heartbeat
	if err := s.nodeRepo.RecordHeartbeat(ctx, hb); err != nil {
		return fmt.Errorf("recording heartbeat: %w", err)
	}

	// Update node's allocated resources based on VM count
	vmCount, err := s.vmRepo.CountByNode(ctx, nodeID)
	if err != nil {
		s.logger.Warn("failed to count VMs for node", "node_id", nodeID, "error", err)
	} else if vmCount != hb.VMCount {
		s.logger.Debug("VM count mismatch between heartbeat and database",
			"node_id", nodeID,
			"heartbeat_count", hb.VMCount,
			"db_count", vmCount)
	}

	s.logger.Debug("heartbeat recorded",
		"node_id", nodeID,
		"hostname", node.Hostname,
		"cpu_percent", hb.CPUPercent,
		"memory_percent", hb.MemoryPercent,
		"vm_count", hb.VMCount)

	return nil
}

// DrainNode sets a node's status to draining, preventing new VM placements.
// Existing VMs continue to run but no new VMs will be placed on this node.
// Use this for planned maintenance.
func (s *NodeService) DrainNode(ctx context.Context, nodeID string) error {
	// Verify node exists
	node, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("node not found: %s", nodeID)
		}
		return fmt.Errorf("getting node: %w", err)
	}

	// Check if node can be drained
	if node.Status == models.NodeStatusDraining {
		return fmt.Errorf("node %s is already draining", nodeID)
	}
	if node.Status == models.NodeStatusFailed {
		return fmt.Errorf("node %s is in failed state, cannot drain", nodeID)
	}

	// Update status to draining
	if err := s.nodeRepo.UpdateStatus(ctx, nodeID, models.NodeStatusDraining); err != nil {
		return fmt.Errorf("updating node status: %w", err)
	}

	s.logger.Info("node set to draining mode",
		"node_id", nodeID,
		"hostname", node.Hostname)

	return nil
}

// FailoverNode sets a node's status to failed and triggers alerting.
// This should be called when a node becomes unresponsive or experiences critical failures.
// In a production system, this would also trigger:
// - Alert notifications (email, Slack, PagerDuty)
// - Automatic VM migration if possible
// - IPMI power cycle attempts
func (s *NodeService) FailoverNode(ctx context.Context, nodeID string) error {
	// Verify node exists
	node, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("node not found: %s", nodeID)
		}
		return fmt.Errorf("getting node: %w", err)
	}

	// Update status to failed
	if err := s.nodeRepo.UpdateStatus(ctx, nodeID, models.NodeStatusFailed); err != nil {
		return fmt.Errorf("updating node status: %w", err)
	}

	// Count affected VMs for logging/alerting
	vmCount, err := s.vmRepo.CountByNode(ctx, nodeID)
	if err != nil {
		s.logger.Warn("failed to count VMs on failed node", "node_id", nodeID, "error", err)
	}

	s.logger.Error("node marked as failed",
		"node_id", nodeID,
		"hostname", node.Hostname,
		"affected_vms", vmCount,
		"ipmi_address", node.IPMIAddress)

	// TODO: Trigger alert notification
	// TODO: Attempt VM migration to other nodes
	// TODO: Attempt IPMI power cycle if configured

	return nil
}

// GetLeastLoadedNode returns the best node for VM placement in a given location.
// It selects the node with the most available capacity that is online and accepting placements.
func (s *NodeService) GetLeastLoadedNode(ctx context.Context, locationID string) (*models.Node, error) {
	if locationID == "" {
		return nil, fmt.Errorf("location ID is required")
	}

	node, err := s.nodeRepo.GetLeastLoadedNode(ctx, locationID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("no available nodes in location %s", locationID)
		}
		return nil, fmt.Errorf("finding least loaded node: %w", err)
	}

	return node, nil
}

// ListNodeVMs returns all VMs running on a specific node.
func (s *NodeService) ListNodeVMs(ctx context.Context, nodeID string) ([]models.VM, error) {
	// Verify node exists
	_, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("node not found: %s", nodeID)
		}
		return nil, fmt.Errorf("getting node: %w", err)
	}

	// List VMs on the node
	filter := models.VMListFilter{
		NodeID: &nodeID,
		PaginationParams: models.PaginationParams{
			Page:    1,
			PerPage: models.MaxPerPage,
		},
	}

	vms, _, err := s.vmRepo.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("listing VMs on node: %w", err)
	}

	return vms, nil
}

// GetNodeStatus returns comprehensive health and status information for a node.
// It aggregates data from the node record and the most recent heartbeat.
func (s *NodeService) GetNodeStatus(ctx context.Context, nodeID string) (*models.NodeStatus, error) {
	// Get node record
	node, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("node not found: %s", nodeID)
		}
		return nil, fmt.Errorf("getting node: %w", err)
	}

	// Get VM count
	vmCount, err := s.vmRepo.CountByNode(ctx, nodeID)
	if err != nil {
		s.logger.Warn("failed to count VMs for node status", "node_id", nodeID, "error", err)
	}

	// Build status response
	status := &models.NodeStatus{
		NodeID:                   node.ID,
		Hostname:                 node.Hostname,
		Status:                   node.Status,
		LastHeartbeatAt:          node.LastHeartbeatAt,
		ConsecutiveHeartbeatMisses: node.ConsecutiveHeartbeatMisses,
		TotalVCPU:                node.TotalVCPU,
		AllocatedVCPU:            node.AllocatedVCPU,
		AvailableVCPU:            node.TotalVCPU - node.AllocatedVCPU,
		TotalMemoryMB:            node.TotalMemoryMB,
		AllocatedMemoryMB:        node.AllocatedMemoryMB,
		AvailableMemoryMB:        node.TotalMemoryMB - node.AllocatedMemoryMB,
		VMCount:                  vmCount,
		IsHealthy:                s.isNodeHealthy(node),
	}

	// Try to get real-time metrics from node agent if available
	if s.nodeAgent != nil && node.Status == models.NodeStatusOnline {
		metrics, err := s.nodeAgent.GetNodeMetrics(ctx, nodeID)
		if err == nil && metrics != nil {
			status.CPUPercent = metrics.CPUPercent
			status.MemoryPercent = metrics.MemoryPercent
			status.DiskPercent = metrics.DiskPercent
			status.LoadAverage = metrics.LoadAverage
		} else {
			s.logger.Debug("failed to get real-time metrics from node agent",
				"node_id", nodeID, "error", err)
		}
	}

	return status, nil
}

// GetNode retrieves a node by ID.
func (s *NodeService) GetNode(ctx context.Context, nodeID string) (*models.Node, error) {
	node, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("node not found: %s", nodeID)
		}
		return nil, fmt.Errorf("getting node: %w", err)
	}
	return node, nil
}

// ListNode retrieves all nodes with optional filtering.
func (s *NodeService) ListNode(ctx context.Context, filter models.NodeListFilter) ([]models.Node, int, error) {
	return s.nodeRepo.List(ctx, filter)
}

// SetNodeOnline sets a node's status back to online.
// Use this after maintenance or recovery.
func (s *NodeService) SetNodeOnline(ctx context.Context, nodeID string) error {
	// Verify node exists
	node, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("node not found: %s", nodeID)
		}
		return fmt.Errorf("getting node: %w", err)
	}

	// Update status to online
	if err := s.nodeRepo.UpdateStatus(ctx, nodeID, models.NodeStatusOnline); err != nil {
		return fmt.Errorf("updating node status: %w", err)
	}

	s.logger.Info("node set back to online",
		"node_id", nodeID,
		"hostname", node.Hostname,
		"previous_status", node.Status)

	return nil
}

// DeleteNode permanently removes a node from the system.
// This should only be called after all VMs have been migrated off the node.
func (s *NodeService) DeleteNode(ctx context.Context, nodeID string) error {
	// Verify node exists
	node, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("node not found: %s", nodeID)
		}
		return fmt.Errorf("getting node: %w", err)
	}

	// Check for remaining VMs
	vmCount, err := s.vmRepo.CountByNode(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("checking VMs on node: %w", err)
	}
	if vmCount > 0 {
		return fmt.Errorf("cannot delete node %s: %d VMs still assigned", nodeID, vmCount)
	}

	// Delete the node
	if err := s.nodeRepo.Delete(ctx, nodeID); err != nil {
		return fmt.Errorf("deleting node: %w", err)
	}

	s.logger.Info("node deleted",
		"node_id", nodeID,
		"hostname", node.Hostname)

	return nil
}

// isNodeHealthy determines if a node is healthy based on its status and heartbeat.
func (s *NodeService) isNodeHealthy(node *models.Node) bool {
	// Node must be online
	if node.Status != models.NodeStatusOnline {
		return false
	}

	// Check for recent heartbeat (within last 5 minutes)
	if node.LastHeartbeatAt == nil {
		return false
	}
	
	heartbeatAge := time.Since(*node.LastHeartbeatAt)
	if heartbeatAge > 5*time.Minute {
		return false
	}

	// Check consecutive heartbeat misses (should be 0 for healthy node)
	if node.ConsecutiveHeartbeatMisses > 0 {
		return false
	}

	return true
}

// generateNodeID generates a unique node ID.
// Currently uses UUID, but this helper allows for future customization.
func generateNodeID() string {
	return uuid.New().String()
}