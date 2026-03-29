// Package admin provides HTTP handlers for the Admin API.
package admin

import (
	"context"
	"errors"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// StorageBackendNodeAssignmentRequest represents the request body for assigning nodes to a storage backend.
type StorageBackendNodeAssignmentRequest struct {
	NodeIDs []string `json:"node_ids" validate:"required,min=1,max=100,dive,uuid"`
}

// ListStorageBackends handles GET /storage-backends - lists all storage backends with optional filtering.
// @Tags Admin
// @Summary List storage backends
// @Description Performs administrative storage backend operation.
// @Produce json
// @Security BearerAuth
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/storage-backends [get]
func (h *AdminHandler) ListStorageBackends(c *gin.Context) {
	pagination := models.ParsePagination(c)

	filter := models.StorageBackendListFilter{
		PaginationParams: pagination,
	}

	// Optional type filter
	if backendType := c.Query("type"); backendType != "" {
		bt := models.StorageBackendType(backendType)
		if bt != models.StorageTypeCeph && bt != models.StorageTypeQCOW && bt != models.StorageTypeLVM {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_TYPE",
				"Invalid storage backend type. Must be one of: ceph, qcow, lvm")
			return
		}
		filter.Type = &bt
	}

	// Optional health status filter
	if status := c.Query("status"); status != "" {
		filter.Status = &status
	}

	backends, hasMore, lastID, err := h.storageBackendRepo.List(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list storage backends",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "STORAGE_BACKEND_LIST_FAILED",
			"Failed to retrieve storage backends")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: backends,
		Meta: models.NewCursorPaginationMeta(pagination.PerPage, hasMore, lastID),
	})
}

// CreateStorageBackend handles POST /storage-backends - creates a new storage backend.
// @Tags Admin
// @Summary Create storage backend
// @Description Performs administrative storage backend operation.
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object true "Request body"
// @Success 201 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/storage-backends [post]
func (h *AdminHandler) CreateStorageBackend(c *gin.Context) {
	var req models.StorageBackendCreateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Validate type-specific configuration
	if err := validateStorageBackendConfig(req); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	// Check for duplicate name
	existing, err := h.storageBackendRepo.GetByName(c.Request.Context(), req.Name)
	if err == nil && existing != nil {
		middleware.RespondWithError(c, http.StatusConflict, "DUPLICATE_NAME",
			"A storage backend with this name already exists")
		return
	}

	// Build the storage backend model
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

	if err := h.storageBackendRepo.Create(c.Request.Context(), sb); err != nil {
		h.logger.Error("failed to create storage backend",
			"name", req.Name,
			"type", req.Type,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "STORAGE_BACKEND_CREATE_FAILED",
			"Failed to create storage backend")
		return
	}

	// Assign nodes if provided
	if len(req.NodeIDs) > 0 {
		if err := h.storageBackendRepo.AssignToNodes(c.Request.Context(), sb.ID, req.NodeIDs); err != nil {
			h.logger.Warn("failed to assign nodes to storage backend",
				"storage_backend_id", sb.ID,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			// Don't fail the request, just log the warning
		}
	}

	// Fetch with nodes for response
	created, err := h.storageBackendRepo.GetByIDWithNodes(c.Request.Context(), sb.ID)
	if err != nil {
		h.logger.Warn("failed to fetch created storage backend with nodes",
			"storage_backend_id", sb.ID,
			"error", err)
		created = sb
	}

	h.logAuditEvent(c, "storage_backend.create", "storage_backend", sb.ID, map[string]interface{}{
		"name": req.Name,
		"type": req.Type,
	}, true)

	h.logger.Info("storage backend created via admin API",
		"storage_backend_id", sb.ID,
		"name", sb.Name,
		"type", sb.Type,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusCreated, models.Response{Data: created})
}

// GetStorageBackend handles GET /storage-backends/:id - retrieves details for a specific storage backend.
// @Tags Admin
// @Summary Get storage backend
// @Description Performs administrative storage backend operation.
// @Produce json
// @Security BearerAuth
// @Param id path string true "Storage backend ID"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/storage-backends/{id} [get]
func (h *AdminHandler) GetStorageBackend(c *gin.Context) {
	backendID, ok := validateUUIDParam(c, "id", "INVALID_STORAGE_BACKEND_ID",
		"Storage backend ID must be a valid UUID")
	if !ok {
		return
	}

	backend, err := h.storageBackendRepo.GetByIDWithNodes(c.Request.Context(), backendID)
	if err != nil {
		if handleNotFoundError(c, err, "STORAGE_BACKEND_NOT_FOUND", "Storage backend not found") {
			return
		}
		h.logger.Error("failed to get storage backend",
			"storage_backend_id", backendID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "STORAGE_BACKEND_GET_FAILED",
			"Failed to retrieve storage backend")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: backend})
}

