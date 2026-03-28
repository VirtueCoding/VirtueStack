package admin

import (
	"errors"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// AdminVMCreateRequest represents the request body for admin VM creation.
// Admins can create VMs for any customer.
type AdminVMCreateRequest struct {
	CustomerID string   `json:"customer_id" validate:"required,uuid"`
	PlanID     string   `json:"plan_id" validate:"required,uuid"`
	TemplateID string   `json:"template_id" validate:"required,uuid"`
	Hostname   string   `json:"hostname" validate:"required,hostname_rfc1123,max=63"`
	Password   string   `json:"password" validate:"required,min=12,max=128"`
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
// @Tags Admin
// @Summary List VMs
// @Description Performs administrative VM management operation.
// @Produce json
// @Security BearerAuth
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/vms [get]
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
	validVMStatuses := map[string]bool{
		"provisioning": true, "running": true, "stopped": true, "suspended": true,
		"migrating": true, "reinstalling": true, "error": true, "deleted": true,
	}
	if status := c.Query("status"); status != "" {
		if !validVMStatuses[status] {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_STATUS", "Invalid status value")
			return
		}
		filter.Status = &status
	}

	// Optional search filter (bounded to 100 chars per QG-05)
	if search := c.Query("search"); search != "" {
		if len(search) > 100 {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_SEARCH", "search parameter must not exceed 100 characters")
			return
		}
		filter.Search = &search
	}

	// isAdmin=true allows viewing all VMs
	vms, total, err := h.vmService.ListVMs(c.Request.Context(), filter, "", true)
	if err != nil {
		h.logger.Error("failed to list VMs",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "VM_LIST_FAILED", "Failed to retrieve VMs")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: vms,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}

// CreateVM handles POST /vms - creates a VM for any customer (admin override).
// @Tags Admin
// @Summary Create VM
// @Description Performs administrative VM management operation.
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object true "Request body"
// @Success 202 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/vms [post]
func (h *AdminHandler) CreateVM(c *gin.Context) {
	var req AdminVMCreateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
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
		middleware.RespondWithError(c, http.StatusInternalServerError, "VM_CREATE_FAILED", "Internal server error")
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
		Data: VMCreateResponse{
			VMID:  vm.ID,
			TaskID: taskID,
		},
	})
}

// GetVM handles GET /vms/:id - retrieves details for any VM.
// @Tags Admin
// @Summary Get VM
// @Description Performs administrative VM management operation.
// @Produce json
// @Security BearerAuth
// @Param id path string true "VM ID"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/vms/{id} [get]
func (h *AdminHandler) GetVM(c *gin.Context) {
	vmID, ok := validateUUIDParam(c, "id", "INVALID_VM_ID", "VM ID must be a valid UUID")
	if !ok {
		return
	}

	// isAdmin=true allows viewing any VM
	vm, err := h.vmService.GetVMDetail(c.Request.Context(), vmID, "", true)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to get VM",
			"vm_id", vmID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "VM_GET_FAILED", "Failed to retrieve VM")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: vm})
}

