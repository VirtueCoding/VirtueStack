package customer

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// LoginRequest represents the request body for customer login.
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email,max=254"`
	Password string `json:"password" validate:"required,min=1,max=128"`
}

// Verify2FARequest represents the request body for 2FA verification.
type Verify2FARequest struct {
	TempToken string `json:"temp_token" validate:"required"`
	TOTPCode  string `json:"totp_code" validate:"required,len=6,numeric"`
}

// RefreshTokenRequest represents the request body for token refresh.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// ChangePasswordRequest represents the request body for password change.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required,min=12,max=128"`
	NewPassword     string `json:"new_password" validate:"required,min=12,max=128"`
}

// AuthResponse represents the response for successful authentication.
type AuthResponse struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	Requires2FA  bool   `json:"requires_2fa,omitempty"`
	TempToken    string `json:"temp_token,omitempty"`
}

// Login handles POST /auth/login - authenticates a customer.
// If the customer has 2FA enabled, returns a temp token for verification.
// Otherwise, returns access and refresh tokens directly.
func (h *CustomerHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body: "+err.Error())
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	tokens, refreshToken, err := h.authService.Login(c.Request.Context(), req.Email, req.Password, ipAddress, userAgent)
	if err != nil {
		h.logger.Warn("customer login failed",
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
	} else {
		resp.AccessToken = tokens.AccessToken
		resp.RefreshToken = refreshToken
	}

	h.logger.Info("customer login successful",
		"email", req.Email,
		"requires_2fa", tokens.Requires2FA,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

// Verify2FA handles POST /auth/verify-2fa - verifies TOTP code for 2FA login.
// Exchanges a temp token (from Login) for access and refresh tokens.
func (h *CustomerHandler) Verify2FA(c *gin.Context) {
	var req Verify2FARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body: "+err.Error())
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	tokens, refreshToken, err := h.authService.Verify2FA(c.Request.Context(), req.TempToken, req.TOTPCode, ipAddress, userAgent)
	if err != nil {
		h.logger.Warn("customer 2FA verification failed",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		if sharederrors.Is(err, sharederrors.ErrUnauthorized) {
			respondWithError(c, http.StatusUnauthorized, "INVALID_2FA_CODE", "Invalid or expired 2FA code")
			return
		}

		respondWithError(c, http.StatusInternalServerError, "2FA_VERIFICATION_FAILED", err.Error())
		return
	}

	resp := AuthResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: refreshToken,
		TokenType:    tokens.TokenType,
		ExpiresIn:    tokens.ExpiresIn,
	}

	h.logger.Info("customer 2FA verification successful",
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

// RefreshToken handles POST /auth/refresh - refreshes an access token.
// Validates the refresh token and issues new tokens (rotation).
func (h *CustomerHandler) RefreshToken(c *gin.Context) {
	var req RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body: "+err.Error())
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")

	tokens, newRefreshToken, err := h.authService.RefreshToken(c.Request.Context(), req.RefreshToken, ipAddress, userAgent)
	if err != nil {
		h.logger.Warn("token refresh failed",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		if sharederrors.Is(err, sharederrors.ErrUnauthorized) {
			respondWithError(c, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "Invalid or expired refresh token")
			return
		}

		respondWithError(c, http.StatusInternalServerError, "REFRESH_FAILED", err.Error())
		return
	}

	resp := AuthResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: newRefreshToken,
		TokenType:    tokens.TokenType,
		ExpiresIn:    tokens.ExpiresIn,
	}

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

// Logout handles POST /auth/logout - invalidates the current session.
// Requires authentication. The session is derived from the refresh token in the request body.
func (h *CustomerHandler) Logout(c *gin.Context) {
	var req RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body: "+err.Error())
		return
	}

	// Get session by refresh token
	refreshTokenHash := hashToken(req.RefreshToken)
	session, err := h.customerRepo.GetSessionByRefreshToken(c.Request.Context(), refreshTokenHash)
	if err != nil {
		// Session not found - still return success (idempotent)
		c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "Logged out successfully"}})
		return
	}

	// Verify the session belongs to the authenticated user
	userID := middleware.GetUserID(c)
	if session.UserID != userID {
		respondWithError(c, http.StatusForbidden, "FORBIDDEN", "Cannot logout another user's session")
		return
	}

	// Delete the session
	if err := h.authService.Logout(c.Request.Context(), session.ID); err != nil {
		h.logger.Warn("failed to logout session",
			"session_id", session.ID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		// Still return success - the session may have already been deleted
	}

	h.logger.Info("customer logged out",
		"user_id", userID,
		"session_id", session.ID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "Logged out successfully"}})
}

// respondWithError sends a standardized error response.
func respondWithError(c *gin.Context, status int, code, message string) {
	correlationID := middleware.GetCorrelationID(c)

	c.JSON(status, gin.H{
		"error": gin.H{
			"code":           code,
			"message":        message,
			"correlation_id": correlationID,
		},
	})
}

// ChangePassword handles PUT /password - changes the authenticated customer's password.
// Requires valid JWT authentication. Rate limited to 5 attempts per 15 minutes per IP.
func (h *CustomerHandler) ChangePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body: "+err.Error())
		return
	}

	if len(req.CurrentPassword) < 12 {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "current_password must be at least 12 characters")
		return
	}

	if len(req.NewPassword) < 12 {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "new_password must be at least 12 characters")
		return
	}

	userID := middleware.GetUserID(c)
	if userID == "" {
		respondWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
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
			respondWithError(c, http.StatusUnauthorized, "INVALID_CURRENT_PASSWORD", "Current password is incorrect")
			return
		}

		respondWithError(c, http.StatusInternalServerError, "PASSWORD_CHANGE_FAILED", err.Error())
		return
	}

	if h.auditRepo != nil {
		audit := &models.AuditLog{
			ActorID:      &userID,
			ActorType:    models.AuditActorCustomer,
			Action:       "password.change",
			ResourceType: "user",
			ResourceID:   &userID,
			Success:      true,
		}
		if err := h.auditRepo.Append(c.Request.Context(), audit); err != nil {
			h.logger.Warn("failed to write audit log for password change",
				"user_id", userID,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
		}
	}

	h.logger.Info("customer password changed",
		"user_id", userID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "Password updated successfully"}})
}

// hashToken computes a SHA-256 hash of a token for secure comparison.
// This is used for refresh token lookups.
func hashToken(token string) string {
	return crypto.HashSHA256(token)
}
