package customer

import (
	"context"
	"errors"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
	"github.com/gin-gonic/gin"
)

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email,max=254"`
	Password string `json:"password" validate:"required,min=12,max=128"`
}

type Verify2FARequest struct {
	TempToken string `json:"temp_token" validate:"required"`
	TOTPCode  string `json:"totp_code" validate:"required,len=6,numeric"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required,min=12,max=128"`
	NewPassword     string `json:"new_password" validate:"required,min=12,max=128"`
}

type AuthResponse struct {
	TokenType           string                         `json:"token_type"`
	ExpiresIn           int                            `json:"expires_in,omitempty"`
	Requires2FA         bool                           `json:"requires_2fa,omitempty"`
	TempToken           string                         `json:"temp_token,omitempty"`
	SessionID           string                         `json:"session_id,omitempty"`
	SessionCleanupToken string                         `json:"session_cleanup_token,omitempty"`
	User                *AuthenticatedCustomerResponse `json:"user,omitempty"`
}

// AuthenticatedCustomerResponse contains the customer fields returned after authentication.
type AuthenticatedCustomerResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// LogoutRequest carries the cleanup token used to revoke the current customer session.
type LogoutRequest struct {
	SessionCleanupToken string `json:"session_cleanup_token"`
}

const customerRefreshCookiePath = "/api/v1/customer/auth/refresh"

// CSRF handles GET /customer/auth/csrf - returns the CSRF token in the response header.
// The CSRF middleware sets the cookie before this handler runs.
// @Tags Customer
// @Summary Get CSRF token
// @Description Issues CSRF cookie/token pair for customer frontend authentication flows.
// @Produce json
// @Success 200 {object} models.Response
// @Router /api/v1/customer/auth/csrf [get]
func (h *CustomerHandler) CSRF(c *gin.Context) {
	c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "CSRF token set"}})
}