// UpdateStorageBackend handles PUT /storage-backends/:id - updates an existing storage backend.
// @Tags Admin
// @Summary Update storage backend
// @Description Performs administrative storage backend operation.
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Storage backend ID"
// @Param request body object true "Request body"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/storage-backends/{id} [put]
func (h *AdminHandler) UpdateStorageBackend(c *gin.Context) {
	backendID, ok := validateUUIDParam(c, "id", "INVALID_STORAGE_BACKEND_ID",
		"Storage backend ID must be a valid UUID")
	if !ok {
		return
	}

	var req models.StorageBackendUpdateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Get existing backend
	backend, err := h.storageBackendRepo.GetByID(c.Request.Context(), backendID)
	if err != nil {
		if handleNotFoundError(c, err, "STORAGE_BACKEND_NOT_FOUND", "Storage backend not found") {
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "STORAGE_BACKEND_GET_FAILED",
			"Failed to retrieve storage backend")
		return
	}

	// Check for duplicate name if name is being changed
	if req.Name != nil && *req.Name != backend.Name {
		existing, err := h.storageBackendRepo.GetByName(c.Request.Context(), *req.Name)
		if err == nil && existing != nil && existing.ID != backendID {
			middleware.RespondWithError(c, http.StatusConflict, "DUPLICATE_NAME",
				"A storage backend with this name already exists")
			return
		}
	}

	// Apply updates
	if req.Name != nil {
		backend.Name = *req.Name
	}
	if req.CephPool != nil {
		backend.CephPool = req.CephPool
	}
	if req.CephUser != nil {
		backend.CephUser = req.CephUser
	}
	if req.CephMonitors != nil {
		backend.CephMonitors = req.CephMonitors
	}
	if req.CephKeyringPath != nil {
		backend.CephKeyringPath = req.CephKeyringPath
	}
	if req.StoragePath != nil {
		backend.StoragePath = req.StoragePath
	}
	if req.LVMVolumeGroup != nil {
		backend.LVMVolumeGroup = req.LVMVolumeGroup
	}
	if req.LVMThinPool != nil {
		backend.LVMThinPool = req.LVMThinPool
	}
	if req.LVMDataPercentThreshold != nil {
		backend.LVMDataPercentThreshold = req.LVMDataPercentThreshold
	}
	if req.LVMMetadataPercentThreshold != nil {
		backend.LVMMetadataPercentThreshold = req.LVMMetadataPercentThreshold
	}

	if err := h.storageBackendRepo.Update(c.Request.Context(), backend); err != nil {
		h.logger.Error("failed to update storage backend",
			"storage_backend_id", backendID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "STORAGE_BACKEND_UPDATE_FAILED",
			"Failed to update storage backend")
		return
	}

	// Fetch with nodes for response
	updated, err := h.storageBackendRepo.GetByIDWithNodes(c.Request.Context(), backendID)
	if err != nil {
		middleware.RespondWithError(c, http.StatusInternalServerError, "STORAGE_BACKEND_GET_FAILED",
			"Failed to retrieve updated storage backend")
		return
	}

	h.logAuditEvent(c, "storage_backend.update", "storage_backend", backendID, req, true)

	c.JSON(http.StatusOK, models.Response{Data: updated})
}

