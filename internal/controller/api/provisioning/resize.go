package provisioning

import (
	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
)

// ResizeVM handles POST /vms/:id/resize - resizes VM resources.
// This endpoint is called by WHMCS when a service is upgraded.
// Supports upgrading vCPU, memory, and disk (shrinking not supported).
func (h *ProvisioningHandler) ResizeVM(c *gin.Context) {
	vmID := c.Param("id")

	req, err := bindAndValidateResizeRequest(c)
	if err != nil {
		respondWithValidationError(c, err)
		return
	}

	vm, err := getValidVMWithChecks(c.Request.Context(), h.vmRepo, vmID, h.logger, checkVMOpts{
		NotSuspended:    true,
		SuspendedErrMsg: "Cannot resize a suspended VM. Unsuspend first.",
	})
	if err != nil {
		respondWithValidationError(c, err)
		return
	}

	newVCPU, newMemoryMB, newDiskGB := calculateResizeValues(vm, *req)
	if err := validateDiskResize(vm.DiskGB, newDiskGB); err != nil {
		respondWithValidationError(c, err)
		return
	}

	taskID, err := h.vmService.ResizeVM(c.Request.Context(), vmID, vm.CustomerID, newVCPU, newMemoryMB, newDiskGB, true)
	if err != nil {
		handleResizeError(c, h.logger, vmID, err)
		return
	}

	h.logResize(vmID, vm, newVCPU, newMemoryMB, newDiskGB, middleware.GetCorrelationID(c))
	c.JSON(resizeResponseStatus(taskID), models.Response{Data: resizeResponseData(vmID, taskID, newVCPU, newMemoryMB, newDiskGB)})
}
