package provisioning

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

type CreateSSOTokenRequest struct {
	VMID              string `json:"vm_id,omitempty" validate:"omitempty,uuid"`
	ExternalServiceID *int   `json:"external_service_id,omitempty" validate:"omitempty,gt=0"`
	CustomerID        string `json:"customer_id,omitempty" validate:"omitempty,uuid"`
	ExternalClientID  *int   `json:"external_client_id,omitempty" validate:"omitempty,gt=0"`
}

type CreateSSOTokenResponse struct {
	Token        string `json:"token"`
	VMID         string `json:"vm_id,omitempty"`
	RedirectPath string `json:"redirect_path"`
	ExpiresAt    string `json:"expires_at"`
}

// CreateSSOToken issues a short-lived opaque token for billing system browser SSO.
// @Tags Provisioning
// @Summary Create SSO token
// @Description Creates one-time SSO token for customer portal login from billing system.
// @Accept json
// @Produce json
// @Security APIKeyAuth
// @Param request body object true "Create SSO token request"
// @Success 201 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/provisioning/sso-tokens [post]
func (h *ProvisioningHandler) CreateSSOToken(c *gin.Context) {
	var req CreateSSOTokenRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}
	customerID, vmID, redirectPath, err := h.resolveSSOTarget(c, req)
	if err != nil {
		var validationErr vmValidationError
		if errors.As(err, &validationErr) {
			middleware.RespondWithError(c, validationErr.status, validationErr.errCode, validationErr.errMsg)
			return
		}
		if errors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "SSO_TARGET_NOT_FOUND", "SSO target not found")
			return
		}
		h.logger.Error("failed to resolve sso token target",
			"vm_id", req.VMID,
			"external_service_id", req.ExternalServiceID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SSO_TOKEN_FAILED", "Failed to issue SSO token")
		return
	}

	token, err := middleware.GenerateRefreshToken()
	if err != nil {
		h.logger.Error("failed to generate opaque sso token", "error", err, "correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SSO_TOKEN_FAILED", "Failed to issue SSO token")
		return
	}

	tokenHash := sha256.Sum256([]byte(token))
	expiresAt := time.Now().Add(models.SSOTokenTTL)
	record := &models.SSOToken{
		TokenHash:    tokenHash[:],
		CustomerID:   customerID,
		VMID:         vmID,
		RedirectPath: redirectPath,
		ExpiresAt:    expiresAt,
	}
	if err := h.ssoTokenRepo.Create(c.Request.Context(), record); err != nil {
		h.logger.Error("failed to persist opaque sso token", "vm_id", vmID, "error", err, "correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SSO_TOKEN_FAILED", "Failed to issue SSO token")
		return
	}

	h.logSSOTokenIssued(c, record)

	c.JSON(http.StatusCreated, models.Response{Data: CreateSSOTokenResponse{
		Token:        token,
		VMID:         optionalStringValue(vmID),
		RedirectPath: redirectPath,
		ExpiresAt:    expiresAt.Format(time.RFC3339),
	}})
}

