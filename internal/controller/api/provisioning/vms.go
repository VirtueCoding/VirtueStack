package provisioning

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CreateVM handles POST /vms - creates a new VM asynchronously.
// This endpoint is called by WHMCS when a new service is provisioned.
// Returns 202 Accepted with a task_id for polling the operation status.
func (h *ProvisioningHandler) CreateVM(c *gin.Context) {
	var req ProvisioningCreateVMRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Get idempotency key from header if present
	idempotencyKey := c.GetHeader("Idempotency-Key")

	// Generate a random password if not provided via SSH keys
	// WHMCS module should provide the password, but we generate one if not
	password := generateRandomPassword()

	// Build the VM create request for the service layer
	vmReq := &models.VMCreateRequest{
		CustomerID:     req.CustomerID,
		PlanID:         req.PlanID,
		TemplateID:     req.TemplateID,
		Hostname:       req.Hostname,
		Password:       password,
		SSHKeys:        req.SSHKeys,
		WHMCSServiceID: &req.WHMCSServiceID,
		IdempotencyKey: idempotencyKey,
	}

	if req.LocationID != "" {
		vmReq.LocationID = &req.LocationID
	}

	// Create VM through the service layer
	vm, taskID, err := h.vmService.CreateVM(c.Request.Context(), vmReq, req.CustomerID)
	if err != nil {
		h.logger.Error("failed to create VM",
			"customer_id", req.CustomerID,
			"hostname", req.Hostname,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "VM_CREATE_FAILED", err.Error())
		return
	}

	h.logger.Info("VM creation initiated via provisioning API",
		"vm_id", vm.ID,
		"task_id", taskID,
		"customer_id", req.CustomerID,
		"whmcs_service_id", req.WHMCSServiceID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusAccepted, models.Response{
		Data: CreateVMResponse{
			TaskID: taskID,
			VMID:   vm.ID,
		},
	})
}

// DeleteVM handles DELETE /vms/:id - terminates a VM asynchronously.
// This endpoint is called by WHMCS when a service is cancelled/terminated.
// Returns 202 Accepted with a task_id for polling the operation status.
func (h *ProvisioningHandler) DeleteVM(c *gin.Context) {
	vmID := c.Param("id")

	// Validate UUID format
	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Get the VM to verify it exists
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
		respondWithError(c, http.StatusGone, "VM_ALREADY_DELETED", "VM has already been terminated")
		return
	}

	// Delete VM through service layer (admin=true to bypass ownership checks)
	taskID, err := h.vmService.DeleteVM(c.Request.Context(), vmID, vm.CustomerID, true)
	if err != nil {
		h.logger.Error("failed to delete VM",
			"vm_id", vmID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "VM_DELETE_FAILED", err.Error())
		return
	}

	h.logger.Info("VM termination initiated via provisioning API",
		"vm_id", vmID,
		"task_id", taskID,
		"customer_id", vm.CustomerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusAccepted, models.Response{
		Data: TaskResponse{
			TaskID: taskID,
		},
	})
}

// generateRandomPassword creates a cryptographically secure random password.
func generateRandomPassword() string {
	// Generate a 16-character password using crypto
	token, err := crypto.GenerateRandomToken(16)
	if err != nil {
		// Fallback to UUID-based password
		return "Vs" + uuid.New().String()[:16]
	}
	return "Vs" + token[:14] // Prefix with "Vs" to ensure it starts with a letter
}

// respondWithError sends a standardized error response.
func respondWithError(c *gin.Context, status int, code, message string) {
	correlationID := middleware.GetCorrelationID(c)

	c.JSON(status, gin.H{
		"error": gin.H{
			"code":           code,
			"message":        message,
			"correlation_id": correlationID,
		},
	})
}
