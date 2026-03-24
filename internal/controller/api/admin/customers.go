package admin

import (
	"errors"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
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
	VMCount     int `json:"vm_count"`
	ActiveVMs   int `json:"active_vms"`
	BackupCount int `json:"backup_count"`
}

// CustomerCreateRequest represents the request body for creating a customer.
type CustomerCreateRequest struct {
	Name     string  `json:"name" validate:"required,max=255"`
	Email    string  `json:"email" validate:"required,email,max=254"`
	Password string  `json:"password" validate:"required,min=12,max=128"`
	Phone    *string `json:"phone,omitempty" validate:"omitempty,max=20"`
}

// CreateCustomer handles POST /customers - creates a new customer.
func (h *AdminHandler) CreateCustomer(c *gin.Context) {
	var req CustomerCreateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Check if email already exists
	existingCustomer, err := h.customerService.GetByEmail(c.Request.Context(), req.Email)
	if err == nil && existingCustomer != nil {
		middleware.RespondWithError(c, http.StatusConflict, "EMAIL_EXISTS", "A customer with this email already exists")
		return
	}
	if err != nil && !sharederrors.Is(err, sharederrors.ErrNotFound) {
		h.logger.Error("failed to check existing email",
			"email", req.Email,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "CUSTOMER_CREATE_FAILED", "Internal server error")
		return
	}

	// Hash the password
	passwordHash, err := h.authService.HashPassword(req.Password)
	if err != nil {
		h.logger.Error("failed to hash password",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "PASSWORD_HASH_FAILED", "Failed to process password")
		return
	}

	// Create the customer
	customer := &models.Customer{
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: passwordHash,
		Phone:        req.Phone,
		Status:       models.CustomerStatusActive,
	}

	actorID := middleware.GetUserID(c)
	actorIP := c.ClientIP()

	createdCustomer, err := h.customerService.Create(c.Request.Context(), actorID, actorIP, customer)
	if err != nil {
		h.logger.Error("failed to create customer",
			"email", req.Email,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "CUSTOMER_CREATE_FAILED", "Failed to create customer")
		return
	}

	// Log audit event
	h.logAuditEvent(c, "customer.create", "customer", createdCustomer.ID, req, true)

	h.logger.Info("customer created via admin API",
		"customer_id", createdCustomer.ID,
		"email", createdCustomer.Email,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusCreated, models.Response{Data: createdCustomer})
}

