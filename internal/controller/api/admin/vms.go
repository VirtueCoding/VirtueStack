package admin

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// AdminVMCreateRequest represents the request body for admin VM creation.
// Admins can create VMs for any customer.
type AdminVMCreateRequest struct {
	CustomerID string   `json:"customer_id" validate:"required,uuid"`
	PlanID     string   `json:"plan_id" validate:"required,uuid"`
	TemplateID string   `json:"template_id" validate:"required,uuid"`
	Hostname   string   `json:"hostname" validate:"required,hostname_rfc1123,max=63"`
	Password   string   `json:"password" validate:"required,min=8,max=128"`
	SSHKeys    []string `json:"ssh_keys,omitempty" validate:"max=10,dive,max=4096"`
	LocationID *string  `json:"location_id,omitempty" validate:"omitempty,uuid"`
	NodeID     *string  `json:"node_id,omitempty" validate:"omitempty,uuid"` // Force specific node
}

// AdminVMUpdateRequest represents the request body for updating a VM.
type AdminVMUpdateRequest struct {
	Hostname         *string `json:"hostname,omitempty" validate:"omitempty,hostname_rfc1123,max=63"`
	VCPU             *int    `json:"vcpu,omitempty" validate:"omitempty,min=1"`
	MemoryMB         *int    `json:"memory_mb,omitempty" validate:"omitempty,min=512"`
	DiskGB           *int    `json:"disk_gb,omitempty" validate:"omitempty,min=10"`
	PortSpeedMbps    *int    `json:"port_speed_mbps,omitempty" validate:"omitempty,min=1"`
	BandwidthLimitGB *int    `json:"bandwidth_limit_gb,omitempty" validate:"omitempty,min=0"`
}

// VMMigrateRequest represents the request body for VM migration.
type VMMigrateRequest struct {
	TargetNodeID string `json:"target_node_id" validate:"required,uuid"`
}

// ListVMs handles GET /vms - lists all VMs across all customers.
func (h *AdminHandler) ListVMs(c *gin.Context) {
	pagination := models.ParsePagination(c)

	filter := models.VMListFilter{
		PaginationParams: pagination,
	}

	// Optional customer filter
	if customerID := c.Query("customer_id"); customerID != "" {
		filter.CustomerID = &customerID
	}

	// Optional node filter
	if nodeID := c.Query("node_id"); nodeID != "" {
		filter.NodeID = &nodeID
	}

	// Optional status filter
	if status := c.Query("status"); status != "" {
		filter.Status = &status
	}

	// Optional search filter
	if search := c.Query("search"); search != "" {
		filter.Search = &search
	}

	// isAdmin=true allows viewing all VMs
	vms, total, err := h.vmService.ListVMs(c.Request.Context(), filter, "", true)
	if err != nil {
		h.logger.Error("failed to list VMs",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "VM_LIST_FAILED", "Failed to retrieve VMs")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: vms,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}

// CreateVM handles POST /vms - creates a VM for any customer (admin override).
func (h *AdminHandler) CreateVM(c *gin.Context) {
	var req AdminVMCreateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Validate UUIDs
	if _, err := uuid.Parse(req.CustomerID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_CUSTOMER_ID", "Customer ID must be a valid UUID")
		return
	}
	if _, err := uuid.Parse(req.PlanID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_PLAN_ID", "Plan ID must be a valid UUID")
		return
	}
	if _, err := uuid.Parse(req.TemplateID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_TEMPLATE_ID", "Template ID must be a valid UUID")
		return
	}

	// Build VM create request
	vmReq := &models.VMCreateRequest{
		CustomerID: req.CustomerID,
		PlanID:     req.PlanID,
		TemplateID: req.TemplateID,
		Hostname:   req.Hostname,
		Password:   req.Password,
		SSHKeys:    req.SSHKeys,
		LocationID: req.LocationID,
	}

	// Create VM through service layer (isAdmin=true for admin override)
	vm, taskID, err := h.vmService.CreateVM(c.Request.Context(), vmReq, req.CustomerID)
	if err != nil {
		h.logger.Error("failed to create VM via admin API",
			"customer_id", req.CustomerID,
			"hostname", req.Hostname,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "VM_CREATE_FAILED", err.Error())
		return
	}

	// Log audit event
	h.logAuditEvent(c, "vm.create", "vm", vm.ID, map[string]interface{}{
		"customer_id": req.CustomerID,
		"plan_id":     req.PlanID,
		"hostname":    req.Hostname,
	}, true)

	h.logger.Info("VM creation initiated via admin API",
		"vm_id", vm.ID,
		"task_id", taskID,
		"customer_id", req.CustomerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusAccepted, models.Response{
		Data: gin.H{
			"vm_id":   vm.ID,
			"task_id": taskID,
		},
	})
}

// GetVM handles GET /vms/:id - retrieves details for any VM.
func (h *AdminHandler) GetVM(c *gin.Context) {
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// isAdmin=true allows viewing any VM
	vm, err := h.vmService.GetVMDetail(c.Request.Context(), vmID, "", true)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to get VM",
			"vm_id", vmID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "VM_GET_FAILED", "Failed to retrieve VM")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: vm})
}