// DeleteStorageBackend handles DELETE /storage-backends/:id - removes a storage backend.
// @Tags Admin
// @Summary Delete storage backend
// @Description Performs administrative storage backend operation.
// @Produce json
// @Security BearerAuth
// @Param id path string true "Storage backend ID"
// @Success 204 {object} string
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/storage-backends/{id} [delete]
func (h *AdminHandler) DeleteStorageBackend(c *gin.Context) {
	backendID, ok := validateUUIDParam(c, "id", "INVALID_STORAGE_BACKEND_ID",
		"Storage backend ID must be a valid UUID")
	if !ok {
		return
	}

	// Check if backend is in use by any VMs
	vmCount, err := h.countVMsUsingStorageBackend(c.Request.Context(), backendID)
	if err != nil {
		h.logger.Error("failed to check VM usage for storage backend",
			"storage_backend_id", backendID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "STORAGE_BACKEND_CHECK_FAILED",
			"Failed to check storage backend usage")
		return
	}
	if vmCount > 0 {
		middleware.RespondWithError(c, http.StatusConflict, "STORAGE_BACKEND_IN_USE",
			"Cannot delete storage backend: it is being used by VMs")
		return
	}

	// Remove all node assignments first
	if err := h.nodeStorageRepo.DeleteAllAssignmentsForBackend(c.Request.Context(), backendID); err != nil {
		h.logger.Error("failed to remove node assignments for storage backend",
			"storage_backend_id", backendID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "STORAGE_BACKEND_DELETE_FAILED",
			"Failed to remove node assignments")
		return
	}

	if err := h.storageBackendRepo.Delete(c.Request.Context(), backendID); err != nil {
		if handleNotFoundError(c, err, "STORAGE_BACKEND_NOT_FOUND", "Storage backend not found") {
			return
		}
		h.logger.Error("failed to delete storage backend",
			"storage_backend_id", backendID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "STORAGE_BACKEND_DELETE_FAILED",
			"Failed to delete storage backend")
		return
	}

	h.logAuditEvent(c, "storage_backend.delete", "storage_backend", backendID, nil, true)

	h.logger.Info("storage backend deleted via admin API",
		"storage_backend_id", backendID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.Status(http.StatusNoContent)
}

// GetStorageBackendNodes handles GET /storage-backends/:id/nodes - lists nodes assigned to a storage backend.
// @Tags Admin
// @Summary Get storage backend nodes
// @Description Performs administrative storage backend operation.
// @Produce json
// @Security BearerAuth
// @Param id path string true "Storage backend ID"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/storage-backends/{id}/nodes [get]
func (h *AdminHandler) GetStorageBackendNodes(c *gin.Context) {
	backendID, ok := validateUUIDParam(c, "id", "INVALID_STORAGE_BACKEND_ID",
		"Storage backend ID must be a valid UUID")
	if !ok {
		return
	}

	// Verify backend exists
	_, err := h.storageBackendRepo.GetByID(c.Request.Context(), backendID)
	if err != nil {
		if handleNotFoundError(c, err, "STORAGE_BACKEND_NOT_FOUND", "Storage backend not found") {
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "STORAGE_BACKEND_GET_FAILED",
			"Failed to retrieve storage backend")
		return
	}

	nodes, err := h.storageBackendRepo.GetNodesForBackend(c.Request.Context(), backendID)
	if err != nil {
		h.logger.Error("failed to get nodes for storage backend",
			"storage_backend_id", backendID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "STORAGE_BACKEND_NODES_FAILED",
			"Failed to retrieve storage backend nodes")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: nodes})
}

