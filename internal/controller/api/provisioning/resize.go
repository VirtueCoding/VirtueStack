package provisioning

import (
	"fmt"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ResizeVM handles POST /vms/:id/resize - resizes VM resources.
// This endpoint is called by WHMCS when a service is upgraded.
// Supports upgrading vCPU, memory, and disk (shrinking not supported).
func (h *ProvisioningHandler) ResizeVM(c *gin.Context) {
	vmID := c.Param("id")

	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	var req ResizeRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Validate at least one field is provided
	if req.VCPU == nil && req.MemoryMB == nil && req.DiskGB == nil {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "At least one resize parameter (vcpu, memory_mb, or disk_gb) must be provided")
		return
	}

	vm, err := h.vmRepo.GetByID(c.Request.Context(), vmID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to get VM for resize",
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

	// Check if VM is suspended
	if vm.Status == models.VMStatusSuspended {
		respondWithError(c, http.StatusBadRequest, "VM_SUSPENDED", "Cannot resize a suspended VM. Unsuspend first.")
		return
	}

	// Unspecified fields keep their current values so partial resize is safe.
	newVCPU := vm.VCPU
	newMemoryMB := vm.MemoryMB
	newDiskGB := vm.DiskGB

	if req.VCPU != nil {
		newVCPU = *req.VCPU
	}
	if req.MemoryMB != nil {
		newMemoryMB = *req.MemoryMB
	}
	if req.DiskGB != nil {
		newDiskGB = *req.DiskGB
	}

	// Validate resource changes (only upgrades allowed for disk)
	if newDiskGB < vm.DiskGB {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR",
			fmt.Sprintf("Disk shrinking is not supported. Current: %d GB, Requested: %d GB", vm.DiskGB, newDiskGB))
		return
	}

	// admin=true bypasses per-plan limits; WHMCS manages billing separately.
	taskID, err := h.vmService.ResizeVM(c.Request.Context(), vmID, vm.CustomerID, newVCPU, newMemoryMB, newDiskGB, true)
	if err != nil {
		h.logger.Error("failed to resize VM",
			"vm_id", vmID,
			"vcpu", newVCPU,
			"memory_mb", newMemoryMB,
			"disk_gb", newDiskGB,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "RESIZE_FAILED", "Internal server error")
		return
	}

	h.logger.Info("VM resized via provisioning API",
		"vm_id", vmID,
		"customer_id", vm.CustomerID,
		"old_vcpu", vm.VCPU,
		"new_vcpu", newVCPU,
		"old_memory_mb", vm.MemoryMB,
		"new_memory_mb", newMemoryMB,
		"old_disk_gb", vm.DiskGB,
		"new_disk_gb", newDiskGB,
		"correlation_id", middleware.GetCorrelationID(c))

	if taskID != "" {
		c.JSON(http.StatusAccepted, models.Response{
			Data: gin.H{
				"task_id":   taskID,
				"vm_id":     vmID,
				"vcpu":      newVCPU,
				"memory_mb": newMemoryMB,
				"disk_gb":   newDiskGB,
			},
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Data: gin.H{
			"vm_id":     vmID,
			"vcpu":      newVCPU,
			"memory_mb": newMemoryMB,
			"disk_gb":   newDiskGB,
		},
	})
}
