package customer

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// Initiate2FARequest represents an empty request body for initiating 2FA setup.
type Initiate2FARequest struct{}

// Initiate2FAResponse contains the TOTP secret, QR code URL, and backup codes for 2FA setup.
type Initiate2FAResponse struct {
	Secret      string   `json:"secret"`
	QRURL       string   `json:"qr_url"`
	BackupCodes []string `json:"backup_codes"`
}

// Enable2FARequest contains the TOTP verification code to enable 2FA.
type Enable2FARequest struct {
	Code string `json:"code" validate:"required,len=6,numeric"`
}

// Enable2FAResponse confirms whether 2FA was successfully enabled.
type Enable2FAResponse struct {
	Enabled bool `json:"enabled"`
}

// Disable2FARequest contains the password required to disable 2FA.
type Disable2FARequest struct {
	Password string `json:"password" validate:"required,min=12"`
}

// Disable2FAResponse confirms whether 2FA was successfully disabled.
type Disable2FAResponse struct {
	Enabled bool `json:"enabled"`
}

// Get2FAStatusResponse contains the current 2FA status for the authenticated user.
type Get2FAStatusResponse struct {
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// GetBackupCodesResponse contains the backup codes for 2FA recovery.
type GetBackupCodesResponse struct {
	Codes []string `json:"codes"`
}

// RegenerateBackupCodesResponse contains newly generated backup codes.
type RegenerateBackupCodesResponse struct {
	Codes []string `json:"codes"`
}

// rateLimitEntry tracks 2FA verification attempts for rate limiting.
type rateLimitEntry struct {
	attempts int
	firstTry time.Time
}

var (
	twoFARateLimitMu     sync.Mutex
	twoFARateLimitMap    = make(map[string]*rateLimitEntry)
	twoFARateLimitMax    = 5
	twoFARateLimitWindow = 15 * time.Minute
)

// check2FARateLimit returns false if the identifier has exceeded the rate limit.
func check2FARateLimit(identifier string) bool {
	twoFARateLimitMu.Lock()
	defer twoFARateLimitMu.Unlock()

	now := time.Now()
	entry, exists := twoFARateLimitMap[identifier]

	if !exists || now.Sub(entry.firstTry) > twoFARateLimitWindow {
		twoFARateLimitMap[identifier] = &rateLimitEntry{attempts: 1, firstTry: now}
		return true
	}

	if entry.attempts >= twoFARateLimitMax {
		return false
	}

	entry.attempts++
	return true
}

// cleanup2FARateLimit removes the rate limit entry for the given identifier.
func cleanup2FARateLimit(identifier string) {
	twoFARateLimitMu.Lock()
	defer twoFARateLimitMu.Unlock()
	delete(twoFARateLimitMap, identifier)
}

// Initiate2FA handles POST /2fa/initiate - generates a new TOTP secret and QR code.
// Returns the secret, QR URL, and backup codes for the user to set up their authenticator app.
func (h *CustomerHandler) Initiate2FA(c *gin.Context) {
	userID := middleware.GetUserID(c)

	customer, err := h.customerRepo.GetByID(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("failed to get customer for 2FA initiation", "user_id", userID, "error", err)
		middleware.RespondWithError(c, http.StatusInternalServerError, "GET_CUSTOMER_FAILED", "Internal server error")
		return
	}

	result, err := h.authService.Initiate2FA(c.Request.Context(), userID, customer.Email)
	if err != nil {
		h.logger.Warn("2FA initiation failed", "user_id", userID, "error", err)

		if errors.Is(err, sharederrors.Err2FAAlreadyEnabled) {
			middleware.RespondWithError(c, http.StatusBadRequest, "ALREADY_ENABLED", "2FA is already enabled for this account")
			return
		}

		middleware.RespondWithError(c, http.StatusInternalServerError, "INITIATION_FAILED", "Internal server error")
		return
	}

	h.logger.Info("2FA setup initiated", "user_id", userID, "correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: Initiate2FAResponse{
		Secret:      result.Secret,
		QRURL:       result.QRURL,
		BackupCodes: result.BackupCodes,
	}})
}

