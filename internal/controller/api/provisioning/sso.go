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
	VMID           string `json:"vm_id,omitempty" validate:"omitempty,uuid"`
	WHMCSServiceID *int   `json:"whmcs_service_id,omitempty" validate:"omitempty,gt=0"`
}

type CreateSSOTokenResponse struct {
	Token        string `json:"token"`
	VMID         string `json:"vm_id"`
	RedirectPath string `json:"redirect_path"`
	ExpiresAt    string `json:"expires_at"`
}

// CreateSSOToken issues a short-lived opaque token for WHMCS browser SSO.
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
	if req.VMID == "" && req.WHMCSServiceID == nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Either vm_id or whmcs_service_id is required")
		return
	}

	vm, err := h.lookupVMForSSO(c, req)
	if err != nil {
		if errors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to resolve vm for sso token",
			"vm_id", req.VMID,
			"whmcs_service_id", req.WHMCSServiceID,
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
	redirectPath := "/vms/" + vm.ID
	record := &models.SSOToken{
		TokenHash:    tokenHash[:],
		CustomerID:   vm.CustomerID,
		VMID:         vm.ID,
		RedirectPath: redirectPath,
		ExpiresAt:    expiresAt,
	}
	if err := h.ssoTokenRepo.Create(c.Request.Context(), record); err != nil {
		h.logger.Error("failed to persist opaque sso token", "vm_id", vm.ID, "error", err, "correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SSO_TOKEN_FAILED", "Failed to issue SSO token")
		return
	}

	h.logSSOTokenIssued(c, record)

	c.JSON(http.StatusCreated, models.Response{Data: CreateSSOTokenResponse{
		Token:        token,
		VMID:         vm.ID,
		RedirectPath: redirectPath,
		ExpiresAt:    expiresAt.Format(time.RFC3339),
	}})
}

func (h *ProvisioningHandler) lookupVMForSSO(c *gin.Context, req CreateSSOTokenRequest) (*models.VM, error) {
	if req.VMID != "" {
		vm, err := h.vmRepo.GetByID(c.Request.Context(), req.VMID)
		if err != nil {
			return nil, err
		}
		return vm, nil
	}
	return h.vmRepo.GetByWHMCSServiceID(c.Request.Context(), *req.WHMCSServiceID)
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
		"redirect_path": token.RedirectPath,
		"expires_at":    token.ExpiresAt,
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