func (h *CustomerHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	tokens, refreshToken, err := h.authService.Login(c.Request.Context(), req.Email, req.Password, ipAddress, userAgent)
	if err != nil {
		h.logger.Warn("customer login failed",
			"email", util.MaskEmail(req.Email),
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		if sharederrors.Is(err, sharederrors.ErrUnauthorized) {
			middleware.RespondWithError(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
			return
		}

		middleware.RespondWithError(c, http.StatusInternalServerError, "LOGIN_FAILED", "Internal server error")
		return
	}

	resp := AuthResponse{
		TokenType:   tokens.TokenType,
		ExpiresIn:   tokens.ExpiresIn,
		Requires2FA: tokens.Requires2FA,
	}

	if tokens.Requires2FA {
		resp.TempToken = tokens.TempToken
	} else {
		user, userErr := h.lookupAuthenticatedCustomer(c, tokens.AccessToken)
		if userErr != nil {
			h.rollbackIssuedSession(c.Request.Context(), tokens.SessionID,
				"failed to roll back login session after customer lookup",
				middleware.GetCorrelationID(c))
			if sharederrors.Is(userErr, sharederrors.ErrNotFound) {
				middleware.RespondWithError(c, http.StatusNotFound, "NOT_FOUND", "customer not found")
				return
			}
			h.logger.Error("failed to load customer auth response user",
				"error", userErr,
				"correlation_id", middleware.GetCorrelationID(c))
			middleware.RespondWithError(c, http.StatusInternalServerError, "LOGIN_FAILED", "Internal server error")
			return
		}

		resp.SessionID = tokens.SessionID
		resp.SessionCleanupToken = tokens.SessionCleanupToken
		resp.User = buildAuthenticatedCustomerResponse(user)
		middleware.SetAuthCookies(c, tokens.AccessToken, refreshToken,
			middleware.AccessTokenMaxAge, middleware.RefreshTokenMaxAge, customerRefreshCookiePath)
	}

	h.logger.Info("customer login successful",
		"email", util.MaskEmail(req.Email),
		"requires_2fa", tokens.Requires2FA,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

// @Tags Customer
// @Summary Verify customer 2FA
// @Description Verifies customer TOTP code and returns session tokens.
// @Accept json
// @Produce json
// @Param request body object true "2FA verification request"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 429 {object} models.ErrorResponse
// @Router /api/v1/customer/auth/verify-2fa [post]
func (h *CustomerHandler) Verify2FA(c *gin.Context) {
	var req Verify2FARequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// F-075: Apply jti-based rate limiting so that a temp token is permanently
	// invalidated after 5 failed verification attempts, regardless of IP.
	jti := extractTempTokenJTI(req.TempToken, h.authConfig)
	if jti != "" {
		if !checkVerify2FARateLimit(jti) {
			middleware.RespondWithError(c, http.StatusUnauthorized, "INVALID_2FA_CODE", "Invalid or expired 2FA code")
			return
		}
	}

	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	tokens, refreshToken, err := h.authService.Verify2FA(c.Request.Context(), req.TempToken, req.TOTPCode, ipAddress, userAgent)
	if err != nil {
		h.logger.Warn("customer 2FA verification failed",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		if sharederrors.Is(err, sharederrors.ErrUnauthorized) {
			middleware.RespondWithError(c, http.StatusUnauthorized, "INVALID_2FA_CODE", "Invalid or expired 2FA code")
			return
		}
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "NOT_FOUND", "customer not found")
			return
		}

		h.logger.Error("2FA verification internal error", "error", err, "correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "2FA_VERIFICATION_FAILED", "Internal server error")
		return
	}

	// F-075: Clear the jti rate-limit entry on successful verification.
	if jti != "" {
		recordVerify2FASuccess(jti)
	}

	user, userErr := h.lookupAuthenticatedCustomer(c, tokens.AccessToken)
	if userErr != nil {
		h.rollbackIssuedSession(c.Request.Context(), tokens.SessionID,
			"failed to roll back 2FA session after customer lookup",
			middleware.GetCorrelationID(c))
		if sharederrors.Is(userErr, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "NOT_FOUND", "customer not found")
			return
		}
		h.logger.Error("failed to load customer 2FA auth response user",
			"error", userErr,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "2FA_VERIFICATION_FAILED", "Internal server error")
		return
	}

	resp := AuthResponse{
		TokenType:           tokens.TokenType,
		ExpiresIn:           tokens.ExpiresIn,
		SessionID:           tokens.SessionID,
		SessionCleanupToken: tokens.SessionCleanupToken,
		User:                buildAuthenticatedCustomerResponse(user),
	}

	middleware.SetAuthCookies(c, tokens.AccessToken, refreshToken,
		middleware.AccessTokenMaxAge, middleware.RefreshTokenMaxAge, customerRefreshCookiePath)

	h.logger.Info("customer 2FA verification successful",
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

// extractTempTokenJTI extracts the jti claim from a temp token without
// full validation. Returns an empty string if parsing fails.
func extractTempTokenJTI(tempToken string, authConfig middleware.AuthConfig) string {
	claims, err := middleware.ValidateJWT(authConfig, tempToken)
	if err != nil {
		return ""
	}
	return claims.ID
}

// @Tags Customer
// @Summary Refresh customer token
// @Description Refreshes customer access token using the HttpOnly refresh_token cookie.
// @Produce json
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Router /api/v1/customer/auth/refresh [post]
func (h *CustomerHandler) RefreshToken(c *gin.Context) {
	// F-005: Refresh tokens are read exclusively from the HttpOnly cookie.
	// The JSON body fallback has been removed to prevent token leakage via
	// JavaScript-accessible request bodies.
	refreshToken := middleware.GetRefreshTokenFromCookie(c)

	if refreshToken == "" {
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "refresh token is required")
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	tokens, newRefreshToken, err := h.authService.RefreshToken(c.Request.Context(), refreshToken, ipAddress, userAgent)
	if err != nil {
		h.logger.Warn("token refresh failed",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		if sharederrors.Is(err, sharederrors.ErrUnauthorized) {
			middleware.ClearAuthCookies(c, customerRefreshCookiePath)
			middleware.RespondWithError(c, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "Invalid or expired refresh token")
			return
		}

		middleware.RespondWithError(c, http.StatusInternalServerError, "REFRESH_FAILED", "Internal server error")
		return
	}

	middleware.SetAuthCookies(c, tokens.AccessToken, newRefreshToken,
		middleware.AccessTokenMaxAge, middleware.RefreshTokenMaxAge, customerRefreshCookiePath)

	resp := AuthResponse{
		TokenType:           tokens.TokenType,
		ExpiresIn:           tokens.ExpiresIn,
		SessionID:           tokens.SessionID,
		SessionCleanupToken: tokens.SessionCleanupToken,
	}

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

// @Tags Customer
// @Summary Customer logout
// @Description Invalidates customer session and refresh token.
// @Produce json
// @Security BearerAuth
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Router /api/v1/customer/auth/logout [post]
func (h *CustomerHandler) Logout(c *gin.Context) {
	var req LogoutRequest
	if err := middleware.BindOptionalJSON(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_REQUEST_BODY", "request body could not be parsed as JSON")
		return
	}

	targetSessionID, currentSessionID, authErr := resolveCustomerLogoutSession(c, h.authConfig, req.SessionCleanupToken)
	if authErr != nil {
		middleware.RespondWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return
	}

	if logoutErr := h.authService.Logout(c.Request.Context(), targetSessionID); logoutErr != nil {
		h.logger.Warn("failed to invalidate session on logout",
			"session_id", targetSessionID,
			"error", logoutErr,
			"correlation_id", middleware.GetCorrelationID(c))
		h.logAudit(c, "session.logout", "session", targetSessionID, nil, false)
		middleware.RespondWithError(c, http.StatusInternalServerError, "LOGOUT_FAILED", "Failed to log out")
		return
	}

	h.logAudit(c, "session.logout", "session", targetSessionID, nil, true)

	if shouldClearLogoutCookies(req.SessionCleanupToken, targetSessionID, currentSessionID) {
		middleware.ClearAuthCookies(c, customerRefreshCookiePath)
	}
	c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "Logged out successfully"}})
}

// @Tags Customer
// @Summary Change password
// @Description Changes password for the authenticated customer.
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object true "Password change request"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Router /api/v1/customer/password [put]
func (h *CustomerHandler) ChangePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// F-160: Password length is enforced solely by struct tag validation (min=12,max=128).
	// Duplicate manual len() checks have been removed.
	userID := middleware.GetUserID(c)
	if userID == "" {
		middleware.RespondWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return
	}

	err := h.authService.ChangePassword(
		c.Request.Context(),
		userID,
		req.CurrentPassword,
		req.NewPassword,
		"customer",
	)
	if err != nil {
		h.logger.Warn("password change failed",
			"user_id", userID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		h.logFailedAudit(c, "password.change", "user", userID, nil, err)

		if sharederrors.Is(err, sharederrors.ErrUnauthorized) {
			middleware.RespondWithError(c, http.StatusUnauthorized, "INVALID_CURRENT_PASSWORD", "Current password is incorrect")
			return
		}
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "NOT_FOUND", "customer not found")
			return
		}

		middleware.RespondWithError(c, http.StatusInternalServerError, "PASSWORD_CHANGE_FAILED", "Internal server error")
		return
	}

	h.logAudit(c, "password.change", "user", userID, nil, true)

	h.logger.Info("customer password changed",
		"user_id", userID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "Password updated successfully"}})
}

