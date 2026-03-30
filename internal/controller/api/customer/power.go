package customer

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// StartVM handles POST /vms/:id/start - starts a stopped or suspended VM.
// Returns 200 OK on success, appropriate error codes on failure.
// @Tags Customer
// @Summary Start VM
// @Description Performs VM power operation for a customer-owned VM.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Success 202 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/vms/{id}/start [post]
func (h *CustomerHandler) StartVM(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Start VM with ownership verification (isAdmin=false)
	if err := h.vmService.StartVM(c.Request.Context(), vmID, customerID, false); err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}

		h.logger.Error("failed to start VM",
			"error", err,
			"vm_id", vmID,
			"customer_id", customerID,
			"correlation_id", middleware.GetCorrelationID(c))

		middleware.RespondWithError(c, http.StatusInternalServerError, "VM_START_FAILED", "Failed to start VM")
		return
	}

	h.logger.Info("VM started via customer API",
		"vm_id", vmID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "VM started successfully"}})
}

// StopVM handles POST /vms/:id/stop - gracefully stops a running VM.
// Uses ACPI shutdown with a timeout. Returns 200 OK on success.
// @Tags Customer
// @Summary Stop VM
// @Description Performs VM power operation for a customer-owned VM.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Success 202 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/vms/{id}/stop [post]
func (h *CustomerHandler) StopVM(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Stop VM gracefully with ownership verification (isAdmin=false)
	if err := h.vmService.StopVM(c.Request.Context(), vmID, customerID, false, false); err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}

		h.logger.Error("failed to stop VM",
			"error", err,
			"vm_id", vmID,
			"customer_id", customerID,
			"correlation_id", middleware.GetCorrelationID(c))

		middleware.RespondWithError(c, http.StatusInternalServerError, "VM_STOP_FAILED", "Failed to stop VM")
		return
	}

	h.logger.Info("VM stopped via customer API",
		"vm_id", vmID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "VM stopped successfully"}})
}

// RestartVM handles POST /vms/:id/restart - restarts a running VM.
// Performs graceful ACPI shutdown followed by start.
// @Tags Customer
// @Summary Restart VM
// @Description Performs VM power operation for a customer-owned VM.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Success 202 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/vms/{id}/restart [post]
func (h *CustomerHandler) RestartVM(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Restart VM with ownership verification (isAdmin=false)
	if err := h.vmService.RestartVM(c.Request.Context(), vmID, customerID, false); err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}

		h.logger.Error("failed to restart VM",
			"error", err,
			"vm_id", vmID,
			"customer_id", customerID,
			"correlation_id", middleware.GetCorrelationID(c))

		middleware.RespondWithError(c, http.StatusInternalServerError, "VM_RESTART_FAILED", "Failed to restart VM")
		return
	}

	h.logger.Info("VM restarted via customer API",
		"vm_id", vmID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "VM restarted successfully"}})
}

// ForceStopVM handles POST /vms/:id/force-stop - forcefully powers off a VM.
// This is equivalent to pulling the power plug. Use with caution.
// @Tags Customer
// @Summary Force stop VM
// @Description Performs VM power operation for a customer-owned VM.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Success 202 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/vms/{id}/force-stop [post]
func (h *CustomerHandler) ForceStopVM(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Force stop VM with ownership verification (isAdmin=false)
	if err := h.vmService.StopVM(c.Request.Context(), vmID, customerID, true, false); err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}

		h.logger.Error("failed to force stop VM",
			"error", err,
			"vm_id", vmID,
			"customer_id", customerID,
			"correlation_id", middleware.GetCorrelationID(c))

		middleware.RespondWithError(c, http.StatusInternalServerError, "VM_FORCE_STOP_FAILED", "Failed to force stop VM")
		return
	}

	h.logger.Info("VM force stopped via customer API",
		"vm_id", vmID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "VM force stopped successfully"}})
}
