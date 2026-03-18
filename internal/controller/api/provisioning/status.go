package provisioning

import (
	"net/http"
	"strconv"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// GetStatus handles GET /vms/:id/status - retrieves the current status of a VM.
func (h *ProvisioningHandler) GetStatus(c *gin.Context) {
	vmID := c.Param("id")

	vm, err := getValidVM(c.Request.Context(), h.vmRepo, vmID, h.logger)
	if err != nil {
		respondWithValidationError(c, err)
		return
	}

	resp := buildVMStatusResponse(vm)

	// Prefer live hypervisor state for running VMs
	if vm.Status == models.VMStatusRunning && vm.NodeID != nil {
		if liveStatus, err := h.vmService.GetVMStatus(c.Request.Context(), vmID, vm.CustomerID, true); err != nil {
			h.logger.Warn("failed to get live VM status, returning database status", "vm_id", vmID, "error", err, "correlation_id", middleware.GetCorrelationID(c))
		} else {
			resp.Status = liveStatus
		}
	}

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

// buildVMStatusResponse builds a VMStatusResponse from a VM.
func buildVMStatusResponse(vm *models.VM) VMStatusResponse {
	resp := VMStatusResponse{Status: vm.Status}
	if vm.NodeID != nil {
		resp.NodeID = *vm.NodeID
	}
	return resp
}

// GetVMInfo handles GET /vms/:id - retrieves detailed VM information.
// This endpoint provides complete VM details for WHMCS module integration.
func (h *ProvisioningHandler) GetVMInfo(c *gin.Context) {
	vmID := c.Param("id")

	if _, err := validateVMID(vmID); err != nil {
		respondWithValidationError(c, err)
		return
	}

	detail, err := h.vmService.GetVMDetail(c.Request.Context(), vmID, "", true)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to get VM details", "vm_id", vmID, "error", err, "correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "VM_LOOKUP_FAILED", "Internal server error")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: detail})
}

// GetVMByWHMCSServiceID handles GET /vms/by-service/:service_id - finds a VM by WHMCS service ID.
// This endpoint is useful for WHMCS to lookup a VM by its service ID instead of UUID.
func (h *ProvisioningHandler) GetVMByWHMCSServiceID(c *gin.Context) {
	serviceIDStr := c.Param("service_id")

	// strconv.Atoi rejects non-digit suffixes (e.g. "123abc") that Sscanf would silently accept.
	serviceID, err := strconv.Atoi(serviceIDStr)
	if err != nil || serviceID <= 0 {
		respondWithError(c, http.StatusBadRequest, "INVALID_SERVICE_ID", "Service ID must be a positive integer")
		return
	}

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
		respondWithError(c, http.StatusInternalServerError, "VM_LOOKUP_FAILED", "Internal server error")
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

	vm, err := getValidVM(c.Request.Context(), h.vmRepo, vmID, h.logger)
	if err != nil {
		respondWithValidationError(c, err)
		return
	}

	var req PowerOperationRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	if err := validatePowerOperation(vm, req.Operation); err != nil {
		respondWithValidationError(c, err)
		return
	}

	if err := h.executePowerOperation(c.Request.Context(), vmID, vm.CustomerID, req.Operation); err != nil {
		respondWithValidationError(c, err)
		return
	}

	h.logger.Info("power operation completed via provisioning API",
		"vm_id", vmID, "operation", req.Operation, "customer_id", vm.CustomerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: powerOperationResponse(vmID, req.Operation)})
}