// UpdateVM handles PUT /vms/:id - updates VM configuration.
// Admins can modify any VM's configuration.
func (h *AdminHandler) UpdateVM(c *gin.Context) {
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	var req AdminVMUpdateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Get existing VM
	vm, err := h.vmService.GetVM(c.Request.Context(), vmID, "", true)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "VM_GET_FAILED", "Failed to retrieve VM")
		return
	}

	// Handle resource resize if specified
	if req.VCPU != nil || req.MemoryMB != nil || req.DiskGB != nil {
		newVCPU := vm.VCPU
		newMemory := vm.MemoryMB
		newDisk := vm.DiskGB

		if req.VCPU != nil {
			newVCPU = *req.VCPU
		}
		if req.MemoryMB != nil {
			newMemory = *req.MemoryMB
		}
		if req.DiskGB != nil {
			newDisk = *req.DiskGB
		}

		// isAdmin=true allows exceeding plan limits
		err = h.vmService.ResizeVM(c.Request.Context(), vmID, "", newVCPU, newMemory, newDisk, true)
		if err != nil {
			h.logger.Error("failed to resize VM",
				"vm_id", vmID,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			respondWithError(c, http.StatusInternalServerError, "VM_RESIZE_FAILED", err.Error())
			return
		}
	}

	// Log audit event
	h.logAuditEvent(c, "vm.update", "vm", vmID, req, true)

	h.logger.Info("VM updated via admin API",
		"vm_id", vmID,
		"correlation_id", middleware.GetCorrelationID(c))

	// Return updated VM
	updatedVM, _ := h.vmService.GetVMDetail(c.Request.Context(), vmID, "", true)
	c.JSON(http.StatusOK, models.Response{Data: updatedVM})
}

// DeleteVM handles DELETE /vms/:id - deletes any VM.
func (h *AdminHandler) DeleteVM(c *gin.Context) {
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// isAdmin=true allows deleting any VM
	taskID, err := h.vmService.DeleteVM(c.Request.Context(), vmID, "", true)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to delete VM",
			"vm_id", vmID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "VM_DELETE_FAILED", err.Error())
		return
	}

	// Log audit event
	h.logAuditEvent(c, "vm.delete", "vm", vmID, nil, true)

	h.logger.Info("VM deletion initiated via admin API",
		"vm_id", vmID,
		"task_id", taskID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusAccepted, models.Response{
		Data: TaskResponse{TaskID: taskID},
	})
}

// MigrateVM handles POST /vms/:id/migrate - migrates a VM to another node.
func (h *AdminHandler) MigrateVM(c *gin.Context) {
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	var req VMMigrateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Validate target node UUID
	if _, err := uuid.Parse(req.TargetNodeID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_NODE_ID", "Target Node ID must be a valid UUID")
		return
	}

	// Get VM to verify it exists
	vm, err := h.vmService.GetVM(c.Request.Context(), vmID, "", true)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "VM_GET_FAILED", "Failed to retrieve VM")
		return
	}

	// Verify target node exists
	_, err = h.nodeService.GetNode(c.Request.Context(), req.TargetNodeID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusBadRequest, "NODE_NOT_FOUND", "Target node not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "NODE_GET_FAILED", "Failed to retrieve target node")
		return
	}

	// Log audit event
	h.logAuditEvent(c, "vm.migrate", "vm", vmID, map[string]interface{}{
		"source_node_id": vm.NodeID,
		"target_node_id": req.TargetNodeID,
	}, true)

	h.logger.Info("VM migration requested via admin API",
		"vm_id", vmID,
		"target_node_id", req.TargetNodeID,
		"correlation_id", middleware.GetCorrelationID(c))

	result, err := h.migrationService.MigrateVM(c.Request.Context(), &services.MigrateVMRequest{
		VMID:         vmID,
		TargetNodeID: &req.TargetNodeID,
		Live:         true,
	}, middleware.GetUserID(c))
	if err != nil {
		h.logger.Error("failed to initiate migration",
			"vm_id", vmID,
			"target_node_id", req.TargetNodeID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "MIGRATION_FAILED", err.Error())
		return
	}

	c.JSON(http.StatusAccepted, models.Response{
		Data: gin.H{
			"vm_id":          vmID,
			"target_node_id": req.TargetNodeID,
			"task_id":        result.TaskID,
			"status":         "migration_initiated",
		},
	})
}
