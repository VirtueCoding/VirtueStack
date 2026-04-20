package customer

import (
	"crypto/sha256"
	"encoding/json"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// ExchangeSSOToken consumes a one-time opaque WHMCS SSO token, mints the normal
// browser session cookies, and redirects to the clean customer WebUI URL.
// @Tags Customer
// @Summary Exchange SSO token
// @Description Exchanges one-time provisioning SSO token for customer session cookies.
// @Produce json
// @Param token query string true "One-time SSO token"
// @Success 302 {string} string "Redirect to customer portal"
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Router /api/v1/customer/auth/sso-exchange [get]
func (h *CustomerHandler) ExchangeSSOToken(c *gin.Context) {
	rawToken := c.Query("token")
	if rawToken == "" {
		middleware.RespondWithError(c, http.StatusBadRequest, "MISSING_TOKEN", "SSO token is required")
		return
	}

	tokenHash := sha256.Sum256([]byte(rawToken))
	record, err := h.ssoTokenRepo.ConsumeByHash(c.Request.Context(), tokenHash[:])
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusUnauthorized, "INVALID_TOKEN", "SSO token is invalid or expired")
			return
		}
		h.logger.Error("failed to consume sso token", "error", err, "correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SSO_EXCHANGE_FAILED", "Failed to exchange SSO token")
		return
	}

	tokens, refreshToken, err := h.authService.CreateCustomerSSOSession(c.Request.Context(), record.CustomerID, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		h.logger.Error("failed to create sso session",
			"customer_id", record.CustomerID,
			"vm_id", record.VMID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusUnauthorized, "SSO_EXCHANGE_FAILED", "Unable to create browser session")
		return
	}

	middleware.SetAuthCookies(
		c,
		tokens.AccessToken,
		refreshToken,
		middleware.AccessTokenMaxAge,
		middleware.RefreshTokenMaxAge,
		"/api/v1/customer/auth/refresh",
	)
	c.Header("Cache-Control", "no-store")
	h.logSSOTokenRedeemed(c, record)
	c.Redirect(http.StatusSeeOther, record.RedirectPath)
}

func (h *CustomerHandler) logSSOTokenRedeemed(c *gin.Context, token *models.SSOToken) {
	if h.auditRepo == nil {
		return
	}
	changes, _ := json.Marshal(gin.H{
		"vm_id":         token.VMID,
		"redirect_path": token.RedirectPath,
		"expires_at":    token.ExpiresAt,
	})
	customerID := token.CustomerID
	correlationID := middleware.GetCorrelationID(c)
	clientIP := c.ClientIP()
	audit := &models.AuditLog{
		ActorID:       &customerID,
		ActorType:     models.AuditActorCustomer,
		ActorIP:       &clientIP,
		Action:        "sso.redeem",
		ResourceType:  "sso_token",
		ResourceID:    &token.ID,
		Changes:       changes,
		CorrelationID: &correlationID,
		Success:       true,
	}
	if err := h.auditRepo.Append(c.Request.Context(), audit); err != nil {
		h.logger.Warn("failed to write sso redemption audit log", "token_id", token.ID, "error", err, "correlation_id", correlationID)
	}
}
