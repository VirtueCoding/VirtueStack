package customer

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ListVMs handles GET /vms - lists all VMs owned by the authenticated customer.
// Supports pagination via query parameters (page, per_page).
func (h *CustomerHandler) ListVMs(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	// Parse pagination
	pagination := models.ParsePagination(c)

	// Build filter with customer isolation
	filter := models.VMListFilter{
		CustomerID:       &customerID,
		PaginationParams: pagination,
	}

	// Get vm_ids scope from API key (if any)
	if vmIDs := middleware.GetVMIDs(c); len(vmIDs) > 0 {
		filter.VMIDs = vmIDs
	}

	// Optional status filter
	validVMStatuses := map[string]bool{
		"provisioning": true, "running": true, "stopped": true, "suspended": true,
		"migrating": true, "reinstalling": true, "error": true, "deleted": true,
	}
	if status := c.Query("status"); status != "" {
		if !validVMStatuses[status] {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_STATUS", "Invalid status value")
			return
		}
		filter.Status = &status
	}

	// Optional search filter
	const maxSearchLength = 100
	if search := c.Query("search"); search != "" {
		if len(search) > maxSearchLength {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_SEARCH", "search parameter must not exceed 100 characters")
			return
		}
		filter.Search = &search
	}

	vms, total, err := h.vmService.ListVMs(c.Request.Context(), filter, customerID, false)
	if err != nil {
		h.logger.Error("failed to list VMs",
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "VM_LIST_FAILED", "Failed to retrieve VMs")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: vms,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}

// GetVM handles GET /vms/:id - retrieves details for a specific VM.
// Enforces customer isolation - returns 404 if VM doesn't belong to customer.
func (h *CustomerHandler) GetVM(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Get VM with ownership verification (isAdmin=false)
	vm, err := h.vmService.GetVMDetail(c.Request.Context(), vmID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to get VM",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "VM_GET_FAILED", "Failed to retrieve VM")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: vm})
}
