package customer

import (
	"errors"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// OAuthAuthorize handles GET /auth/oauth/:provider/authorize.
// Redirects the browser to the OAuth provider's consent page with PKCE.
func (h *CustomerHandler) OAuthAuthorize(c *gin.Context) {
	provider := c.Param("provider")
	if !models.IsValidOAuthProvider(provider) {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_PROVIDER", "Unsupported OAuth provider")
		return
	}

	codeChallenge := c.Query("code_challenge")
	state := c.Query("state")
	redirectURI := c.Query("redirect_uri")

	if codeChallenge == "" || state == "" || redirectURI == "" {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"VALIDATION_ERROR", "Missing code_challenge, state, or redirect_uri")
		return
	}

	authURL, err := h.oauthService.GetAuthorizationURL(
		provider, codeChallenge, state, redirectURI)
	if err != nil {
		h.logger.Error("failed to generate oauth authorization URL",
			"provider", provider, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusBadRequest,
			"OAUTH_ERROR", err.Error())
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

// OAuthCallback handles POST /auth/oauth/:provider/callback.
// Exchanges the authorization code for tokens, resolves the customer,
// and returns JWT tokens.
func (h *CustomerHandler) OAuthCallback(c *gin.Context) {
	provider := c.Param("provider")
	if !models.IsValidOAuthProvider(provider) {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_PROVIDER", "Unsupported OAuth provider")
		return
	}

	var req models.OAuthCallbackRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest,
			"VALIDATION_ERROR", "Invalid callback request")
		return
	}

	authTokens, refreshToken, err := h.oauthService.HandleCallback(
		c.Request.Context(), provider,
		req.Code, req.CodeVerifier, req.RedirectURI,
		c.ClientIP(), c.Request.UserAgent(),
	)
	if err != nil {
		h.logger.Error("oauth callback failed",
			"provider", provider, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		var valErr *sharederrors.ValidationError
		if errors.As(err, &valErr) {
			middleware.RespondWithError(c, http.StatusBadRequest,
				"OAUTH_ERROR", valErr.Error())
			return
		}
		if sharederrors.Is(err, sharederrors.ErrForbidden) {
			middleware.RespondWithError(c, http.StatusForbidden,
				"ACCOUNT_SUSPENDED", "Account is suspended or deleted")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"OAUTH_ERROR", "OAuth authentication failed")
		return
	}

	resp := AuthResponse{
		TokenType:           authTokens.TokenType,
		ExpiresIn:           authTokens.ExpiresIn,
		SessionID:           authTokens.SessionID,
		SessionCleanupToken: authTokens.SessionCleanupToken,
	}
	customer, userErr := h.lookupAuthenticatedCustomer(c, authTokens.AccessToken)
	if userErr != nil {
		h.rollbackIssuedSession(c.Request.Context(), authTokens.SessionID,
			"failed to roll back oauth session after customer lookup",
			middleware.GetCorrelationID(c))
		if sharederrors.Is(userErr, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "NOT_FOUND", "customer not found")
			return
		}
		h.logger.Error("failed to load oauth auth response user",
			"provider", provider,
			"error", userErr,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"OAUTH_ERROR", "OAuth authentication failed")
		return
	}
	resp.User = buildAuthenticatedCustomerResponse(customer)

	middleware.SetAuthCookies(c, authTokens.AccessToken,
		refreshToken, middleware.AccessTokenMaxAge,
		middleware.RefreshTokenMaxAge,
		customerRefreshCookiePath)

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

// ListOAuthLinks handles GET /account/oauth.
// Returns the customer's linked OAuth providers.
func (h *CustomerHandler) ListOAuthLinks(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	links, err := h.oauthService.GetLinkedAccounts(c.Request.Context(), customerID)
	if err != nil {
		h.logger.Error("failed to list oauth links",
			"customer_id", customerID, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"OAUTH_ERROR", "Failed to list linked accounts")
		return
	}

	if links == nil {
		links = []*models.OAuthLink{}
	}

	c.JSON(http.StatusOK, models.Response{Data: links})
}

// LinkOAuthAccount handles POST /account/oauth/:provider/link.
// Links a new OAuth provider to the authenticated customer's account.
func (h *CustomerHandler) LinkOAuthAccount(c *gin.Context) {
	provider := c.Param("provider")
	if !models.IsValidOAuthProvider(provider) {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_PROVIDER", "Unsupported OAuth provider")
		return
	}

	var req models.OAuthCallbackRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest,
			"VALIDATION_ERROR", "Invalid request")
		return
	}

	customerID := middleware.GetUserID(c)
	err := h.oauthService.LinkAccount(
		c.Request.Context(), customerID, provider,
		req.Code, req.CodeVerifier, req.RedirectURI,
	)
	if err != nil {
		h.logger.Error("failed to link oauth account",
			"customer_id", customerID, "provider", provider,
			"error", err, "correlation_id", middleware.GetCorrelationID(c))

		var valErr *sharederrors.ValidationError
		if errors.As(err, &valErr) {
			middleware.RespondWithError(c, http.StatusConflict,
				"LINK_FAILED", valErr.Error())
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"LINK_FAILED", "Failed to link OAuth account")
		return
	}

	h.logAudit(c, "oauth.link", "oauth_link", provider, map[string]any{
		"provider": provider,
	}, true)

	c.JSON(http.StatusOK, models.Response{
		Data: gin.H{"message": "OAuth account linked successfully"},
	})
}

// UnlinkOAuthAccount handles DELETE /account/oauth/:provider.
// Removes an OAuth provider link from the authenticated customer's account.
func (h *CustomerHandler) UnlinkOAuthAccount(c *gin.Context) {
	provider := c.Param("provider")
	if !models.IsValidOAuthProvider(provider) {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_PROVIDER", "Unsupported OAuth provider")
		return
	}

	customerID := middleware.GetUserID(c)
	err := h.oauthService.UnlinkAccount(
		c.Request.Context(), customerID, provider)
	if err != nil {
		h.logger.Error("failed to unlink oauth account",
			"customer_id", customerID, "provider", provider,
			"error", err, "correlation_id", middleware.GetCorrelationID(c))

		var valErr *sharederrors.ValidationError
		if errors.As(err, &valErr) {
			middleware.RespondWithError(c, http.StatusBadRequest,
				"UNLINK_FAILED", valErr.Error())
			return
		}
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound,
				"NOT_FOUND", "OAuth link not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"UNLINK_FAILED", "Failed to unlink OAuth account")
		return
	}

	h.logAudit(c, "oauth.unlink", "oauth_link", provider, map[string]any{
		"provider": provider,
	}, true)

	c.JSON(http.StatusOK, models.Response{
		Data: gin.H{"message": "OAuth account unlinked successfully"},
	})
}