func (h *ProvisioningHandler) resolveSSOTarget(c *gin.Context, req CreateSSOTokenRequest) (string, *string, string, error) {
	if req.VMID == "" && req.ExternalServiceID == nil {
		return h.resolveCustomerOnlySSO(c, req)
	}
	if req.VMID == "" {
		return "", nil, "", vmValidationError{
			status:  http.StatusBadRequest,
			errCode: "VM_ID_REQUIRED",
			errMsg:  "vm_id is required for VM-scoped SSO",
		}
	}
	if req.ExternalServiceID == nil {
		return "", nil, "", vmValidationError{
			status:  http.StatusBadRequest,
			errCode: "EXTERNAL_SERVICE_ID_REQUIRED",
			errMsg:  "external_service_id is required for VM-scoped SSO",
		}
	}
	if !hasSSOOwnerAssertion(req) {
		return "", nil, "", vmValidationError{
			status:  http.StatusBadRequest,
			errCode: "OWNERSHIP_ASSERTION_REQUIRED",
			errMsg:  "customer_id or external_client_id is required",
		}
	}
	vm, err := h.lookupVMForSSO(c, req)
	if err != nil {
		return "", nil, "", err
	}
	if !matchesSSOVMAssertion(vm, req) {
		return "", nil, "", sharederrors.ErrNotFound
	}
	if req.CustomerID != "" && req.CustomerID != vm.CustomerID {
		return "", nil, "", sharederrors.ErrNotFound
	}
	if err := h.validateSSOExternalClient(c, vm.CustomerID, req.ExternalClientID); err != nil {
		return "", nil, "", err
	}
	return vm.CustomerID, &vm.ID, "/vms/" + vm.ID, nil
}

func hasSSOOwnerAssertion(req CreateSSOTokenRequest) bool {
	return req.CustomerID != "" || req.ExternalClientID != nil
}

func matchesSSOVMAssertion(vm *models.VM, req CreateSSOTokenRequest) bool {
	return req.ExternalServiceID != nil && vm.ExternalServiceID != nil && *vm.ExternalServiceID == *req.ExternalServiceID
}

func (h *ProvisioningHandler) resolveCustomerOnlySSO(c *gin.Context, req CreateSSOTokenRequest) (string, *string, string, error) {
	if req.CustomerID == "" {
		return "", nil, "", sharederrors.ErrNotFound
	}
	if err := h.validateSSOExternalClient(c, req.CustomerID, req.ExternalClientID); err != nil {
		return "", nil, "", err
	}
	if req.ExternalClientID == nil {
		if _, err := h.customerRepo.GetByID(c.Request.Context(), req.CustomerID); err != nil {
			return "", nil, "", err
		}
	}
	return req.CustomerID, nil, "/vms", nil
}

func (h *ProvisioningHandler) lookupVMForSSO(c *gin.Context, req CreateSSOTokenRequest) (*models.VM, error) {
	vm, err := h.vmRepo.GetByID(c.Request.Context(), req.VMID)
	if err != nil {
		return nil, err
	}
	return vm, nil
}

func (h *ProvisioningHandler) validateSSOExternalClient(c *gin.Context, customerID string, externalClientID *int) error {
	if externalClientID == nil {
		return nil
	}
	customer, err := h.customerRepo.GetByID(c.Request.Context(), customerID)
	if err != nil {
		return err
	}
	if customer.ExternalClientID == nil || *customer.ExternalClientID != *externalClientID {
		return sharederrors.ErrNotFound
	}
	return nil
}

func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (h *ProvisioningHandler) logSSOTokenIssued(c *gin.Context, token *models.SSOToken) {
	if h.auditRepo == nil {
		return
	}
	apiKeyID := ""
	if value, exists := c.Get("api_key_id"); exists {
		if s, ok := value.(string); ok {
			apiKeyID = s
		}
	}
	changes, _ := json.Marshal(gin.H{
		"vm_id":         token.VMID,
		"customer_id":   token.CustomerID,
		"redirect_path": token.RedirectPath,
		"expires_at":    token.ExpiresAt,
		"api_key_id":    apiKeyID,
	})
	correlationID := middleware.GetCorrelationID(c)
	audit := &models.AuditLog{
		ActorID:       &apiKeyID,
		ActorType:     models.AuditActorProvisioning,
		Action:        "sso.issue",
		ResourceType:  "sso_token",
		ResourceID:    &token.ID,
		Changes:       changes,
		CorrelationID: &correlationID,
		Success:       true,
	}
	if err := h.auditRepo.Append(c.Request.Context(), audit); err != nil {
		h.logger.Warn("failed to write sso issuance audit log", "token_id", token.ID, "error", err, "correlation_id", correlationID)
	}
}
