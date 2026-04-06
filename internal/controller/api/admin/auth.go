package admin

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
	"github.com/gin-gonic/gin"
)

// adminVerify2FARateLimit enforces F-075: after adminVerify2FAMaxAttempts failed
// attempts for a given jti (temp-token ID), the jti is permanently exhausted.
var (
	adminVerify2FARateLimitMu     sync.Mutex
	adminVerify2FARateLimitMap    = make(map[string]*adminRateLimitEntry)
	adminVerify2FAMaxAttempts     = 5
	adminVerify2FARateLimitWindow = 15 * time.Minute
)

type adminRateLimitEntry struct {
	attempts  int
	firstTry  time.Time
	exhausted bool
}

// checkAdminVerify2FARateLimit returns true when the attempt is allowed,
// false when the jti is rate-limited or permanently exhausted.
func checkAdminVerify2FARateLimit(jti string) bool {
	adminVerify2FARateLimitMu.Lock()
	defer adminVerify2FARateLimitMu.Unlock()
	now := time.Now()
	pruneExpiredAdminVerify2FAEntries(now)
	entry, exists := adminVerify2FARateLimitMap[jti]
	if !exists {
		adminVerify2FARateLimitMap[jti] = &adminRateLimitEntry{attempts: 1, firstTry: now}
		return true
	}
	if entry.exhausted {
		return false
	}
	if now.Sub(entry.firstTry) > adminVerify2FARateLimitWindow {
		adminVerify2FARateLimitMap[jti] = &adminRateLimitEntry{attempts: 1, firstTry: now}
		return true
	}
	if entry.attempts >= adminVerify2FAMaxAttempts {
		entry.exhausted = true
		return false
	}
	entry.attempts++
	return true
}

func pruneExpiredAdminVerify2FAEntries(now time.Time) {
	for jti, entry := range adminVerify2FARateLimitMap {
		if now.Sub(entry.firstTry) > adminVerify2FARateLimitWindow {
			delete(adminVerify2FARateLimitMap, jti)
		}
	}
}

// recordAdminVerify2FASuccess permanently exhausts a successfully verified jti
// so the stateless temp token becomes single-use until it naturally expires.
func recordAdminVerify2FASuccess(jti string) {
	adminVerify2FARateLimitMu.Lock()
	defer adminVerify2FARateLimitMu.Unlock()
	entry, exists := adminVerify2FARateLimitMap[jti]
	if !exists {
		entry = &adminRateLimitEntry{firstTry: time.Now()}
		adminVerify2FARateLimitMap[jti] = entry
	}
	entry.exhausted = true
	if entry.attempts == 0 {
		entry.attempts = adminVerify2FAMaxAttempts
	}
}

type LoginRequest struct {
	Email    string `json:"email" validate:"required,email,max=254"`
	Password string `json:"password" validate:"required,min=12,max=128"`
}

type Verify2FARequest struct {
	TempToken string `json:"temp_token" validate:"required"`
	TOTPCode  string `json:"totp_code" validate:"required,len=6,numeric"`
}

type AuthResponse struct {
	TokenType           string      `json:"token_type"`
	ExpiresIn           int         `json:"expires_in,omitempty"`
	Requires2FA         bool        `json:"requires_2fa,omitempty"`
	TempToken           string      `json:"temp_token,omitempty"`
	SessionID           string      `json:"session_id,omitempty"`
	SessionCleanupToken string      `json:"session_cleanup_token,omitempty"`
	User                *MeResponse `json:"user,omitempty"`
}

// LogoutRequest carries the cleanup token used to revoke the current admin session.
type LogoutRequest struct {
	SessionCleanupToken string `json:"session_cleanup_token"`
}

const adminRefreshCookiePath = "/api/v1/admin/auth/refresh"

// CSRF handles GET /admin/auth/csrf - returns the CSRF token in the response header.
// The CSRF middleware sets the cookie before this handler runs.
// @Tags Admin
// @Summary Get CSRF token
// @Description Issues CSRF cookie/token pair for admin frontend authentication flows.
// @Produce json
// @Success 200 {object} models.Response
// @Router /api/v1/admin/auth/csrf [get]
func (h *AdminHandler) CSRF(c *gin.Context) {
	c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "CSRF token set"}})
}

// @Tags Admin
// @Summary Admin login
// @Description Authenticates an admin with email and password and starts 2FA flow when required.
// @Accept json
// @Produce json
// @Param request body object true "Admin login request"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 429 {object} models.ErrorResponse
// @Router /api/v1/admin/auth/login [post]
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

