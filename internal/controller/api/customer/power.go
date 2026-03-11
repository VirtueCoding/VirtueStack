package customer

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// StartVM handles POST /vms/:id/start - starts a stopped or suspended VM.
// Returns 200 OK on success, appropriate error codes on failure.
func (h *CustomerHandler) StartVM(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Start VM with ownership verification (isAdmin=false)
	if err := h.vmService.StartVM(c.Request.Context(), vmID, customerID, false); err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}

		h.logger.Warn("failed to start VM",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		// Check for specific error conditions
		errMsg := err.Error()
		if contains(errMsg, "cannot start VM in status") {
			respondWithError(c, http.StatusConflict, "INVALID_VM_STATE", errMsg)
			return
		}

		respondWithError(c, http.StatusInternalServerError, "VM_START_FAILED", errMsg)
		return
	}

	h.logger.Info("VM started via customer API",
		"vm_id", vmID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, gin.H{
		"message": "VM started successfully",
	})
}

// StopVM handles POST /vms/:id/stop - gracefully stops a running VM.
// Uses ACPI shutdown with a timeout. Returns 200 OK on success.
func (h *CustomerHandler) StopVM(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Stop VM gracefully with ownership verification (isAdmin=false)
	if err := h.vmService.StopVM(c.Request.Context(), vmID, customerID, false, false); err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}

		h.logger.Warn("failed to stop VM",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		errMsg := err.Error()
		if contains(errMsg, "cannot stop VM in status") {
			respondWithError(c, http.StatusConflict, "INVALID_VM_STATE", errMsg)
			return
		}

		respondWithError(c, http.StatusInternalServerError, "VM_STOP_FAILED", errMsg)
		return
	}

	h.logger.Info("VM stopped via customer API",
		"vm_id", vmID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, gin.H{
		"message": "VM stopped successfully",
	})
}

// RestartVM handles POST /vms/:id/restart - restarts a running VM.
// Performs graceful ACPI shutdown followed by start.
func (h *CustomerHandler) RestartVM(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Restart VM with ownership verification (isAdmin=false)
	if err := h.vmService.RestartVM(c.Request.Context(), vmID, customerID, false); err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}

		h.logger.Warn("failed to restart VM",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		errMsg := err.Error()
		if contains(errMsg, "cannot restart VM in status") {
			respondWithError(c, http.StatusConflict, "INVALID_VM_STATE", errMsg)
			return
		}

		respondWithError(c, http.StatusInternalServerError, "VM_RESTART_FAILED", errMsg)
		return
	}

	h.logger.Info("VM restarted via customer API",
		"vm_id", vmID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, gin.H{
		"message": "VM restarted successfully",
	})
}

// ForceStopVM handles POST /vms/:id/force-stop - forcefully powers off a VM.
// This is equivalent to pulling the power plug. Use with caution.
func (h *CustomerHandler) ForceStopVM(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Force stop VM with ownership verification (isAdmin=false)
	if err := h.vmService.StopVM(c.Request.Context(), vmID, customerID, true, false); err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}

		h.logger.Warn("failed to force stop VM",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		errMsg := err.Error()
		if contains(errMsg, "cannot stop VM in status") {
			respondWithError(c, http.StatusConflict, "INVALID_VM_STATE", errMsg)
			return
		}

		respondWithError(c, http.StatusInternalServerError, "VM_FORCE_STOP_FAILED", errMsg)
		return
	}

	h.logger.Info("VM force stopped via customer API",
		"vm_id", vmID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, gin.H{
		"message": "VM force stopped successfully",
	})
}

// contains checks if a string contains a substring (case-sensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}