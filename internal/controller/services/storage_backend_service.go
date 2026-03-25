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

// StorageBackendService provides business logic for managing storage backends.
// It handles CRUD operations, node assignments, and deletion guards.
type StorageBackendService struct {
	repo            *repository.StorageBackendRepository
	nodeRepo        *repository.NodeRepository
	nodeStorage     *repository.NodeStorageRepository
	taskRepo        *repository.TaskRepository
	adminBackupScheduleRepo *repository.AdminBackupScheduleRepository
	logger          *slog.Logger
}

// NewStorageBackendService creates a new StorageBackendService with the given dependencies.
func NewStorageBackendService(
	repo *repository.StorageBackendRepository,
	nodeRepo *repository.NodeRepository,
	nodeStorage *repository.NodeStorageRepository,
	taskRepo *repository.TaskRepository,
	adminBackupScheduleRepo *repository.AdminBackupScheduleRepository,
	logger *slog.Logger,
) *StorageBackendService {
	return &StorageBackendService{
		repo:               repo,
		nodeRepo:           nodeRepo,
		nodeStorage:        nodeStorage,
		taskRepo:           taskRepo,
		adminBackupScheduleRepo: adminBackupScheduleRepo,
		logger:             logger.With("component", "storage-backend-service"),
	}
}

// Create creates a new storage backend and optionally assigns it to nodes.
func (s *StorageBackendService) Create(ctx context.Context, req *models.StorageBackendCreateRequest) (*models.StorageBackend, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if req.Type == "" {
		return nil, fmt.Errorf("type is required")
	}

	// Validate type-specific fields
	if err := s.validateTypeConfig(req); err != nil {
		return nil, err
	}

	// Check for duplicate name
	existing, err := s.repo.GetByName(ctx, req.Name)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("storage backend with name %q already exists", req.Name)
	}

	// Validate LVM thresholds if provided
	if req.LVMDataPercentThreshold != nil && (*req.LVMDataPercentThreshold < 1 || *req.LVMDataPercentThreshold > 100) {
		return nil, fmt.Errorf("lvm_data_percent_threshold must be between 1 and 100, got %d", *req.LVMDataPercentThreshold)
	}
	if req.LVMMetadataPercentThreshold != nil && (*req.LVMMetadataPercentThreshold < 1 || *req.LVMMetadataPercentThreshold > 100) {
		return nil, fmt.Errorf("lvm_metadata_percent_threshold must be between 1 and 100, got %d", *req.LVMMetadataPercentThreshold)
	}

	// Create the storage backend record
	sb := &models.StorageBackend{
		Name:             req.Name,
		Type:             req.Type,
		CephPool:         req.CephPool,
		CephUser:         req.CephUser,
		CephMonitors:     req.CephMonitors,
		CephKeyringPath:  req.CephKeyringPath,
		StoragePath:      req.StoragePath,
		LVMVolumeGroup:   req.LVMVolumeGroup,
		LVMThinPool:      req.LVMThinPool,
		LVMDataPercentThreshold:     req.LVMDataPercentThreshold,
		LVMMetadataPercentThreshold: req.LVMMetadataPercentThreshold,
		HealthStatus:     "unknown",
	}

	if err := s.repo.Create(ctx, sb); err != nil {
		return nil, fmt.Errorf("creating storage backend: %w", err)
	}

	// Assign to nodes if node IDs provided
	if len(req.NodeIDs) > 0 {
		if err := s.repo.AssignToNodes(ctx, sb.ID, req.NodeIDs); err != nil {
			s.logger.Warn("failed to assign storage backend to nodes",
				"storage_backend_id", sb.ID,
				"node_ids", req.NodeIDs,
				"error", err)
		}
	}

	s.logger.Info("storage backend created",
		"storage_backend_id", sb.ID,
		"name", sb.Name,
		"type", sb.Type,
		"node_count", len(req.NodeIDs))

	return sb, nil
}

