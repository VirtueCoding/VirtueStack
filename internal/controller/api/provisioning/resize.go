package provisioning

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// ResizeVM handles POST /vms/:id/resize - resizes VM resources.
// This endpoint is called by WHMCS when a service is upgraded.
// SECURITY: plan_id is REQUIRED. All resize operations must be validated against
// a plan to ensure billing integrity. WHMCS is responsible for price-to-plan matching.
// Arbitrary resource values are NOT accepted - they must come from a valid plan.
// @Tags Provisioning
// @Summary Resize VM
// @Description Resizes VM compute and storage resources.
// @Accept json
// @Produce json
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Param request body object true "Resize VM request"
// @Success 202 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/provisioning/vms/{id}/resize [post]
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

	// SECURITY: plan_id is REQUIRED for all resize operations.
	// This ensures billing integrity - WHMCS validates price-to-plan matching.
	// Arbitrary resource values without a plan_id are rejected.
	if req.PlanID == "" {
		respondWithValidationError(c, vmValidationError{
			status:  http.StatusBadRequest,
			errCode: "PLAN_ID_REQUIRED",
			errMsg:  "plan_id is required for resize operations. Contact WHMCS to upgrade your service.",
		})
		return
	}

	// Validate the plan exists and is active
	plan, err := h.vmService.GetPlan(c.Request.Context(), req.PlanID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithValidationError(c, vmValidationError{
				status:  http.StatusBadRequest,
				errCode: "INVALID_PLAN",
				errMsg:  "Plan not found",
			})
			return
		}
		handleResizeError(c, h.logger, vmID, err)
		return
	}
	if !plan.IsActive {
		respondWithValidationError(c, vmValidationError{
			status:  http.StatusBadRequest,
			errCode: "PLAN_INACTIVE",
			errMsg:  "Plan is not active",
		})
		return
	}

	// Use plan values for resize (ignore any arbitrary values in request)
	newVCPU := plan.VCPU
	newMemoryMB := plan.MemoryMB
	newDiskGB := plan.DiskGB

	// Validate disk is not shrinking (not supported)
	if err := validateDiskResize(vm.DiskGB, newDiskGB); err != nil {
		respondWithValidationError(c, err)
		return
	}

	// isAdmin=true is safe because all values come from a validated plan
	taskID, err := h.vmService.ResizeVMWithPlan(c.Request.Context(), vmID, vm.CustomerID, newVCPU, newMemoryMB, newDiskGB, plan.ID, true)
	if err != nil {
		handleResizeError(c, h.logger, vmID, err)
		return
	}

	h.logResize(vmID, vm, newVCPU, newMemoryMB, newDiskGB, middleware.GetCorrelationID(c))
	c.JSON(resizeResponseStatus(taskID), models.Response{Data: resizeResponseData(vmID, taskID, newVCPU, newMemoryMB, newDiskGB)})
}