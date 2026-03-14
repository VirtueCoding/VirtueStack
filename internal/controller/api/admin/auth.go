package admin

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
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
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	tokens, err := h.authService.AdminLogin(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		h.logger.Warn("admin login failed",
			"email", req.Email,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		if sharederrors.Is(err, sharederrors.ErrUnauthorized) {
			respondWithError(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
			return
		}

		respondWithError(c, http.StatusInternalServerError, "LOGIN_FAILED", err.Error())
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
		"email", req.Email,
		"requires_2fa", tokens.Requires2FA,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

func (h *AdminHandler) Verify2FA(c *gin.Context) {
	var req Verify2FARequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			respondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
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
			respondWithError(c, http.StatusUnauthorized, "INVALID_2FA_CODE", "Invalid or expired 2FA code")
			return
		}

		respondWithError(c, http.StatusInternalServerError, "2FA_VERIFICATION_FAILED", err.Error())
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
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "refresh token is required")
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
			respondWithError(c, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "Invalid or expired refresh token")
			return
		}

		respondWithError(c, http.StatusInternalServerError, "REFRESH_FAILED", err.Error())
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
