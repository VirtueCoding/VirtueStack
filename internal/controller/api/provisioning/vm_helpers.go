package provisioning

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// vmValidationError represents a validation error with HTTP response details.
type vmValidationError struct {
	status  int
	errCode string
	errMsg  string
}

// validateVMID validates that a VM ID is a valid UUID.
func validateVMID(vmID string) (string, error) {
	if _, err := uuid.Parse(vmID); err != nil {
		return "", vmValidationError{
			status:  http.StatusBadRequest,
			errCode: "INVALID_VM_ID",
			errMsg:  "VM ID must be a valid UUID",
		}
	}
	return vmID, nil
}

// Error implements the error interface.
func (e vmValidationError) Error() string {
	return e.errMsg
}

// getValidVM validates the VM ID, fetches the VM, and checks common error conditions.
// It returns the VM on success, or a vmValidationError on failure.
// The caller should check if the returned error is a vmValidationError and respond accordingly.
func getValidVM(ctx context.Context, vmRepo *repository.VMRepository, vmID string, logger interface{ Error(string, ...any) }) (*models.VM, error) {
	if _, err := uuid.Parse(vmID); err != nil {
		return nil, vmValidationError{
			status:  http.StatusBadRequest,
			errCode: "INVALID_VM_ID",
			errMsg:  "VM ID must be a valid UUID",
		}
	}

	vm, err := vmRepo.GetByID(ctx, vmID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, vmValidationError{
				status:  http.StatusNotFound,
				errCode: "VM_NOT_FOUND",
				errMsg:  "VM not found",
			}
		}
		logger.Error("failed to get VM", "vm_id", vmID, "error", err)
		return nil, vmValidationError{
			status:  http.StatusInternalServerError,
			errCode: "VM_LOOKUP_FAILED",
			errMsg:  "Internal server error",
		}
	}

	if vm.IsDeleted() {
		return nil, vmValidationError{
			status:  http.StatusGone,
			errCode: "VM_DELETED",
			errMsg:  "VM has been deleted",
		}
	}

	return vm, nil
}

// checkVMOpts provides optional checks for getValidVMWithChecks.
type checkVMOpts struct {
	NotSuspended    bool
	SuspendedErrMsg string
	AlreadyDeleted  bool
	DeletedErrMsg   string
}

// getValidVMWithChecks is like getValidVM but with additional optional checks.
func getValidVMWithChecks(ctx context.Context, vmRepo *repository.VMRepository, vmID string, logger interface{ Error(string, ...any) }, opts checkVMOpts) (*models.VM, error) {
	vm, err := getValidVM(ctx, vmRepo, vmID, logger)
	if err != nil {
		// Override error messages if specified
		if opts.AlreadyDeleted {
			var ve vmValidationError
			if errors.As(err, &ve) && ve.errCode == "VM_DELETED" {
				ve.errCode = "VM_ALREADY_DELETED"
				if opts.DeletedErrMsg != "" {
					ve.errMsg = opts.DeletedErrMsg
				} else {
					ve.errMsg = "VM has already been terminated"
				}
				return nil, ve
			}
		}
		return nil, err
	}

	if opts.NotSuspended && vm.Status == models.VMStatusSuspended {
		errMsg := "Cannot perform this operation on a suspended VM. Unsuspend first."
		if opts.SuspendedErrMsg != "" {
			errMsg = opts.SuspendedErrMsg
		}
		return nil, vmValidationError{
			status:  http.StatusBadRequest,
			errCode: "VM_SUSPENDED",
			errMsg:  errMsg,
		}
	}

	return vm, nil
}

// respondWithValidationError sends a vmValidationError as an HTTP response.
func respondWithValidationError(c *gin.Context, err error) {
	var ve vmValidationError
	if errors.As(err, &ve) {
		middleware.RespondWithError(c, ve.status, ve.errCode, ve.errMsg)
		return
	}
	middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
}

// calculateResizeValues computes the new resource values for a resize operation.
// Unspecified fields keep their current values for partial resize.
func calculateResizeValues(vm *models.VM, req ResizeRequest) (vcpu, memoryMB, diskGB int) {
	vcpu = vm.VCPU
	memoryMB = vm.MemoryMB
	diskGB = vm.DiskGB

	if req.VCPU != nil {
		vcpu = *req.VCPU
	}
	if req.MemoryMB != nil {
		memoryMB = *req.MemoryMB
	}
	if req.DiskGB != nil {
		diskGB = *req.DiskGB
	}

	return vcpu, memoryMB, diskGB
}