// @Tags Admin
// @Summary Verify admin 2FA
// @Description Verifies TOTP code and returns authenticated admin session tokens.
// @Accept json
// @Produce json
// @Param request body object true "2FA verification request"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 429 {object} models.ErrorResponse
// @Router /api/v1/admin/auth/verify-2fa [post]
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

	// F-075: Apply jti-based rate limiting so the temp token is permanently
	// invalidated after adminVerify2FAMaxAttempts failed verification attempts,
	// regardless of source IP (prevents distributed brute-force).
	if claims, err := middleware.ValidateTempToken(h.authConfig, req.TempToken); err == nil && claims.ID != "" {
		if !checkAdminVerify2FARateLimit(claims.ID) {
			middleware.RespondWithError(c, http.StatusUnauthorized, "INVALID_2FA_CODE", "Invalid or expired 2FA code")
			return
		}
		defer func() {
			// On success (no error path taken above), clear the rate-limit entry.
			// recordAdminVerify2FASuccess is also called explicitly on the success path.
			_ = claims.ID // captured for use in success path below
		}()
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

	// Clear jti rate-limit entry on successful verification.
	if claims, err := middleware.ValidateTempToken(h.authConfig, req.TempToken); err == nil && claims.ID != "" {
		recordAdminVerify2FASuccess(claims.ID)
	}

	claims, claimsErr := middleware.ValidateTempToken(h.authConfig, req.TempToken)
	if claimsErr != nil {
		if tokens.SessionID != "" {
			if logoutErr := h.authService.Logout(c.Request.Context(), tokens.SessionID); logoutErr != nil {
				h.logger.Warn("failed to roll back admin session after temp token reload failure",
					"session_id", tokens.SessionID,
					"error", logoutErr,
					"correlation_id", middleware.GetCorrelationID(c))
			}
		}
		h.logger.Error("failed to reload temp token after admin 2FA verification",
			"error", claimsErr,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "2FA_VERIFICATION_FAILED", "Internal server error")
		return
	}

	admin, adminErr := h.authService.GetAdminByID(c.Request.Context(), claims.UserID)
	if adminErr != nil {
		if tokens.SessionID != "" {
			if logoutErr := h.authService.Logout(c.Request.Context(), tokens.SessionID); logoutErr != nil {
				h.logger.Warn("failed to roll back admin session after user lookup failure",
					"session_id", tokens.SessionID,
					"error", logoutErr,
					"correlation_id", middleware.GetCorrelationID(c))
			}
		}
		if handleNotFoundError(c, adminErr, "NOT_FOUND", "admin not found") {
			return
		}
		h.logger.Error("failed to load admin auth response user",
			"error", adminErr,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "2FA_VERIFICATION_FAILED", "Internal server error")
		return
	}

	resp := AuthResponse{
		TokenType:           tokens.TokenType,
		ExpiresIn:           tokens.ExpiresIn,
		SessionID:           tokens.SessionID,
		SessionCleanupToken: tokens.SessionCleanupToken,
		User:                buildMeResponse(admin),
	}

	middleware.SetAuthCookies(c, tokens.AccessToken, refreshToken,
		middleware.AccessTokenMaxAge, middleware.RefreshTokenMaxAgeAdmin, adminRefreshCookiePath)

	h.logger.Info("admin 2FA verification successful",
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

// @Tags Admin
// @Summary Refresh admin token
// @Description Refreshes admin access token using the HttpOnly refresh_token cookie.
// @Produce json
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Router /api/v1/admin/auth/refresh [post]
func (h *AdminHandler) RefreshToken(c *gin.Context) {
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
		TokenType:           tokens.TokenType,
		ExpiresIn:           tokens.ExpiresIn,
		SessionID:           tokens.SessionID,
		SessionCleanupToken: tokens.SessionCleanupToken,
	}

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

// @Tags Admin
// @Summary Admin logout
// @Description Invalidates the current admin session and refresh token.
// @Produce json
// @Security BearerAuth
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Router /api/v1/admin/auth/logout [post]
func (h *AdminHandler) Logout(c *gin.Context) {
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

	targetSessionID, currentSessionID, authErr := resolveAdminLogoutSession(c, h.authConfig, req.SessionCleanupToken)
	if authErr != nil {
		middleware.RespondWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	if logoutErr := h.authService.Logout(c.Request.Context(), targetSessionID); logoutErr != nil {
		h.logger.Warn("failed to invalidate admin session on logout",
			"session_id", targetSessionID,
			"error", logoutErr,
			"correlation_id", middleware.GetCorrelationID(c))
		h.logLogoutAuditEvent(c, targetSessionID, req.SessionCleanupToken, false, logoutErr.Error())
		middleware.RespondWithError(c, http.StatusInternalServerError, "LOGOUT_FAILED", "Failed to log out")
		return
	}

	h.logLogoutAuditEvent(c, targetSessionID, req.SessionCleanupToken, true, "")

	if shouldClearLogoutCookies(req.SessionCleanupToken, targetSessionID, currentSessionID) {
		middleware.ClearAuthCookies(c, adminRefreshCookiePath)
	}
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

// ReauthRequest contains the admin's current password for re-authentication.
type ReauthRequest struct {
	Password string `json:"password" validate:"required,min=12,max=128"`
}

// ReauthResponse contains the re-auth token.
type ReauthResponse struct {
	ReauthToken string `json:"reauth_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// Me returns the current authenticated admin user's identity.
// This is a lightweight endpoint used for session validation that returns
// only the essential user fields (id, email, role) without any heavy queries.
// The user is identified from the JWT claims set by the JWTAuth middleware.
// @Tags Admin
// @Summary Get current admin
// @Description Returns profile data for the authenticated admin user.
// @Produce json
// @Security BearerAuth
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Router /api/v1/admin/auth/me [get]
func (h *AdminHandler) Me(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == "" {
		middleware.RespondWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "user not authenticated")
		return
	}

	admin, err := h.authService.GetAdminByID(c.Request.Context(), userID)
	if err != nil {
		if handleNotFoundError(c, err, "NOT_FOUND", "admin not found") {
			return
		}
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

func buildMeResponse(admin *models.Admin) *MeResponse {
	permissions := admin.Permissions
	if len(permissions) == 0 {
		permissions = models.GetDefaultPermissions(admin.Role)
	}

	return &MeResponse{
		ID:          admin.ID,
		Email:       admin.Email,
		Role:        admin.Role,
		Permissions: models.PermissionsToStrings(permissions),
	}
}

func resolveAdminLogoutSession(c *gin.Context, authConfig middleware.AuthConfig, sessionCleanupToken string) (targetSessionID, currentSessionID string, err error) {
	currentSessionID = resolveCurrentAdminSessionID(c, authConfig)

	if sessionCleanupToken != "" {
		claims, err := middleware.ValidateSessionCleanupToken(authConfig, sessionCleanupToken)
		if err != nil {
			return "", currentSessionID, err
		}
		if claims.UserType != "admin" {
			return "", currentSessionID, errors.New("invalid cleanup token user type")
		}
		return claims.SessionID, currentSessionID, nil
	}

	if currentSessionID == "" {
		return "", "", errors.New("missing session id")
	}

	return currentSessionID, currentSessionID, nil
}

func (h *AdminHandler) logLogoutAuditEvent(c *gin.Context, targetSessionID, sessionCleanupToken string, success bool, errorMessage string) {
	if h.auditRepo == nil {
		return
	}

	actorID := resolveAdminLogoutActorID(c, h.authConfig, sessionCleanupToken)
	h.appendAuditLog(c, &adminAuditLogEntry{
		actorID:      actorID,
		action:       "session.logout",
		resourceType: "session",
		resourceID:   targetSessionID,
		success:      success,
		errorMessage: errorMessage,
	})
}

func resolveAdminLogoutActorID(c *gin.Context, authConfig middleware.AuthConfig, sessionCleanupToken string) string {
	if sessionCleanupToken != "" {
		claims, err := middleware.ValidateSessionCleanupToken(authConfig, sessionCleanupToken)
		if err == nil && claims.UserType == "admin" {
			return claims.UserID
		}
	}

	accessToken := middleware.GetAccessTokenFromCookie(c)
	if accessToken == "" {
		return ""
	}

	claims, err := middleware.ValidateJWT(authConfig, accessToken)
	if err != nil || claims.Purpose != "" || claims.UserType != "admin" {
		return ""
	}

	return claims.UserID
}

func resolveCurrentAdminSessionID(c *gin.Context, authConfig middleware.AuthConfig) string {
	accessToken := middleware.GetAccessTokenFromCookie(c)
	if accessToken == "" {
		return ""
	}

	claims, err := middleware.ValidateJWT(authConfig, accessToken)
	if err != nil {
		return ""
	}
	if claims.Purpose != "" || claims.UserType != "admin" {
		return ""
	}

	return claims.SessionID
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

// Reauth handles POST /api/v1/admin/auth/reauth.
// It verifies the admin's password and issues a short-lived re-auth token
// that can be used to authorize destructive operations within 15 minutes.
// @Tags Admin
// @Summary Re-authenticate admin
// @Description Verifies credentials and returns a short-lived X-Reauth-Token for destructive operations.
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object true "Re-authentication request"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Router /api/v1/admin/auth/reauth [post]
func (h *AdminHandler) Reauth(c *gin.Context) {
	var req ReauthRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	userID := middleware.GetUserID(c)
	if userID == "" {
		middleware.RespondWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "user not authenticated")
		return
	}

	// Verify the admin's password
	admin, err := h.authService.GetAdminByID(c.Request.Context(), userID)
	if err != nil {
		if handleNotFoundError(c, err, "NOT_FOUND", "admin not found") {
			return
		}
		h.logger.Warn("failed to get admin for reauth",
			"admin_id", userID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to verify identity")
		return
	}

	// Verify password matches
	match, err := h.authService.VerifyPassword(req.Password, admin.PasswordHash)
	if err != nil || !match {
		h.logger.Warn("reauth failed: invalid password",
			"admin_id", userID,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusForbidden, "INVALID_PASSWORD", "password is incorrect")
		return
	}

	// Generate re-auth token with purpose="reauth" and 15-minute expiry
	reauthToken, err := middleware.GenerateReauthToken(h.authConfig, admin.ID, "admin")
	if err != nil {
		h.logger.Error("failed to generate reauth token",
			"admin_id", userID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate re-auth token")
		return
	}

	h.logger.Info("admin reauth successful",
		"admin_id", userID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: ReauthResponse{
		ReauthToken: reauthToken,
		ExpiresIn:   int(middleware.ReauthTokenDuration.Seconds()),
	}})
}
