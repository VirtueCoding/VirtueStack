package provisioning

import (
	"errors"
	"net/http"
	"strings"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
	"github.com/gin-gonic/gin"
)

// CreateCustomerRequest represents the request body for creating a customer
// via the provisioning API. Only email and name are required; a random
// password is generated automatically because the billing module manages auth via SSO.
type CreateCustomerRequest struct {
	Email            string  `json:"email" validate:"required,email,max=254"`
	Name             string  `json:"name" validate:"required,max=255"`
	ExternalClientID *int    `json:"external_client_id,omitempty" validate:"omitempty,gt=0"`
	BillingProvider  *string `json:"billing_provider,omitempty" validate:"omitempty,oneof=whmcs blesta native"`
}

// CreateCustomerResponse represents the response for customer creation or lookup.
type CreateCustomerResponse struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Created bool   `json:"created"`
}

// CreateOrGetCustomer handles POST /customers — idempotent create-or-get by email.
// If a customer with the given email already exists, the existing customer is
// returned (with created=false). Otherwise a new customer is created with a
// random password and returned (with created=true).
// @Tags Provisioning
// @Summary Create or get customer
// @Description Creates a customer account or returns existing customer by email for provisioning workflow.
// @Accept json
// @Produce json
// @Security APIKeyAuth
// @Param request body object true "Create or get customer request"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Router /api/v1/provisioning/customers [post]
func (h *ProvisioningHandler) CreateOrGetCustomer(c *gin.Context) {
	var req CreateCustomerRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Name = strings.TrimSpace(req.Name)

	ctx := c.Request.Context()
	correlationID := middleware.GetCorrelationID(c)

	existing, err := h.customerRepo.GetByEmail(ctx, req.Email)
	if err == nil && existing != nil {
		h.updateExternalClientID(c, existing, req.ExternalClientID)
		h.logger.Info("existing customer returned via provisioning API",
			"customer_id", existing.ID,
			"correlation_id", correlationID)
		c.JSON(http.StatusOK, models.Response{Data: CreateCustomerResponse{
			ID:      existing.ID,
			Email:   existing.Email,
			Name:    existing.Name,
			Created: false,
		}})
		return
	}
	if err != nil && !sharederrors.Is(err, sharederrors.ErrNotFound) {
		h.logger.Error("failed to look up customer by email",
			"error", err,
			"correlation_id", correlationID)
		middleware.RespondWithError(c, http.StatusInternalServerError, "CUSTOMER_LOOKUP_FAILED", "Internal server error")
		return
	}

	password, err := generateRandomPassword()
	if err != nil {
		h.logger.Error("failed to generate random password for customer",
			"error", err,
			"correlation_id", correlationID)
		middleware.RespondWithError(c, http.StatusInternalServerError, "PASSWORD_GENERATION_FAILED", "Internal server error")
		return
	}

	passwordHash, err := h.authService.HashPassword(password)
	if err != nil {
		h.logger.Error("failed to hash password for customer",
			"error", err,
			"correlation_id", correlationID)
		middleware.RespondWithError(c, http.StatusInternalServerError, "PASSWORD_HASH_FAILED", "Internal server error")
		return
	}

	billingProvider := models.BillingProviderWHMCS
	if req.BillingProvider != nil {
		billingProvider = *req.BillingProvider
	}

	customer := &models.Customer{
		Email:            req.Email,
		Name:             req.Name,
		PasswordHash:     &passwordHash,
		AuthProvider:     models.AuthProviderLocal,
		ExternalClientID: req.ExternalClientID,
		BillingProvider:  util.StringPtr(billingProvider),
		Status:           models.CustomerStatusActive,
	}

	actorID := "provisioning"
	actorIP := c.ClientIP()

	created, err := h.customerService.Create(ctx, actorID, actorIP, customer)
	if err != nil {
		h.logger.Error("failed to create customer",
			"email", req.Email,
			"error", err,
			"correlation_id", correlationID)
		middleware.RespondWithError(c, http.StatusInternalServerError, "CUSTOMER_CREATE_FAILED", "Failed to create customer")
		return
	}

	h.logger.Info("customer created via provisioning API",
		"customer_id", created.ID,
		"correlation_id", correlationID)
	c.JSON(http.StatusCreated, models.Response{Data: CreateCustomerResponse{
		ID:      created.ID,
		Email:   created.Email,
		Name:    created.Name,
		Created: true,
	}})
}

// updateExternalClientID sets external_client_id on an existing customer if the
// request carries one and the customer does not already have one set.
func (h *ProvisioningHandler) updateExternalClientID(c *gin.Context, customer *models.Customer, externalClientID *int) {
	if externalClientID == nil {
		return
	}
	if customer.ExternalClientID != nil {
		if *customer.ExternalClientID == *externalClientID {
			return
		}
		h.logger.Warn("external_client_id mismatch on existing customer",
			"customer_id", customer.ID,
			"existing_external_client_id", *customer.ExternalClientID,
			"requested_external_client_id", *externalClientID,
			"correlation_id", middleware.GetCorrelationID(c))
		return
	}
	if err := h.customerRepo.UpdateExternalClientID(c.Request.Context(), customer.ID, *externalClientID); err != nil {
		h.logger.Error("failed to update external_client_id on existing customer",
			"customer_id", customer.ID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
	}
}
