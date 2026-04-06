package customer

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

const (
	customerOAuthStateCookiePath   = "/api/v1/customer/auth/oauth/"
	customerOAuthStateCookieMaxAge = 600
)

type oauthStateCookiePayload struct {
	Provider  string `json:"provider"`
	State     string `json:"state"`
	ExpiresAt int64  `json:"expires_at"`
}

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

	if err := h.setOAuthStateCookie(c, provider, state); err != nil {
		h.logger.Error("failed to set oauth state cookie",
			"provider", provider, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"OAUTH_ERROR", "Failed to start OAuth flow")
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

	if err := h.validateOAuthStateCookie(c, provider, req.State); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"OAUTH_ERROR", "Invalid or expired OAuth state")
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

func (h *CustomerHandler) setOAuthStateCookie(c *gin.Context, provider, state string) error {
	value, err := h.signOAuthStateCookie(provider, state)
	if err != nil {
		return err
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthStateCookieName(provider),
		Value:    value,
		Path:     customerOAuthStateCookiePath,
		MaxAge:   customerOAuthStateCookieMaxAge,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	return nil
}

func (h *CustomerHandler) validateOAuthStateCookie(c *gin.Context, provider, state string) error {
	defer h.clearOAuthStateCookie(c, provider)

	cookieValue, err := c.Cookie(oauthStateCookieName(provider))
	if err != nil {
		return err
	}

	payload, err := h.parseOAuthStateCookie(provider, cookieValue)
	if err != nil {
		return err
	}

	if payload.State != state {
		return fmt.Errorf("oauth state mismatch")
	}

	return nil
}

func (h *CustomerHandler) clearOAuthStateCookie(c *gin.Context, provider string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oauthStateCookieName(provider),
		Value:    "",
		Path:     customerOAuthStateCookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func (h *CustomerHandler) signOAuthStateCookie(provider, state string) (string, error) {
	payload := oauthStateCookiePayload{
		Provider:  provider,
		State:     state,
		ExpiresAt: time.Now().Add(customerOAuthStateCookieMaxAge * time.Second).Unix(),
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal oauth state cookie: %w", err)
	}

	payloadEncoded := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signature := oauthStateSignature(h.authConfig.JWTSecret, payloadEncoded)
	signatureEncoded := base64.RawURLEncoding.EncodeToString(signature)

	return payloadEncoded + "." + signatureEncoded, nil
}

func (h *CustomerHandler) parseOAuthStateCookie(provider, value string) (*oauthStateCookiePayload, error) {
	payloadEncoded, signatureEncoded, ok := strings.Cut(value, ".")
	if !ok {
		return nil, fmt.Errorf("invalid oauth state cookie format")
	}

	signature, err := base64.RawURLEncoding.DecodeString(signatureEncoded)
	if err != nil {
		return nil, fmt.Errorf("decode oauth state cookie signature: %w", err)
	}

	expectedSignature := oauthStateSignature(h.authConfig.JWTSecret, payloadEncoded)
	if !hmac.Equal(signature, expectedSignature) {
		return nil, fmt.Errorf("invalid oauth state cookie signature")
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(payloadEncoded)
	if err != nil {
		return nil, fmt.Errorf("decode oauth state cookie payload: %w", err)
	}

	var payload oauthStateCookiePayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal oauth state cookie payload: %w", err)
	}

	if payload.Provider != provider {
		return nil, fmt.Errorf("oauth state cookie provider mismatch")
	}
	if time.Now().Unix() > payload.ExpiresAt {
		return nil, fmt.Errorf("oauth state cookie expired")
	}

	return &payload, nil
}

func oauthStateCookieName(provider string) string {
	return "customer_oauth_state_" + provider
}

func oauthStateSignature(secret, payload string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
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
