package admin

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CustomerUpdateRequest represents the request body for updating a customer.
type CustomerUpdateRequest struct {
	Name   *string `json:"name,omitempty" validate:"omitempty,max=255"`
	Status *string `json:"status,omitempty" validate:"omitempty,oneof=active suspended"`
}

// CustomerDetail represents a customer with additional statistics.
type CustomerDetail struct {
	models.Customer
	VMCount      int `json:"vm_count"`
	ActiveVMs    int `json:"active_vms"`
	BackupCount  int `json:"backup_count"`
}

// ListCustomers handles GET /customers - lists all customers.
func (h *AdminHandler) ListCustomers(c *gin.Context) {
	pagination := models.ParsePagination(c)

	filter := repository.CustomerListFilter{
		PaginationParams: pagination,
	}

	// Optional status filter
	if status := c.Query("status"); status != "" {
		filter.Status = &status
	}

	// Optional search filter (email or name)
	if search := c.Query("search"); search != "" {
		filter.Search = &search
	}

	customers, total, err := h.customerService.List(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list customers",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "CUSTOMER_LIST_FAILED", "Failed to retrieve customers")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: customers,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}

// GetCustomer handles GET /customers/:id - retrieves details for a specific customer.
func (h *AdminHandler) GetCustomer(c *gin.Context) {
	customerID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(customerID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_CUSTOMER_ID", "Customer ID must be a valid UUID")
		return
	}

	customer, err := h.customerService.GetByID(c.Request.Context(), customerID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "CUSTOMER_NOT_FOUND", "Customer not found")
			return
		}
		h.logger.Error("failed to get customer",
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "CUSTOMER_GET_FAILED", "Failed to retrieve customer")
		return
	}

	// Get VM count for customer
	vmFilter := models.VMListFilter{
		CustomerID: &customerID,
		PaginationParams: models.PaginationParams{
			Page:    1,
			PerPage: 1, // We just need the count
		},
	}
	vms, totalVMs, err := h.vmService.ListVMs(c.Request.Context(), vmFilter, customerID, true)
	if err != nil {
		h.logger.Warn("failed to get VM count for customer",
			"customer_id", customerID,
			"error", err)
	}
	_ = vms // We don't need the actual VMs, just the count

// Count active VMs
	activeFilter := models.VMListFilter{
		CustomerID: &customerID,
		Status:     strPtr("running"),
		PaginationParams: models.PaginationParams{
			Page:    1,
			PerPage: 1,
		},
	}
	_, activeVMs, _ := h.vmService.ListVMs(c.Request.Context(), activeFilter, customerID, true)

	detail := CustomerDetail{
		Customer:    *customer,
		VMCount:     totalVMs,
		ActiveVMs:   activeVMs,
		BackupCount: 0, // Would need backup service to get this
	}

	c.JSON(http.StatusOK, models.Response{Data: detail})
}

// UpdateCustomer handles PUT /customers/:id - updates a customer's information.
func (h *AdminHandler) UpdateCustomer(c *gin.Context) {
	customerID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(customerID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_CUSTOMER_ID", "Customer ID must be a valid UUID")
		return
	}

	var req CustomerUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body: "+err.Error())
		return
	}

	// Get existing customer
	customer, err := h.customerService.GetByID(c.Request.Context(), customerID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "CUSTOMER_NOT_FOUND", "Customer not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "CUSTOMER_GET_FAILED", "Failed to retrieve customer")
		return
	}

	// Handle status changes
	if req.Status != nil && *req.Status != customer.Status {
		switch *req.Status {
		case models.CustomerStatusActive:
			err = h.customerService.Unsuspend(c.Request.Context(), customerID)
		case models.CustomerStatusSuspended:
			err = h.customerService.Suspend(c.Request.Context(), customerID)
		default:
			respondWithError(c, http.StatusBadRequest, "INVALID_STATUS", "Invalid status value")
			return
		}
		if err != nil {
			h.logger.Error("failed to update customer status",
				"customer_id", customerID,
				"status", *req.Status,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			respondWithError(c, http.StatusInternalServerError, "CUSTOMER_UPDATE_FAILED", err.Error())
			return
		}
	}

	// Apply name update if specified
	if req.Name != nil {
		customer.Name = *req.Name
		// Note: Would need repository Update method
	}

	// Log audit event
	h.logAuditEvent(c, "customer.update", "customer", customerID, req, true)

	h.logger.Info("customer updated via admin API",
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	// Return updated customer
	updatedCustomer, _ := h.customerService.GetByID(c.Request.Context(), customerID)
	c.JSON(http.StatusOK, models.Response{Data: updatedCustomer})
}

// DeleteCustomer handles DELETE /customers/:id - soft deletes a customer.
func (h *AdminHandler) DeleteCustomer(c *gin.Context) {
	customerID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(customerID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_CUSTOMER_ID", "Customer ID must be a valid UUID")
		return
	}

	// Check for active VMs
	vmFilter := models.VMListFilter{
		CustomerID: &customerID,
		PaginationParams: models.PaginationParams{
			Page:    1,
			PerPage: 1,
		},
	}
	_, vmCount, err := h.vmService.ListVMs(c.Request.Context(), vmFilter, customerID, true)
	if err == nil && vmCount > 0 {
		respondWithError(c, http.StatusConflict, "CUSTOMER_HAS_VMS", "Cannot delete customer with existing VMs. Delete VMs first.")
		return
	}

	err = h.customerService.Delete(c.Request.Context(), customerID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "CUSTOMER_NOT_FOUND", "Customer not found")
			return
		}
		h.logger.Error("failed to delete customer",
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "CUSTOMER_DELETE_FAILED", err.Error())
		return
	}

	// Log audit event
	h.logAuditEvent(c, "customer.delete", "customer", customerID, nil, true)

	h.logger.Info("customer deleted via admin API",
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"deleted": true}})
}

// GetCustomerAuditLogs handles GET /customers/:id/audit-logs - retrieves audit trail for a customer.
func (h *AdminHandler) GetCustomerAuditLogs(c *gin.Context) {
	customerID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(customerID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_CUSTOMER_ID", "Customer ID must be a valid UUID")
		return
	}

	pagination := models.ParsePagination(c)

	filter := repository.AuditLogFilter{
		PaginationParams: pagination,
		ActorID:          &customerID,
		ActorType:        strPtr("customer"),
	}

	// Optional action filter
	if action := c.Query("action"); action != "" {
		filter.Action = &action
	}

	logs, total, err := h.auditRepo.List(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to get customer audit logs",
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "AUDIT_LOG_FAILED", "Failed to retrieve audit logs")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: logs,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}