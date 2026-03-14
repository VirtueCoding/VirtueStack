package provisioning

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetStatus handles GET /vms/:id/status - retrieves the current status of a VM.
// This endpoint is used by WHMCS to check if a VM is running, stopped, suspended, etc.
// It returns both the database status and, if available, the live status from the node agent.
func (h *ProvisioningHandler) GetStatus(c *gin.Context) {
	vmID := c.Param("id")

	// Validate UUID format
	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Get the VM from repository
	vm, err := h.vmRepo.GetByID(c.Request.Context(), vmID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to get VM",
			"vm_id", vmID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "VM_LOOKUP_FAILED", err.Error())
		return
	}

	// Build the response
	resp := VMStatusResponse{
		Status: vm.Status,
	}

	// Include node ID if VM is assigned to a node
	if vm.NodeID != nil {
		resp.NodeID = *vm.NodeID
	}

	// For running VMs, try to get live status from node agent
	// This gives WHMCS the most accurate status
	if vm.Status == models.VMStatusRunning && vm.NodeID != nil {
		liveStatus, err := h.vmService.GetVMStatus(c.Request.Context(), vmID, vm.CustomerID, true)
		if err != nil {
			h.logger.Warn("failed to get live VM status, returning database status",
				"vm_id", vmID,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			// Continue with database status
		} else {
			resp.Status = liveStatus
		}
	}

	c.JSON(http.StatusOK, models.Response{
		Data: resp,
	})
}

// GetVMInfo handles GET /vms/:id - retrieves detailed VM information.
// This endpoint provides complete VM details for WHMCS module integration.
func (h *ProvisioningHandler) GetVMInfo(c *gin.Context) {
	vmID := c.Param("id")

	// Validate UUID format
	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Get the VM with details
	detail, err := h.vmService.GetVMDetail(c.Request.Context(), vmID, "", true)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to get VM details",
			"vm_id", vmID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "VM_LOOKUP_FAILED", err.Error())
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Data: detail,
	})
}

// GetVMByWHMCSServiceID handles GET /vms/by-service/:service_id - finds a VM by WHMCS service ID.
// This endpoint is useful for WHMCS to lookup a VM by its service ID instead of UUID.
func (h *ProvisioningHandler) GetVMByWHMCSServiceID(c *gin.Context) {
	serviceIDStr := c.Param("service_id")

	// Parse service ID as integer
	var serviceID int
	if _, err := uuid.Parse(serviceIDStr); err == nil {
		// If it's a valid UUID, it's not a service ID
		respondWithError(c, http.StatusBadRequest, "INVALID_SERVICE_ID", "Service ID must be a number, not a UUID")
		return
	}

	// Try to parse as integer
	if _, err := c.Params.Get("service_id"); !err {
		respondWithError(c, http.StatusBadRequest, "MISSING_SERVICE_ID", "Service ID is required")
		return
	}

	// Simple integer parsing
	for i, ch := range serviceIDStr {
		if ch < '0' || ch > '9' {
			if i == 0 {
				respondWithError(c, http.StatusBadRequest, "INVALID_SERVICE_ID", "Service ID must be a positive integer")
				return
			}
			break
		}
	}

	// Parse to int
	for _, ch := range serviceIDStr {
		serviceID = serviceID*10 + int(ch-'0')
	}

	// Get the VM by WHMCS service ID
	vm, err := h.vmRepo.GetByWHMCSServiceID(c.Request.Context(), serviceID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "No VM found with the specified WHMCS service ID")
			return
		}
		h.logger.Error("failed to get VM by WHMCS service ID",
			"service_id", serviceID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "VM_LOOKUP_FAILED", err.Error())
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Data: vm,
	})
}

// PowerOperationRequest represents a request for power operations.
type PowerOperationRequest struct {
	Operation string `json:"operation" validate:"required,oneof=start stop restart"`
}

// PowerOperation handles POST /vms/:id/power - performs power operations on a VM.
// Supported operations: start, stop, restart.
func (h *ProvisioningHandler) PowerOperation(c *gin.Context) {
	vmID := c.Param("id")

	// Validate UUID format
	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Parse request body
	var req PowerOperationRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Get the VM
	vm, err := h.vmRepo.GetByID(c.Request.Context(), vmID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "VM_LOOKUP_FAILED", err.Error())
		return
	}

	// Check if VM is already deleted
	if vm.IsDeleted() {
		respondWithError(c, http.StatusGone, "VM_DELETED", "VM has been deleted")
		return
	}

	// Check if VM is suspended (only allow start operation)
	if vm.Status == models.VMStatusSuspended && req.Operation != "start" {
		respondWithError(c, http.StatusBadRequest, "VM_SUSPENDED", "VM is suspended. Unsuspend it first.")
		return
	}

	// Perform the power operation
	var opErr error
	switch req.Operation {
	case "start":
		opErr = h.vmService.StartVM(c.Request.Context(), vmID, vm.CustomerID, true)
	case "stop":
		opErr = h.vmService.StopVM(c.Request.Context(), vmID, vm.CustomerID, false, true)
	case "restart":
		opErr = h.vmService.RestartVM(c.Request.Context(), vmID, vm.CustomerID, true)
	default:
		respondWithError(c, http.StatusBadRequest, "INVALID_OPERATION", "Invalid operation. Use: start, stop, or restart")
		return
	}

	if opErr != nil {
		h.logger.Error("power operation failed",
			"vm_id", vmID,
			"operation", req.Operation,
			"error", opErr,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "POWER_OPERATION_FAILED", opErr.Error())
		return
	}

	h.logger.Info("power operation completed via provisioning API",
		"vm_id", vmID,
		"operation", req.Operation,
		"customer_id", vm.CustomerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{
		Data: gin.H{
			"vm_id":     vmID,
			"operation": req.Operation,
			"message":   "Power operation completed successfully",
		},
	})
}