// Enable2FA handles POST /2fa/enable - verifies the TOTP code and enables 2FA.
// Subject to rate limiting to prevent brute force attacks.
func (h *CustomerHandler) Enable2FA(c *gin.Context) {
	var req Enable2FARequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	userID := middleware.GetUserID(c)
	ipAddress := c.ClientIP()

	if !check2FARateLimit(userID + ":" + ipAddress) {
		middleware.RespondWithError(c, http.StatusTooManyRequests, "RATE_LIMITED", "Too many verification attempts. Please try again later.")
		return
	}

	err := h.authService.Enable2FA(c.Request.Context(), userID, req.Code)
	if err != nil {
		h.logger.Warn("2FA enable failed", "user_id", userID, "error", err)

		if sharederrors.Is(err, sharederrors.ErrUnauthorized) {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_CODE", "Invalid TOTP code")
			return
		}

		if errors.Is(err, sharederrors.Err2FAAlreadyEnabled) {
			middleware.RespondWithError(c, http.StatusBadRequest, "ALREADY_ENABLED", "2FA is already enabled for this account")
			return
		}

		if errors.Is(err, sharederrors.Err2FASetupNotInitiated) {
			middleware.RespondWithError(c, http.StatusBadRequest, "NOT_INITIATED", "Please initiate 2FA setup first")
			return
		}

		middleware.RespondWithError(c, http.StatusInternalServerError, "ENABLE_FAILED", "Internal server error")
		return
	}

	cleanup2FARateLimit(userID + ":" + ipAddress)

	h.logger.Info("2FA enabled", "user_id", userID, "correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: Enable2FAResponse{Enabled: true}})
}

// Disable2FA handles POST /2fa/disable - disables 2FA after password verification.
func (h *CustomerHandler) Disable2FA(c *gin.Context) {
	var req Disable2FARequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	userID := middleware.GetUserID(c)

	err := h.authService.Disable2FA(c.Request.Context(), userID, req.Password)
	if err != nil {
		h.logger.Warn("2FA disable failed", "user_id", userID, "error", err)

		if sharederrors.Is(err, sharederrors.ErrUnauthorized) {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_PASSWORD", "Invalid password")
			return
		}

		if errors.Is(err, sharederrors.Err2FANotEnabled) {
			middleware.RespondWithError(c, http.StatusBadRequest, "NOT_ENABLED", "2FA is not enabled for this account")
			return
		}

		middleware.RespondWithError(c, http.StatusInternalServerError, "DISABLE_FAILED", "Internal server error")
		return
	}

	h.logger.Info("2FA disabled", "user_id", userID, "correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: Disable2FAResponse{Enabled: false}})
}

// Get2FAStatus handles GET /2fa/status - returns the current 2FA status for the user.
func (h *CustomerHandler) Get2FAStatus(c *gin.Context) {
	userID := middleware.GetUserID(c)

	enabled, _, err := h.authService.Get2FAStatus(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("2FA status check failed", "user_id", userID, "error", err)
		middleware.RespondWithError(c, http.StatusInternalServerError, "STATUS_FAILED", "Internal server error")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: Get2FAStatusResponse{
		Enabled: enabled,
	}})
}

// GetBackupCodes handles GET /2fa/backup-codes - returns backup codes (only available once).
func (h *CustomerHandler) GetBackupCodes(c *gin.Context) {
	userID := middleware.GetUserID(c)

	// Check if 2FA is enabled first
	enabled, _, err := h.authService.Get2FAStatus(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("failed to get 2FA status for backup codes", "user_id", userID, "error", err)
		middleware.RespondWithError(c, http.StatusInternalServerError, "STATUS_FAILED", "Internal server error")
		return
	}

	if !enabled {
		middleware.RespondWithError(c, http.StatusBadRequest, "2FA_NOT_ENABLED", "2FA is not enabled for this account")
		return
	}

	// Get backup codes
	codes, alreadyShown, err := h.authService.GetBackupCodes(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("failed to get backup codes", "user_id", userID, "error", err)
		middleware.RespondWithError(c, http.StatusInternalServerError, "GET_BACKUP_CODES_FAILED", "Internal server error")
		return
	}

	// If codes have already been shown once, return error
	if alreadyShown {
		middleware.RespondWithError(c, http.StatusBadRequest, "BACKUP_CODES_ALREADY_SHOWN", "Backup codes have already been displayed. Please regenerate to see new codes.")
		return
	}

	h.logger.Info("backup codes retrieved", "user_id", userID, "correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: GetBackupCodesResponse{
		Codes: codes,
	}})
}

// RegenerateBackupCodes handles POST /2fa/backup-codes/regenerate - generates new backup codes.
// Subject to rate limiting to prevent abuse.
func (h *CustomerHandler) RegenerateBackupCodes(c *gin.Context) {
	userID := middleware.GetUserID(c)
	ipAddress := c.ClientIP()

	// Check rate limit for regeneration
	if !check2FARateLimit("regen:" + userID + ":" + ipAddress) {
		middleware.RespondWithError(c, http.StatusTooManyRequests, "RATE_LIMITED", "Too many regeneration attempts. Please try again later.")
		return
	}

	// Check if 2FA is enabled first
	enabled, _, err := h.authService.Get2FAStatus(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("failed to get 2FA status for backup codes regeneration", "user_id", userID, "error", err)
		middleware.RespondWithError(c, http.StatusInternalServerError, "STATUS_FAILED", "Internal server error")
		return
	}

	if !enabled {
		middleware.RespondWithError(c, http.StatusBadRequest, "2FA_NOT_ENABLED", "2FA is not enabled for this account")
		return
	}

	// Regenerate backup codes
	codes, err := h.authService.RegenerateBackupCodes(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("failed to regenerate backup codes", "user_id", userID, "error", err)
		middleware.RespondWithError(c, http.StatusInternalServerError, "REGENERATE_BACKUP_CODES_FAILED", "Internal server error")
		return
	}

	h.logger.Info("backup codes regenerated", "user_id", userID, "correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: RegenerateBackupCodesResponse{
		Codes: codes,
	}})
}
