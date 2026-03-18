package provisioning

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// SuspendVM handles POST /vms/:id/suspend - suspends a VM for billing purposes.
// This endpoint is called by WHMCS when a service is suspended (e.g., non-payment).
// The VM is stopped and marked as suspended, blocking console access.
func (h *ProvisioningHandler) SuspendVM(c *gin.Context) {
	vmID := c.Param("id")

	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	vm, err := h.vmRepo.GetByID(c.Request.Context(), vmID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to get VM for suspend",
			"vm_id", vmID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "VM_LOOKUP_FAILED", "Internal server error")
		return
	}

	if vm.IsDeleted() {
		respondWithError(c, http.StatusGone, "VM_DELETED", "VM has been deleted")
		return
	}

	if vm.Status == models.VMStatusSuspended {
		c.JSON(http.StatusOK, models.Response{
			Data: gin.H{
				"message": "VM is already suspended",
				"vm_id":   vmID,
			},
		})
		return
	}

	// Force-stop before suspending so QEMU releases the vCPUs and memory.
	if vm.Status == models.VMStatusRunning {
		if err := h.vmService.StopVM(c.Request.Context(), vmID, vm.CustomerID, true, true); err != nil {
			h.logger.Warn("failed to stop VM during suspend",
				"vm_id", vmID,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			// Continue with suspend even if stop fails
		}
	}

	if err := h.vmRepo.UpdateStatus(c.Request.Context(), vmID, models.VMStatusSuspended); err != nil {
		h.logger.Error("failed to update VM status to suspended",
			"vm_id", vmID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "SUSPEND_FAILED", "Failed to suspend VM")
		return
	}

	h.logger.Info("VM suspended via provisioning API",
		"vm_id", vmID,
		"customer_id", vm.CustomerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{
		Data: gin.H{
			"vm_id":  vmID,
			"status": models.VMStatusSuspended,
		},
	})
}

// UnsuspendVM handles POST /vms/:id/unsuspend - unsuspends a VM.
// This endpoint is called by WHMCS when a service is reactivated (e.g., payment received).
// The VM status is restored, but the VM remains stopped until manually started.
func (h *ProvisioningHandler) UnsuspendVM(c *gin.Context) {
	vmID := c.Param("id")

	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	vm, err := h.vmRepo.GetByID(c.Request.Context(), vmID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to get VM for unsuspend",
			"vm_id", vmID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "VM_LOOKUP_FAILED", "Internal server error")
		return
	}

	if vm.IsDeleted() {
		respondWithError(c, http.StatusGone, "VM_DELETED", "VM has been deleted")
		return
	}

	if vm.Status != models.VMStatusSuspended {
		c.JSON(http.StatusOK, models.Response{
			Data: gin.H{
				"message": "VM is not suspended",
				"vm_id":   vmID,
				"status":  vm.Status,
			},
		})
		return
	}

	// Restore to stopped rather than running; customer decides when to restart.
	if err := h.vmRepo.UpdateStatus(c.Request.Context(), vmID, models.VMStatusStopped); err != nil {
		h.logger.Error("failed to update VM status to stopped during unsuspend",
			"vm_id", vmID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "UNSUSPEND_FAILED", "Failed to unsuspend VM")
		return
	}

	h.logger.Info("VM unsuspended via provisioning API",
		"vm_id", vmID,
		"customer_id", vm.CustomerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{
		Data: gin.H{
			"vm_id":  vmID,
			"status": models.VMStatusStopped,
		},
	})
}