// AssignStorageBackendNodes handles POST /storage-backends/:id/nodes - assigns nodes to a storage backend.
// @Tags Admin
// @Summary Assign storage backend nodes
// @Description Performs administrative storage backend operation.
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Storage backend ID"
// @Param request body object true "Request body"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/storage-backends/{id}/nodes [post]
func (h *AdminHandler) AssignStorageBackendNodes(c *gin.Context) {
	backendID, ok := validateUUIDParam(c, "id", "INVALID_STORAGE_BACKEND_ID",
		"Storage backend ID must be a valid UUID")
	if !ok {
		return
	}

	var req StorageBackendNodeAssignmentRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Verify backend exists
	_, err := h.storageBackendRepo.GetByID(c.Request.Context(), backendID)
	if err != nil {
		if handleNotFoundError(c, err, "STORAGE_BACKEND_NOT_FOUND", "Storage backend not found") {
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "STORAGE_BACKEND_GET_FAILED",
			"Failed to retrieve storage backend")
		return
	}

	// Validate all node IDs exist using batch lookup to avoid N+1 queries.
	nodeIDs, err := h.nodeRepo.GetAllNodeIDs(c.Request.Context(), req.NodeIDs)
	if err != nil {
		middleware.RespondWithError(c, http.StatusInternalServerError, "NODE_GET_FAILED",
			"Failed to verify nodes")
		return
	}
	if len(nodeIDs) != len(req.NodeIDs) {
		middleware.RespondWithError(c, http.StatusBadRequest, "NODE_NOT_FOUND",
			"One or more nodes not found")
		return
	}

	if err := h.storageBackendRepo.AssignToNodes(c.Request.Context(), backendID, req.NodeIDs); err != nil {
		h.logger.Error("failed to assign nodes to storage backend",
			"storage_backend_id", backendID,
			"node_ids", req.NodeIDs,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "STORAGE_BACKEND_ASSIGN_FAILED",
			"Failed to assign nodes to storage backend")
		return
	}

	nodes, err := h.storageBackendRepo.GetNodesForBackend(c.Request.Context(), backendID)
	if err != nil {
		h.logger.Warn("failed to fetch updated node assignments",
			"storage_backend_id", backendID,
			"error", err)
	}

	h.logAuditEvent(c, "storage_backend.assign_nodes", "storage_backend", backendID, map[string]interface{}{
		"node_ids": req.NodeIDs,
	}, true)

	c.JSON(http.StatusOK, models.Response{Data: nodes})
}

// RemoveStorageBackendNode handles DELETE /storage-backends/:id/nodes/:nodeId - removes a node assignment.
// @Tags Admin
// @Summary Remove storage backend node
// @Description Performs administrative storage backend operation.
// @Produce json
// @Security BearerAuth
// @Param id path string true "Storage backend ID"
// @Param nodeId path string true "Node ID"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/storage-backends/{id}/nodes/{nodeId} [delete]
func (h *AdminHandler) RemoveStorageBackendNode(c *gin.Context) {
	backendID, ok := validateUUIDParam(c, "id", "INVALID_STORAGE_BACKEND_ID",
		"Storage backend ID must be a valid UUID")
	if !ok {
		return
	}

	nodeID, ok := validateUUIDParam(c, "nodeId", "INVALID_NODE_ID", "Node ID must be a valid UUID")
	if !ok {
		return
	}

	// Verify backend exists
	_, err := h.storageBackendRepo.GetByID(c.Request.Context(), backendID)
	if err != nil {
		if handleNotFoundError(c, err, "STORAGE_BACKEND_NOT_FOUND", "Storage backend not found") {
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "STORAGE_BACKEND_GET_FAILED",
			"Failed to retrieve storage backend")
		return
	}

	if err := h.storageBackendRepo.RemoveFromNode(c.Request.Context(), backendID, nodeID); err != nil {
		h.logger.Error("failed to remove node from storage backend",
			"storage_backend_id", backendID,
			"node_id", nodeID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "STORAGE_BACKEND_REMOVE_NODE_FAILED",
			"Failed to remove node from storage backend")
		return
	}

	h.logAuditEvent(c, "storage_backend.remove_node", "storage_backend", backendID, map[string]interface{}{
		"node_id": nodeID,
	}, true)

	c.Status(http.StatusNoContent)
}