// GetByID retrieves a storage backend by ID.
func (s *StorageBackendService) GetByID(ctx context.Context, id string) (*models.StorageBackend, error) {
	sb, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("storage backend not found: %s", id)
		}
		return nil, fmt.Errorf("getting storage backend: %w", err)
	}
	return sb, nil
}

// GetByIDWithNodes retrieves a storage backend by ID with its assigned nodes.
func (s *StorageBackendService) GetByIDWithNodes(ctx context.Context, id string) (*models.StorageBackend, error) {
	sb, err := s.repo.GetByIDWithNodes(ctx, id)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("storage backend not found: %s", id)
		}
		return nil, fmt.Errorf("getting storage backend: %w", err)
	}
	return sb, nil
}

// List returns a paginated list of storage backends.
func (s *StorageBackendService) List(ctx context.Context, filter models.StorageBackendListFilter) ([]models.StorageBackend, int, error) {
	return s.repo.List(ctx, filter)
}

// ListAll returns all storage backends without pagination.
func (s *StorageBackendService) ListAll(ctx context.Context) ([]models.StorageBackend, error) {
	return s.repo.ListAll(ctx)
}

// Update updates an existing storage backend.
func (s *StorageBackendService) Update(ctx context.Context, id string, req *models.StorageBackendUpdateRequest) (*models.StorageBackend, error) {
	sb, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("storage backend not found: %s", id)
		}
		return nil, fmt.Errorf("getting storage backend: %w", err)
	}

	// Apply updates
	if req.Name != nil {
		// Check for duplicate name if name is being changed
		if *req.Name != sb.Name {
			existing, err := s.repo.GetByName(ctx, *req.Name)
			if err == nil && existing != nil {
				return nil, fmt.Errorf("storage backend with name %q already exists", *req.Name)
			}
		}
		sb.Name = *req.Name
	}
	if req.CephPool != nil {
		sb.CephPool = req.CephPool
	}
	if req.CephUser != nil {
		sb.CephUser = req.CephUser
	}
	if req.CephMonitors != nil {
		sb.CephMonitors = req.CephMonitors
	}
	if req.CephKeyringPath != nil {
		sb.CephKeyringPath = req.CephKeyringPath
	}
	if req.StoragePath != nil {
		sb.StoragePath = req.StoragePath
	}
	if req.LVMVolumeGroup != nil {
		sb.LVMVolumeGroup = req.LVMVolumeGroup
	}
	if req.LVMThinPool != nil {
		sb.LVMThinPool = req.LVMThinPool
	}
	if req.LVMDataPercentThreshold != nil {
		if *req.LVMDataPercentThreshold < 1 || *req.LVMDataPercentThreshold > 100 {
			return nil, fmt.Errorf("lvm_data_percent_threshold must be between 1 and 100, got %d", *req.LVMDataPercentThreshold)
		}
		sb.LVMDataPercentThreshold = req.LVMDataPercentThreshold
	}
	if req.LVMMetadataPercentThreshold != nil {
		if *req.LVMMetadataPercentThreshold < 1 || *req.LVMMetadataPercentThreshold > 100 {
			return nil, fmt.Errorf("lvm_metadata_percent_threshold must be between 1 and 100, got %d", *req.LVMMetadataPercentThreshold)
		}
		sb.LVMMetadataPercentThreshold = req.LVMMetadataPercentThreshold
	}

	if err := s.repo.Update(ctx, sb); err != nil {
		return nil, fmt.Errorf("updating storage backend: %w", err)
	}

	s.logger.Info("storage backend updated",
		"storage_backend_id", sb.ID,
		"name", sb.Name)

	return sb, nil
}

