package admin

import (
	"errors"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// NodeUpdateRequest represents the request body for updating a node.
type NodeUpdateRequest struct {
	GRPCAddress    *string `json:"grpc_address,omitempty" validate:"omitempty,max=255"`
	LocationID     *string `json:"location_id,omitempty" validate:"omitempty,uuid"`
	TotalVCPU      *int    `json:"total_vcpu,omitempty" validate:"omitempty,min=1"`
	TotalMemory    *int    `json:"total_memory_mb,omitempty" validate:"omitempty,min=1024"`
	IPMIAddress    *string `json:"ipmi_address,omitempty" validate:"omitempty,ip"`
	StorageBackend *string `json:"storage_backend,omitempty" validate:"omitempty,oneof=ceph qcow"`
	StoragePath    *string `json:"storage_path,omitempty" validate:"omitempty,max=500"`
}

// validateStorageConfig validates storage_backend and storage_path consistency.
func validateStorageConfig(storageBackend, storagePath string) error {
	if storageBackend == models.StorageBackendQcow && storagePath == "" {
		return errors.New("storage_path is required when storage_backend is 'qcow'")
	}
	return nil
}

// ListNodes handles GET /nodes - lists all hypervisor nodes with optional filtering.
func (h *AdminHandler) ListNodes(c *gin.Context) {
	pagination := models.ParsePagination(c)

	filter := models.NodeListFilter{
		PaginationParams: pagination,
	}

	// Optional status filter
	if status := c.Query("status"); status != "" {
		filter.Status = &status
	}

	// Optional location filter
	if locationID := c.Query("location_id"); locationID != "" {
		filter.LocationID = &locationID
	}

	nodes, total, err := h.nodeService.ListNode(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list nodes",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "NODE_LIST_FAILED", "Failed to retrieve nodes")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: nodes,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}

// RegisterNode handles POST /nodes - registers a new hypervisor node.
func (h *AdminHandler) RegisterNode(c *gin.Context) {
	var req models.NodeCreateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	if err := validateStorageConfig(req.StorageBackend, req.StoragePath); err != nil {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	node, err := h.nodeService.RegisterNode(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("failed to register node",
			"hostname", req.Hostname,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "NODE_REGISTER_FAILED", err.Error())
		return
	}

	// Log audit event
	h.logAuditEvent(c, "node.create", "node", node.ID, map[string]interface{}{
		"hostname":     node.Hostname,
		"grpc_address": node.GRPCAddress,
		"location_id":  node.LocationID,
	}, true)

	h.logger.Info("node registered via admin API",
		"node_id", node.ID,
		"hostname", node.Hostname,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusCreated, models.Response{Data: node})
}

// GetNode handles GET /nodes/:id - retrieves details for a specific node.
func (h *AdminHandler) GetNode(c *gin.Context) {
	nodeID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(nodeID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_NODE_ID", "Node ID must be a valid UUID")
		return
	}

	node, err := h.nodeService.GetNode(c.Request.Context(), nodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "NODE_NOT_FOUND", "Node not found")
			return
		}
		h.logger.Error("failed to get node",
			"node_id", nodeID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "NODE_GET_FAILED", "Failed to retrieve node")
		return
	}

	// Get node status with metrics
	status, err := h.nodeService.GetNodeStatus(c.Request.Context(), nodeID)
	if err != nil {
		h.logger.Warn("failed to get node status, returning basic node info",
			"node_id", nodeID,
			"error", err)
		c.JSON(http.StatusOK, models.Response{Data: node})
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: status})
}

// UpdateNode handles PUT /nodes/:id - updates an existing node's configuration.
func (h *AdminHandler) UpdateNode(c *gin.Context) {
	nodeID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(nodeID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_NODE_ID", "Node ID must be a valid UUID")
		return
	}

	var req NodeUpdateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Get existing node
	node, err := h.nodeService.GetNode(c.Request.Context(), nodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "NODE_NOT_FOUND", "Node not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "NODE_GET_FAILED", "Failed to retrieve node")
		return
	}

	if req.StorageBackend != nil || req.StoragePath != nil {
		currentBackend := node.StorageBackend
		currentPath := node.StoragePath
		if req.StorageBackend != nil {
			currentBackend = *req.StorageBackend
		}
		if req.StoragePath != nil {
			currentPath = *req.StoragePath
		}
		if err := validateStorageConfig(currentBackend, currentPath); err != nil {
			respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
	}

	// Apply updates
	if req.GRPCAddress != nil {
		node.GRPCAddress = *req.GRPCAddress
	}
	if req.LocationID != nil {
		node.LocationID = req.LocationID
	}
	if req.TotalVCPU != nil {
		node.TotalVCPU = *req.TotalVCPU
	}
	if req.TotalMemory != nil {
		node.TotalMemoryMB = *req.TotalMemory
	}
	if req.IPMIAddress != nil {
		node.IPMIAddress = req.IPMIAddress
	}
	if req.StorageBackend != nil {
		node.StorageBackend = *req.StorageBackend
	}
	if req.StoragePath != nil {
		node.StoragePath = *req.StoragePath
	}

	if err := h.nodeService.UpdateNode(c.Request.Context(), node); err != nil {
		h.logger.Error("failed to update node",
			"node_id", nodeID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "NODE_UPDATE_FAILED", err.Error())
		return
	}

	updatedNode, err := h.nodeService.GetNode(c.Request.Context(), nodeID)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "NODE_GET_FAILED", "Failed to retrieve updated node")
		return
	}

	// Log audit event
	h.logAuditEvent(c, "node.update", "node", nodeID, req, true)

	c.JSON(http.StatusOK, models.Response{Data: updatedNode})
}