// ListCustomers handles GET /customers - lists all customers.
func (h *AdminHandler) ListCustomers(c *gin.Context) {
	pagination := models.ParsePagination(c)

	filter := repository.CustomerListFilter{
		PaginationParams: pagination,
	}

	// Optional status filter
	validCustomerStatuses := map[string]bool{
		"active": true, "suspended": true, "deleted": true,
	}
	if status := c.Query("status"); status != "" {
		if !validCustomerStatuses[status] {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_STATUS", "Invalid status value")
			return
		}
		filter.Status = &status
	}

	// Optional search filter (email or name)
	if search := c.Query("search"); search != "" {
		if len(search) > 100 {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_SEARCH", "search parameter must not exceed 100 characters")
			return
		}
		filter.Search = &search
	}

	customers, total, err := h.customerService.List(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list customers",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "CUSTOMER_LIST_FAILED", "Failed to retrieve customers")
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
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_CUSTOMER_ID", "Customer ID must be a valid UUID")
		return
	}

	customer, err := h.customerService.GetByID(c.Request.Context(), customerID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "CUSTOMER_NOT_FOUND", "Customer not found")
			return
		}
		h.logger.Error("failed to get customer",
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "CUSTOMER_GET_FAILED", "Failed to retrieve customer")
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
	_, totalVMs, err := h.vmService.ListVMs(c.Request.Context(), vmFilter, customerID, true)
	if err != nil {
		h.logger.Warn("failed to get VM count for customer",
			"customer_id", customerID,
			"error", err)
	}

	// Count active VMs
	activeFilter := models.VMListFilter{
		CustomerID: &customerID,
		Status:     util.StringPtr("running"),
		PaginationParams: models.PaginationParams{
			Page:    1,
			PerPage: 1,
		},
	}
	_, activeVMs, err := h.vmService.ListVMs(c.Request.Context(), activeFilter, customerID, true)
	if err != nil {
		h.logger.Warn("failed to get active VM count for customer",
			"customer_id", customerID,
			"error", err)
	}

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
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_CUSTOMER_ID", "Customer ID must be a valid UUID")
		return
	}

	var req CustomerUpdateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Get existing customer
	customer, err := h.customerService.GetByID(c.Request.Context(), customerID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "CUSTOMER_NOT_FOUND", "Customer not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "CUSTOMER_GET_FAILED", "Failed to retrieve customer")
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
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_STATUS", "Invalid status value")
			return
		}
		if err != nil {
			h.logger.Error("failed to update customer status",
				"customer_id", customerID,
				"status", *req.Status,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			middleware.RespondWithError(c, http.StatusInternalServerError, "CUSTOMER_UPDATE_FAILED", "Internal server error")
			return
		}
	}

	// Apply name update if specified
	if req.Name != nil {
		customer.Name = *req.Name
		actorIP := c.ClientIP()
		actorID := middleware.GetUserID(c)
		if err := h.customerService.Update(c.Request.Context(), actorID, actorIP, customer); err != nil {
			h.logger.Error("failed to update customer profile",
				"customer_id", customerID,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			middleware.RespondWithError(c, http.StatusInternalServerError, "CUSTOMER_UPDATE_FAILED", "Internal server error")
			return
		}
	}

	// Log audit event
	h.logAuditEvent(c, "customer.update", "customer", customerID, req, true)

	h.logger.Info("customer updated via admin API",
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	// Return updated customer
	updatedCustomer, err := h.customerService.GetByID(c.Request.Context(), customerID)
	if err != nil {
		h.logger.Error("failed to fetch updated customer after update",
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "CUSTOMER_GET_FAILED", "Failed to retrieve updated customer")
		return
	}
	c.JSON(http.StatusOK, models.Response{Data: updatedCustomer})
}

// DeleteCustomer handles DELETE /customers/:id - soft deletes a customer.
func (h *AdminHandler) DeleteCustomer(c *gin.Context) {
	customerID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(customerID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_CUSTOMER_ID", "Customer ID must be a valid UUID")
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
		middleware.RespondWithError(c, http.StatusConflict, "CUSTOMER_HAS_VMS", "Cannot delete customer with existing VMs. Delete VMs first.")
		return
	}

	err = h.customerService.Delete(c.Request.Context(), customerID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "CUSTOMER_NOT_FOUND", "Customer not found")
			return
		}
		h.logger.Error("failed to delete customer",
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "CUSTOMER_DELETE_FAILED", "Internal server error")
		return
	}

	// Log audit event
	h.logAuditEvent(c, "customer.delete", "customer", customerID, nil, true)

	h.logger.Info("customer deleted via admin API",
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.Status(http.StatusNoContent)
}

// GetCustomerAuditLogs handles GET /customers/:id/audit-logs - retrieves audit trail for a customer.
func (h *AdminHandler) GetCustomerAuditLogs(c *gin.Context) {
	customerID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(customerID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_CUSTOMER_ID", "Customer ID must be a valid UUID")
		return
	}

	pagination := models.ParsePagination(c)

	filter := models.AuditLogFilter{
		PaginationParams: pagination,
		ActorID:          &customerID,
		ActorType:        util.StringPtr("customer"),
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
		middleware.RespondWithError(c, http.StatusInternalServerError, "AUDIT_LOG_FAILED", "Failed to retrieve audit logs")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: logs,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}