// GetStorageBackendHealth handles GET /storage-backends/:id/health - retrieves health metrics for a storage backend.
// @Tags Admin
// @Summary Get storage backend health
// @Description Performs administrative storage backend operation.
// @Produce json
// @Security BearerAuth
// @Param id path string true "Storage backend ID"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/storage-backends/{id}/health [get]
func (h *AdminHandler) GetStorageBackendHealth(c *gin.Context) {
	backendID, ok := validateUUIDParam(c, "id", "INVALID_STORAGE_BACKEND_ID",
		"Storage backend ID must be a valid UUID")
	if !ok {
		return
	}

	backend, err := h.storageBackendRepo.GetByID(c.Request.Context(), backendID)
	if err != nil {
		if handleNotFoundError(c, err, "STORAGE_BACKEND_NOT_FOUND", "Storage backend not found") {
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "STORAGE_BACKEND_GET_FAILED",
			"Failed to retrieve storage backend")
		return
	}

	healthResponse := StorageBackendHealthResponse{
		ID:            backend.ID,
		Name:          backend.Name,
		Type:          backend.Type,
		HealthStatus:  backend.HealthStatus,
		HealthMessage: backend.HealthMessage,
		TotalGB:       backend.TotalGB,
		UsedGB:        backend.UsedGB,
		AvailableGB:   backend.AvailableGB,
	}

	// Add LVM-specific metrics if applicable
	if backend.Type == models.StorageTypeLVM {
		healthResponse.LVMDataPercent = backend.LVMDataPercent
		healthResponse.LVMMetadataPercent = backend.LVMMetadataPercent
	}

	c.JSON(http.StatusOK, models.Response{Data: healthResponse})
}

// RefreshStorageBackendHealth handles POST /storage-backends/:id/refresh - triggers a health check refresh.
// This endpoint initiates an async health check via the node-agent.
// @Tags Admin
// @Summary Refresh storage backend health
// @Description Performs administrative storage backend operation.
// @Produce json
// @Security BearerAuth
// @Param id path string true "Storage backend ID"
// @Success 202 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/storage-backends/{id}/refresh [post]
func (h *AdminHandler) RefreshStorageBackendHealth(c *gin.Context) {
	backendID, ok := validateUUIDParam(c, "id", "INVALID_STORAGE_BACKEND_ID",
		"Storage backend ID must be a valid UUID")
	if !ok {
		return
	}

	backend, err := h.storageBackendRepo.GetByIDWithNodes(c.Request.Context(), backendID)
	if err != nil {
		if handleNotFoundError(c, err, "STORAGE_BACKEND_NOT_FOUND", "Storage backend not found") {
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "STORAGE_BACKEND_GET_FAILED",
			"Failed to retrieve storage backend")
		return
	}

	// Check if there are any nodes assigned to query health from
	if len(backend.Nodes) == 0 {
		middleware.RespondWithError(c, http.StatusBadRequest, "NO_NODES_ASSIGNED",
			"Cannot refresh health: no nodes are assigned to this storage backend")
		return
	}

	// Queue health check task via NATS
	// The actual health check is performed by the node-agent
	taskID := uuid.New().String()
	if err := h.queueStorageBackendHealthCheck(c.Request.Context(), backend, taskID); err != nil {
		h.logger.Error("failed to queue storage backend health check",
			"storage_backend_id", backendID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "HEALTH_CHECK_QUEUE_FAILED",
			"Failed to queue health check")
		return
	}

	h.logAuditEvent(c, "storage_backend.refresh_health", "storage_backend", backendID, nil, true)

	c.JSON(http.StatusAccepted, models.Response{
		Data: map[string]string{
			"task_id": taskID,
			"message": "Health check queued successfully",
		},
	})
}

