package customer

import (
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
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in,omitempty"`
	Requires2FA bool   `json:"requires_2fa,omitempty"`
	TempToken   string `json:"temp_token,omitempty"`
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

		h.logger.Error("2FA verification internal error", "error", err, "correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "2FA_VERIFICATION_FAILED", "Internal server error")
		return
	}

	// F-075: Clear the jti rate-limit entry on successful verification.
	if jti != "" {
		recordVerify2FASuccess(jti)
	}

	middleware.SetAuthCookies(c, tokens.AccessToken, refreshToken,
		middleware.AccessTokenMaxAge, middleware.RefreshTokenMaxAge, customerRefreshCookiePath)

	resp := AuthResponse{
		TokenType: tokens.TokenType,
		ExpiresIn: tokens.ExpiresIn,
	}

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
// @Description Refreshes customer access token using refresh token.
// @Accept json
// @Produce json
// @Param request body object true "Refresh token request"
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
		TokenType: tokens.TokenType,
		ExpiresIn: tokens.ExpiresIn,
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
	refreshToken := middleware.GetRefreshTokenFromCookie(c)

	if refreshToken != "" {
		refreshTokenHash := hashToken(refreshToken)
		session, err := h.customerRepo.GetSessionByRefreshToken(c.Request.Context(), refreshTokenHash)
		if err == nil {
			userID := middleware.GetUserID(c)
			if session.UserID == userID {
				// Error intentionally ignored: logout clears cookies regardless of session
				// invalidation failure so the client is always logged out locally.
				if logoutErr := h.authService.Logout(c.Request.Context(), session.ID); logoutErr != nil {
					h.logger.Warn("failed to invalidate session on logout",
						"session_id", session.ID,
						"error", logoutErr,
						"correlation_id", middleware.GetCorrelationID(c))
				}
				// F-152: Audit log entry for successful session logout.
				h.logAudit(c, "session.logout", "session", session.ID, nil, true)
				h.logger.Info("customer logged out",
					"user_id", userID,
					"session_id", session.ID,
					"correlation_id", middleware.GetCorrelationID(c))
			}
		}
	}

	middleware.ClearAuthCookies(c, customerRefreshCookiePath)
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

		if sharederrors.Is(err, sharederrors.ErrUnauthorized) {
			middleware.RespondWithError(c, http.StatusUnauthorized, "INVALID_CURRENT_PASSWORD", "Current password is incorrect")
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