// Delete removes a storage backend after checking for assigned nodes, VMs, and templates.
func (s *StorageBackendService) Delete(ctx context.Context, id string) error {
	// Verify the storage backend exists
	sb, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("storage backend not found: %s", id)
		}
		return fmt.Errorf("getting storage backend: %w", err)
	}

	// Check for assigned nodes
	nodeCount, err := s.nodeStorage.CountNodesForBackend(ctx, id)
	if err != nil {
		return fmt.Errorf("checking assigned nodes: %w", err)
	}
	if nodeCount > 0 {
		return fmt.Errorf("cannot delete storage backend %q: %d nodes are still assigned", sb.Name, nodeCount)
	}

	// Check for in-flight tasks referencing this storage backend
	if s.taskRepo != nil {
		taskCount, err := s.taskRepo.CountByStorageBackend(ctx, id)
		if err != nil {
			return fmt.Errorf("checking in-flight tasks: %w", err)
		}
		if taskCount > 0 {
			return fmt.Errorf("cannot delete storage backend %q: %d in-flight tasks are using this backend", sb.Name, taskCount)
		}
	}

	// Check for active backup schedules using this storage backend
	if s.adminBackupScheduleRepo != nil {
		scheduleCount, err := s.adminBackupScheduleRepo.CountByStorageBackend(ctx, id)
		if err != nil {
			return fmt.Errorf("checking backup schedules: %w", err)
		}
		if scheduleCount > 0 {
			return fmt.Errorf("cannot delete storage backend %q: %d backup schedules are using this backend", sb.Name, scheduleCount)
		}
	}

	// Delete the storage backend
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("deleting storage backend: %w", err)
	}

	// Clean up any remaining node assignments (should be none, but be safe)
	if err := s.nodeStorage.DeleteAllAssignmentsForBackend(ctx, id); err != nil {
		s.logger.Warn("failed to clean up node assignments after delete",
			"storage_backend_id", id,
			"error", err)
	}

	s.logger.Info("storage backend deleted",
		"storage_backend_id", id,
		"name", sb.Name)

	return nil
}

// AssignToNodes assigns a storage backend to multiple nodes.
func (s *StorageBackendService) AssignToNodes(ctx context.Context, id string, nodeIDs []string) error {
	// Verify the storage backend exists
	_, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("storage backend not found: %s", id)
		}
		return fmt.Errorf("getting storage backend: %w", err)
	}

	// Verify all nodes exist
	for _, nodeID := range nodeIDs {
		_, err := s.nodeRepo.GetByID(ctx, nodeID)
		if err != nil {
			if sharederrors.Is(err, sharederrors.ErrNotFound) {
				return fmt.Errorf("node not found: %s", nodeID)
			}
			return fmt.Errorf("getting node %s: %w", nodeID, err)
		}
	}

	if err := s.repo.AssignToNodes(ctx, id, nodeIDs); err != nil {
		return fmt.Errorf("assigning storage backend to nodes: %w", err)
	}

	s.logger.Info("storage backend assigned to nodes",
		"storage_backend_id", id,
		"node_count", len(nodeIDs))

	return nil
}

// RemoveFromNode removes a storage backend assignment from a specific node.
func (s *StorageBackendService) RemoveFromNode(ctx context.Context, id, nodeID string) error {
	// Verify the storage backend exists
	_, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("storage backend not found: %s", id)
		}
		return fmt.Errorf("getting storage backend: %w", err)
	}

	if err := s.repo.RemoveFromNode(ctx, id, nodeID); err != nil {
		return fmt.Errorf("removing storage backend from node: %w", err)
	}

	s.logger.Info("storage backend removed from node",
		"storage_backend_id", id,
		"node_id", nodeID)

	return nil
}

// GetBackendsForNode returns all storage backends assigned to a specific node.
func (s *StorageBackendService) GetBackendsForNode(ctx context.Context, nodeID string) ([]models.StorageBackend, error) {
	return s.repo.GetBackendsForNode(ctx, nodeID)
}

// GetBackendsForNodeByType returns all storage backends of a specific type assigned to a node.
func (s *StorageBackendService) GetBackendsForNodeByType(ctx context.Context, nodeID string, backendType string) ([]models.StorageBackend, error) {
	backends, err := s.repo.GetBackendsForNode(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("getting backends for node %s: %w", nodeID, err)
	}

	// Filter by type
	var filtered []models.StorageBackend
	for _, b := range backends {
		if string(b.Type) == backendType {
			filtered = append(filtered, b)
		}
	}

	return filtered, nil
}