// DeleteNode handles DELETE /nodes/:id - permanently removes a node.
func (h *AdminHandler) DeleteNode(c *gin.Context) {
	nodeID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(nodeID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_NODE_ID", "Node ID must be a valid UUID")
		return
	}

	err := h.nodeService.DeleteNode(c.Request.Context(), nodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "NODE_NOT_FOUND", "Node not found")
			return
		}
		h.logger.Error("failed to delete node",
			"node_id", nodeID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "NODE_DELETE_FAILED", err.Error())
		return
	}

	// Log audit event
	h.logAuditEvent(c, "node.delete", "node", nodeID, nil, true)

	h.logger.Info("node deleted via admin API",
		"node_id", nodeID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"deleted": true}})
}

// DrainNode handles POST /nodes/:id/drain - sets a node to draining mode.
// Draining prevents new VM placements while allowing existing VMs to run.
func (h *AdminHandler) DrainNode(c *gin.Context) {
	nodeID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(nodeID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_NODE_ID", "Node ID must be a valid UUID")
		return
	}

	err := h.nodeService.DrainNode(c.Request.Context(), nodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "NODE_NOT_FOUND", "Node not found")
			return
		}
		h.logger.Error("failed to drain node",
			"node_id", nodeID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "NODE_DRAIN_FAILED", err.Error())
		return
	}

	// Log audit event
	h.logAuditEvent(c, "node.drain", "node", nodeID, nil, true)

	h.logger.Info("node set to draining mode via admin API",
		"node_id", nodeID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"status": "draining"}})
}

// FailoverNode handles POST /nodes/:id/failover - marks a node as failed.
// This triggers alerting and potentially automatic VM migration.
func (h *AdminHandler) FailoverNode(c *gin.Context) {
	nodeID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(nodeID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_NODE_ID", "Node ID must be a valid UUID")
		return
	}

	err := h.nodeService.FailoverNode(c.Request.Context(), nodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "NODE_NOT_FOUND", "Node not found")
			return
		}
		h.logger.Error("failed to failover node",
			"node_id", nodeID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "NODE_FAILOVER_FAILED", err.Error())
		return
	}

	// Log audit event
	h.logAuditEvent(c, "node.failover", "node", nodeID, nil, true)

	h.logger.Warn("node marked as failed via admin API",
		"node_id", nodeID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"status": "failed"}})
}

func (h *AdminHandler) UndrainNode(c *gin.Context) {
	nodeID := c.Param("id")

	if _, err := uuid.Parse(nodeID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_NODE_ID", "Node ID must be a valid UUID")
		return
	}

	err := h.nodeService.UndrainNode(c.Request.Context(), nodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "NODE_NOT_FOUND", "Node not found")
			return
		}
		h.logger.Error("failed to undrain node",
			"node_id", nodeID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "NODE_UNDRAIN_FAILED", err.Error())
		return
	}

	h.logAuditEvent(c, "node.undrain", "node", nodeID, nil, true)

	h.logger.Info("node restored to online mode via admin API",
		"node_id", nodeID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"status": "online"}})
}