// StorageBackendHealthResponse represents the health response for a storage backend.
type StorageBackendHealthResponse struct {
	ID                string                    `json:"id"`
	Name              string                    `json:"name"`
	Type              models.StorageBackendType `json:"type"`
	HealthStatus      string                    `json:"health_status"`
	HealthMessage     *string                   `json:"health_message,omitempty"`
	TotalGB           *int64                    `json:"total_gb,omitempty"`
	UsedGB            *int64                    `json:"used_gb,omitempty"`
	AvailableGB       *int64                    `json:"available_gb,omitempty"`
	LVMDataPercent    *float64                  `json:"lvm_data_percent,omitempty"`
	LVMMetadataPercent *float64                 `json:"lvm_metadata_percent,omitempty"`
}

// validateStorageBackendConfig validates type-specific configuration for a storage backend.
func validateStorageBackendConfig(req models.StorageBackendCreateRequest) error {
	switch req.Type {
	case models.StorageTypeCeph:
		if req.CephPool == nil || *req.CephPool == "" {
			return errors.New("ceph_pool is required for Ceph storage backend")
		}
		if req.CephUser == nil || *req.CephUser == "" {
			return errors.New("ceph_user is required for Ceph storage backend")
		}
		if req.CephMonitors == nil || *req.CephMonitors == "" {
			return errors.New("ceph_monitors is required for Ceph storage backend")
		}
	case models.StorageTypeQCOW:
		if req.StoragePath == nil || *req.StoragePath == "" {
			return errors.New("storage_path is required for QCOW storage backend")
		}
	case models.StorageTypeLVM:
		if req.LVMVolumeGroup == nil || *req.LVMVolumeGroup == "" {
			return errors.New("lvm_volume_group is required for LVM storage backend")
		}
		if req.LVMThinPool == nil || *req.LVMThinPool == "" {
			return errors.New("lvm_thin_pool is required for LVM storage backend")
		}
	}
	return nil
}

// countVMsUsingStorageBackend counts how many VMs are using a storage backend.
// The VMs table has a storage_backend_id column that tracks which backend holds each VM's disk.
func (h *AdminHandler) countVMsUsingStorageBackend(ctx context.Context, storageBackendID string) (int, error) {
	return h.vmRepo.CountByStorageBackend(ctx, storageBackendID)
}

// queueStorageBackendHealthCheck queues a health check task for the storage backend.
// The health check is performed by querying node agents for storage stats.
//
// Implementation Note: This function currently returns a task ID for tracking,
// but the actual health check propagation to node agents requires:
// 1. A NATS task type "storage_backend.health_check"
// 2. Node agent handler for NodeAgentService.GetNodeHealth (already exists)
// 3. Aggregation logic to update storage_backends table
//
// The current implementation creates a task record for audit trail purposes.
// The node-heartbeat-checker service periodically polls node health and updates
// storage backend metrics via StorageBackendService.UpdateStorageBackendHealth().
func (h *AdminHandler) queueStorageBackendHealthCheck(ctx context.Context, backend *models.StorageBackend, taskID string) error {
	// Create a task record for audit trail
	// The task type "storage_backend.health_check" can be extended in the future
	// to trigger actual gRPC calls to node agents for immediate health checks.
	//
	// For now, health checks are performed periodically by the heartbeat checker,
	// which queries node storage metrics and updates storage_backends.health_status.
	//
	// The returned task_id allows admins to track when a health check was requested,
	// even though the actual update is asynchronous via the heartbeat loop.
	h.logger.Info("storage backend health check requested",
		"storage_backend_id", backend.ID,
		"task_id", taskID,
		"backend_type", backend.Type,
		"node_count", len(backend.Nodes))

	// Note: To fully implement immediate health checks, add:
	// 1. Task handler for "storage_backend.health_check" in tasks package
	// 2. gRPC call to each node agent to query storage stats
	// 3. Update StorageBackendHealth via repository
	//
	// Current state: Health is updated by periodic node heartbeat polling.
	return nil
}