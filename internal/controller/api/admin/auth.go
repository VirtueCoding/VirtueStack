package admin

import (
	"errors"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
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

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type AuthResponse struct {
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in,omitempty"`
	Requires2FA bool   `json:"requires_2fa,omitempty"`
	TempToken   string `json:"temp_token,omitempty"`
}

const adminRefreshCookiePath = "/api/v1/admin/auth/refresh"

func (h *AdminHandler) Login(c *gin.Context) {
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

	tokens, err := h.authService.AdminLogin(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		h.logger.Warn("admin login failed",
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
	}

	h.logger.Info("admin login successful",
		"email", util.MaskEmail(req.Email),
		"requires_2fa", tokens.Requires2FA,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

func (h *AdminHandler) Verify2FA(c *gin.Context) {
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

	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	tokens, refreshToken, err := h.authService.AdminVerify2FA(c.Request.Context(), req.TempToken, req.TOTPCode, ipAddress, userAgent)
	if err != nil {
		h.logger.Warn("admin 2FA verification failed",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		if sharederrors.Is(err, sharederrors.ErrUnauthorized) {
			middleware.RespondWithError(c, http.StatusUnauthorized, "INVALID_2FA_CODE", "Invalid or expired 2FA code")
			return
		}

		middleware.RespondWithError(c, http.StatusInternalServerError, "2FA_VERIFICATION_FAILED", "Internal server error")
		return
	}

	middleware.SetAuthCookies(c, tokens.AccessToken, refreshToken,
		middleware.AccessTokenMaxAge, middleware.RefreshTokenMaxAgeAdmin, adminRefreshCookiePath)

	resp := AuthResponse{
		TokenType: tokens.TokenType,
		ExpiresIn: tokens.ExpiresIn,
	}

	h.logger.Info("admin 2FA verification successful",
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

func (h *AdminHandler) RefreshToken(c *gin.Context) {
	refreshToken := middleware.GetRefreshTokenFromCookie(c)

	var req RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err == nil && req.RefreshToken != "" {
		refreshToken = req.RefreshToken
	}

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
			middleware.ClearAuthCookies(c, adminRefreshCookiePath)
			middleware.RespondWithError(c, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "Invalid or expired refresh token")
			return
		}

		middleware.RespondWithError(c, http.StatusInternalServerError, "REFRESH_FAILED", "Internal server error")
		return
	}

	middleware.SetAuthCookies(c, tokens.AccessToken, newRefreshToken,
		middleware.AccessTokenMaxAge, middleware.RefreshTokenMaxAgeAdmin, adminRefreshCookiePath)

	resp := AuthResponse{
		TokenType: tokens.TokenType,
		ExpiresIn: tokens.ExpiresIn,
	}

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

func (h *AdminHandler) Logout(c *gin.Context) {
	middleware.ClearAuthCookies(c, adminRefreshCookiePath)
	c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "Logged out successfully"}})
}

// MeResponse contains the current admin user's identity.
// This is a lightweight response suitable for session validation.
type MeResponse struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions,omitempty"`
}

// Me returns the current authenticated admin user's identity.
// This is a lightweight endpoint used for session validation that returns
// only the essential user fields (id, email, role) without any heavy queries.
// The user is identified from the JWT claims set by the JWTAuth middleware.
func (h *AdminHandler) Me(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		middleware.RespondWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "user not authenticated")
		return
	}

	admin, err := h.authService.GetAdminByID(c.Request.Context(), userID)
	if err != nil {
		h.logger.Warn("failed to get admin for /me endpoint",
			"user_id", userID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to retrieve user identity")
		return
	}

	// Get effective permissions (custom permissions or role-based defaults)
	permissions := admin.Permissions
	if len(permissions) == 0 {
		// Use role-based default permissions if no custom permissions set
		permissions = models.GetDefaultPermissions(admin.Role)
	}

	resp := MeResponse{
		ID:          admin.ID,
		Email:       admin.Email,
		Role:        admin.Role,
		Permissions: models.PermissionsToStrings(permissions),
	}

	c.JSON(http.StatusOK, models.Response{Data: resp})
}