// validateTypeConfig validates that the required type-specific fields are set.
func (s *StorageBackendService) validateTypeConfig(req *models.StorageBackendCreateRequest) error {
	switch req.Type {
	case models.StorageTypeCeph:
		if req.CephPool == nil || *req.CephPool == "" {
			return fmt.Errorf("ceph_pool is required for ceph storage backend")
		}
	case models.StorageTypeQCOW:
		if req.StoragePath == nil || *req.StoragePath == "" {
			return fmt.Errorf("storage_path is required for qcow storage backend")
		}
	case models.StorageTypeLVM:
		if req.LVMVolumeGroup == nil || *req.LVMVolumeGroup == "" {
			return fmt.Errorf("lvm_volume_group is required for lvm storage backend")
		}
		if req.LVMThinPool == nil || *req.LVMThinPool == "" {
			return fmt.Errorf("lvm_thin_pool is required for lvm storage backend")
		}
	default:
		return fmt.Errorf("unknown storage backend type: %s", req.Type)
	}
	return nil
}

// PollStorageBackendHealth polls storage backend health from assigned nodes and updates the backend.
//
// Implementation Architecture:
// 
// Storage backend health is updated through the node heartbeat system, not direct polling:
//
// 1. Node Agent Heartbeat: Each node agent periodically sends heartbeat messages to the controller
//    containing node health metrics (CPU, memory, disk, VM count). This is implemented in
//    internal/controller/services/heartbeat_checker.go.
//
// 2. Storage Metrics: For nodes assigned storage backends (via node_storage_backends junction),
//    the heartbeat includes storage backend health metrics:
//    - Ceph: Output of 'ceph df' (total_gb, used_gb, available_gb, health_status)
//    - QCOW: Disk usage of storage_path directory
//    - LVM: Output of 'lvs' (data_percent, metadata_percent)
//
// 3. Database Update: The heartbeat processor calls UpdateStorageBackendHealth() to persist
//    the metrics to storage_backends.health_status, health_message, total_gb, used_gb, etc.
//
// This method (PollStorageBackendHealth) is a placeholder for on-demand polling. The actual
// implementation lives in the heartbeat checker service, which calls UpdateStorageBackendHealth()
// when node health updates arrive.
//
// To implement immediate/on-demand health checks:
// 1. Add gRPC method to node_agent.proto for GetStorageBackendHealth
// 2. Create NATS task type "storage_backend.health_check"
// 3. Implement task handler that calls node agent gRPC and updates database
//
// For now, health status is updated passively via heartbeat polling (default: 30-second interval).
func (s *StorageBackendService) PollStorageBackendHealth(ctx context.Context, sb *models.StorageBackend) error {
	if sb == nil {
		return fmt.Errorf("storage backend is nil")
	}

	s.logger.Debug("polling storage backend health - health updates are delivered via node heartbeat",
		"storage_backend_id", sb.ID,
		"type", sb.Type)

	// Health polling is performed by the heartbeat checker service.
	// Node agents send periodic health updates that include storage metrics.
	// The controller's heartbeat processor calls UpdateStorageBackendHealth()
	// when new storage health data arrives.
	//
	// This method exists for API completeness but currently returns nil without action.
	// See internal/controller/services/heartbeat_checker.go for the actual implementation.
	return nil
}

// UpdateStorageBackendHealth updates the health metrics for a storage backend.
func (s *StorageBackendService) UpdateStorageBackendHealth(ctx context.Context, id string, health models.StorageBackendHealth) error {
	if err := s.repo.UpdateHealth(ctx, id, health); err != nil {
		return fmt.Errorf("updating storage backend health: %w", err)
	}

	s.logger.Debug("storage backend health updated",
		"storage_backend_id", id,
		"status", health.HealthStatus)

	return nil
}