func hashToken(token string) string {
	return crypto.HashSHA256(token)
}

func (h *CustomerHandler) rollbackIssuedSession(ctx context.Context, sessionID, message, correlationID string) {
	if sessionID == "" {
		return
	}
	if err := h.authService.Logout(ctx, sessionID); err != nil {
		h.logger.Warn(message,
			"session_id", sessionID,
			"error", err,
			"correlation_id", correlationID)
	}
}

func (h *CustomerHandler) lookupAuthenticatedCustomer(c *gin.Context, accessToken string) (*models.Customer, error) {
	claims, err := middleware.ValidateJWT(h.authConfig, accessToken)
	if err != nil {
		return nil, err
	}

	customer, err := h.customerRepo.GetByID(c.Request.Context(), claims.UserID)
	if err != nil {
		return nil, err
	}

	return customer, nil
}

func buildAuthenticatedCustomerResponse(customer *models.Customer) *AuthenticatedCustomerResponse {
	return &AuthenticatedCustomerResponse{
		ID:    customer.ID,
		Email: customer.Email,
		Role:  "customer",
	}
}

func resolveCustomerLogoutSession(c *gin.Context, authConfig middleware.AuthConfig, sessionCleanupToken string) (targetSessionID, currentSessionID string, err error) {
	currentSessionID = middleware.GetSessionID(c)
	if sessionCleanupToken != "" {
		claims, cleanupErr := middleware.ValidateSessionCleanupToken(authConfig, sessionCleanupToken)
		if cleanupErr != nil {
			return "", currentSessionID, cleanupErr
		}
		if claims.UserType != "customer" {
			return "", currentSessionID, errors.New("invalid cleanup token user type")
		}
		return claims.SessionID, currentSessionID, nil
	}

	if currentSessionID == "" {
		return "", "", errors.New("missing session id")
	}

	return currentSessionID, currentSessionID, nil
}

func shouldClearLogoutCookies(sessionCleanupToken, targetSessionID, currentSessionID string) bool {
	if sessionCleanupToken == "" {
		return true
	}

	if currentSessionID == "" {
		return targetSessionID != ""
	}

	return targetSessionID == currentSessionID
}