// validateDiskResize checks that disk shrinking is not attempted.
func validateDiskResize(currentGB, newGB int) error {
	if newGB < currentGB {
		return vmValidationError{
			status:  http.StatusBadRequest,
			errCode: "VALIDATION_ERROR",
			errMsg:  fmt.Sprintf("Disk shrinking is not supported. Current: %d GB, Requested: %d GB", currentGB, newGB),
		}
	}
	return nil
}

// resizeResponseData creates the response data for a resize operation.
func resizeResponseData(vmID string, taskID string, vcpu, memoryMB, diskGB int) gin.H {
	data := gin.H{
		"vm_id":     vmID,
		"vcpu":      vcpu,
		"memory_mb": memoryMB,
		"disk_gb":   diskGB,
	}
	if taskID != "" {
		data["task_id"] = taskID
	}
	return data
}

// validateResizeRequest validates the resize request has at least one field.
func validateResizeRequest(req ResizeRequest) error {
	if req.VCPU == nil && req.MemoryMB == nil && req.DiskGB == nil {
		return vmValidationError{
			status:  http.StatusBadRequest,
			errCode: "VALIDATION_ERROR",
			errMsg:  "At least one resize parameter (vcpu, memory_mb, or disk_gb) must be provided",
		}
	}
	return nil
}

// bindAndValidateResizeRequest binds and validates a resize request.
// It returns the request and an error if binding or validation fails.
func bindAndValidateResizeRequest(c *gin.Context) (*ResizeRequest, error) {
	var req ResizeRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			return nil, vmValidationError{
				status:  apiErr.HTTPStatus,
				errCode: apiErr.Code,
				errMsg:  apiErr.Message,
			}
		}
		return nil, vmValidationError{
			status:  http.StatusBadRequest,
			errCode: "VALIDATION_ERROR",
			errMsg:  "Invalid request",
		}
	}

	if err := validateResizeRequest(req); err != nil {
		return nil, err
	}

	return &req, nil
}

// handleResizeError logs and responds to a resize error.
func handleResizeError(c *gin.Context, logger interface{ Error(string, ...any) }, vmID string, err error) {
	logger.Error("failed to resize VM", "vm_id", vmID, "error", err, "correlation_id", middleware.GetCorrelationID(c))
	middleware.RespondWithError(c, http.StatusInternalServerError, "RESIZE_FAILED", "Internal server error")
}

// logResize logs a successful resize operation.
func (h *ProvisioningHandler) logResize(vmID string, vm *models.VM, newVCPU, newMemoryMB, newDiskGB int, correlationID string) {
	h.logger.Info("VM resized via provisioning API",
		"vm_id", vmID, "customer_id", vm.CustomerID,
		"old_vcpu", vm.VCPU, "new_vcpu", newVCPU,
		"old_memory_mb", vm.MemoryMB, "new_memory_mb", newMemoryMB,
		"old_disk_gb", vm.DiskGB, "new_disk_gb", newDiskGB,
		"correlation_id", correlationID)
}

// resizeResponseStatus returns the appropriate HTTP status for a resize response.
func resizeResponseStatus(taskID string) int {
	if taskID != "" {
		return http.StatusAccepted
	}
	return http.StatusOK
}

// executePowerOperation executes the specified power operation on a VM.
func (h *ProvisioningHandler) executePowerOperation(ctx context.Context, vmID, customerID, operation string) error {
	switch operation {
	case "start":
		return h.vmService.StartVM(ctx, vmID, customerID, true)
	case "stop":
		return h.vmService.StopVM(ctx, vmID, customerID, false, true)
	case "restart":
		return h.vmService.RestartVM(ctx, vmID, customerID, true)
	default:
		return vmValidationError{
			status:  http.StatusBadRequest,
			errCode: "INVALID_OPERATION",
			errMsg:  "Invalid operation. Use: start, stop, or restart",
		}
	}
}

// validatePowerOperation checks if the power operation is allowed for the VM state.
func validatePowerOperation(vm *models.VM, operation string) error {
	if vm.Status == models.VMStatusSuspended && operation != "start" {
		return vmValidationError{
			status:  http.StatusBadRequest,
			errCode: "VM_SUSPENDED",
			errMsg:  "VM is suspended. Unsuspend it first.",
		}
	}
	return nil
}

// powerOperationResponse creates the response data for a power operation.
func powerOperationResponse(vmID, operation string) gin.H {
	return gin.H{
		"vm_id":     vmID,
		"operation": operation,
		"message":   "Power operation completed successfully",
	}
}