// UpdateVM handles PUT /vms/:id - updates VM configuration.
// Admins can modify any VM's configuration.
// @Tags Admin
// @Summary Update VM
// @Description Performs administrative VM management operation.
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "VM ID"
// @Param request body object true "Request body"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/vms/{id} [put]
func (h *AdminHandler) UpdateVM(c *gin.Context) {
	vmID, ok := validateUUIDParam(c, "id", "INVALID_VM_ID", "VM ID must be a valid UUID")
	if !ok {
		return
	}

	var req AdminVMUpdateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Get existing VM
	vm, err := h.vmService.GetVM(c.Request.Context(), vmID, "", true)
	if err != nil {
		if handleNotFoundError(c, err, "VM_NOT_FOUND", "VM not found") {
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "VM_GET_FAILED", "Failed to retrieve VM")
		return
	}

	// Handle hostname update if specified
	if req.Hostname != nil {
		if err := h.vmService.UpdateVMHostname(c.Request.Context(), vmID, *req.Hostname, "", true); err != nil {
			h.logger.Error("failed to update VM hostname",
				"vm_id", vmID,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			middleware.RespondWithError(c, http.StatusInternalServerError, "VM_UPDATE_FAILED", "Internal server error")
			return
		}
	}

	// Handle network limit update if specified
	if req.PortSpeedMbps != nil || req.BandwidthLimitGB != nil {
		portSpeed := vm.PortSpeedMbps
		bwLimit := vm.BandwidthLimitGB
		if req.PortSpeedMbps != nil {
			portSpeed = *req.PortSpeedMbps
		}
		if req.BandwidthLimitGB != nil {
			bwLimit = *req.BandwidthLimitGB
		}
		if err := h.vmService.UpdateVMNetworkLimits(c.Request.Context(), vmID, portSpeed, bwLimit); err != nil {
			h.logger.Error("failed to update VM network limits",
				"vm_id", vmID,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			middleware.RespondWithError(c, http.StatusInternalServerError, "VM_UPDATE_FAILED", "Internal server error")
			return
		}
	}

	// Handle resource resize if specified
	if taskID, handled := h.handleVMResize(c, vmID, vm, req); handled {
		h.logAuditEvent(c, "vm.update", "vm", vmID, req, true)
		c.JSON(http.StatusAccepted, models.Response{
			Data: VMResizeResponse{
				TaskID:   taskID,
				VMID:     vmID,
				VCPU:     vm.VCPU,
				MemoryMB: vm.MemoryMB,
				DiskGB:   vm.DiskGB,
			},
		})
		return
	}

	h.logAuditEvent(c, "vm.update", "vm", vmID, req, true)

	h.logger.Info("VM updated via admin API",
		"vm_id", vmID,
		"correlation_id", middleware.GetCorrelationID(c))

	// Return updated VM
	updatedVM, err := h.vmService.GetVMDetail(c.Request.Context(), vmID, "", true)
	if err != nil {
		h.logger.Error("failed to fetch updated VM after update",
			"vm_id", vmID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "VM_GET_FAILED", "Failed to retrieve updated VM")
		return
	}
	c.JSON(http.StatusOK, models.Response{Data: updatedVM})
}

// handleVMResize handles VM resource resize operations.
// Returns the taskID if a resize was performed, and a boolean indicating if the response was handled.
func (h *AdminHandler) handleVMResize(c *gin.Context, vmID string, vm *models.VM, req AdminVMUpdateRequest) (string, bool) {
	if req.VCPU == nil && req.MemoryMB == nil && req.DiskGB == nil {
		return "", false
	}

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
	taskID, err := h.vmService.ResizeVM(c.Request.Context(), vmID, "", newVCPU, newMemory, newDisk, true)
	if err != nil {
		h.logger.Error("failed to resize VM",
			"vm_id", vmID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "VM_RESIZE_FAILED", "Internal server error")
		return "", true // handled via error response
	}

	if taskID != "" {
		return taskID, true
	}
	return "", false
}

// DeleteVM handles DELETE /vms/:id - deletes any VM.
// @Tags Admin
// @Summary Delete VM
// @Description Performs administrative VM management operation.
// @Produce json
// @Security BearerAuth
// @Param id path string true "VM ID"
// @Success 202 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/vms/{id} [delete]
func (h *AdminHandler) DeleteVM(c *gin.Context) {
	vmID, ok := validateUUIDParam(c, "id", "INVALID_VM_ID", "VM ID must be a valid UUID")
	if !ok {
		return
	}

	// isAdmin=true allows deleting any VM
	taskID, err := h.vmService.DeleteVM(c.Request.Context(), vmID, "", true)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to delete VM",
			"vm_id", vmID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "VM_DELETE_FAILED", "Internal server error")
		return
	}

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
// @Tags Admin
// @Summary Migrate VM
// @Description Performs administrative VM management operation.
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "VM ID"
// @Param request body object true "Request body"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/vms/{id}/migrate [post]
func (h *AdminHandler) MigrateVM(c *gin.Context) {
	vmID, ok := validateUUIDParam(c, "id", "INVALID_VM_ID", "VM ID must be a valid UUID")
	if !ok {
		return
	}

	var req VMMigrateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Get VM to verify it exists
	vm, err := h.vmService.GetVM(c.Request.Context(), vmID, "", true)
	if err != nil {
		if handleNotFoundError(c, err, "VM_NOT_FOUND", "VM not found") {
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "VM_GET_FAILED", "Failed to retrieve VM")
		return
	}

	// Verify target node exists
	_, err = h.nodeService.GetNode(c.Request.Context(), req.TargetNodeID)
	if err != nil {
		if handleNotFoundError(c, err, "NODE_NOT_FOUND", "Target node not found") {
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "NODE_GET_FAILED", "Failed to retrieve target node")
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
		middleware.RespondWithError(c, http.StatusInternalServerError, "MIGRATION_FAILED", "Internal server error")
		return
	}

	c.JSON(http.StatusAccepted, models.Response{
		Data: VMMigrateResponse{
			VMID:         vmID,
			TargetNodeID: req.TargetNodeID,
			TaskID:       result.TaskID,
			Status:       "migration_initiated",
		},
	})
}
