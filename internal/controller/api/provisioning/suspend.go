package provisioning

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// SuspendVM handles POST /vms/:id/suspend - suspends a VM for billing purposes.
// This endpoint is called by the billing module when a service is suspended (e.g., non-payment).
// @Tags Provisioning
// @Summary Suspend VM
// @Description Suspends a VM for billing or policy reasons.
// @Produce json
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/provisioning/vms/{id}/suspend [post]
func (h *ProvisioningHandler) SuspendVM(c *gin.Context) {
	vmID := c.Param("id")

	vm, err := h.getValidOwnedVM(c, vmID)
	if err != nil {
		respondWithValidationError(c, err)
		return
	}

	if vm.Status == models.VMStatusSuspended {
		c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "VM is already suspended", "vm_id": vmID}})
		return
	}

	// Force-stop before suspending so QEMU releases the vCPUs and memory.
	stopSucceeded := false
	if vm.Status == models.VMStatusRunning {
		if err := h.vmService.StopVM(c.Request.Context(), vmID, vm.CustomerID, true, true); err != nil {
			h.logger.Warn("failed to stop VM during suspend", "vm_id", vmID, "error", err, "correlation_id", middleware.GetCorrelationID(c))
		} else {
			stopSucceeded = true
		}
	}

	fromStatus := vm.Status
	if vm.Status == models.VMStatusRunning && stopSucceeded {
		fromStatus = models.VMStatusStopped
	}

	if err := h.vmRepo.TransitionStatus(c.Request.Context(), vmID, fromStatus, models.VMStatusSuspended); err != nil {
		if errors.Is(err, sharederrors.ErrConflict) {
			h.logger.Error("invalid VM status transition during suspend",
				"vm_id", vmID,
				"from_status", fromStatus,
				"to_status", models.VMStatusSuspended,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			middleware.RespondWithError(c, http.StatusConflict, "INVALID_VM_STATE", fmt.Sprintf("Cannot suspend VM from status %s", fromStatus))
			return
		}
		h.logger.Error("failed to update VM status to suspended", "vm_id", vmID, "error", err, "correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SUSPEND_FAILED", "Failed to suspend VM")
		return
	}

	h.logger.Info("VM suspended via provisioning API", "vm_id", vmID, "customer_id", vm.CustomerID, "correlation_id", middleware.GetCorrelationID(c))
	c.JSON(http.StatusOK, models.Response{Data: gin.H{"vm_id": vmID, "status": models.VMStatusSuspended}})
}

// UnsuspendVM handles POST /vms/:id/unsuspend - unsuspends a VM.
// This endpoint is called by the billing module when a service is reactivated (e.g., payment received).
// @Tags Provisioning
// @Summary Unsuspend VM
// @Description Lifts VM suspension and restores normal operation.
// @Produce json
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/provisioning/vms/{id}/unsuspend [post]
func (h *ProvisioningHandler) UnsuspendVM(c *gin.Context) {
	vmID := c.Param("id")

	vm, err := h.getValidOwnedVM(c, vmID)
	if err != nil {
		respondWithValidationError(c, err)
		return
	}

	if vm.Status != models.VMStatusSuspended {
		c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "VM is not suspended", "vm_id": vmID, "status": vm.Status}})
		return
	}

	if err := h.vmRepo.TransitionStatus(c.Request.Context(), vmID, models.VMStatusSuspended, models.VMStatusStopped); err != nil {
		if errors.Is(err, sharederrors.ErrConflict) {
			h.logger.Error("invalid VM status transition during unsuspend",
				"vm_id", vmID,
				"from_status", models.VMStatusSuspended,
				"to_status", models.VMStatusStopped,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			middleware.RespondWithError(c, http.StatusConflict, "INVALID_VM_STATE", "Cannot unsuspend VM because state changed")
			return
		}
		h.logger.Error("failed to update VM status to stopped during unsuspend", "vm_id", vmID, "error", err, "correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "UNSUSPEND_FAILED", "Failed to unsuspend VM")
		return
	}

	h.logger.Info("VM unsuspended via provisioning API", "vm_id", vmID, "customer_id", vm.CustomerID, "correlation_id", middleware.GetCorrelationID(c))
	c.JSON(http.StatusOK, models.Response{Data: gin.H{"vm_id": vmID, "status": models.VMStatusStopped}})
}
