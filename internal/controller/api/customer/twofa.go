package customer

import (
	"net/http"
	"sync"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

type Initiate2FARequest struct{}

type Initiate2FAResponse struct {
	Secret      string   `json:"secret"`
	QRURL       string   `json:"qr_url"`
	BackupCodes []string `json:"backup_codes"`
}

type Enable2FARequest struct {
	Code string `json:"code" validate:"required,len=6,numeric"`
}

type Enable2FAResponse struct {
	Enabled bool `json:"enabled"`
}

type Disable2FARequest struct {
	Password string `json:"password" validate:"required,min=1"`
}

type Disable2FAResponse struct {
	Enabled bool `json:"enabled"`
}

type Get2FAStatusResponse struct {
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

type GetBackupCodesResponse struct {
	Codes []string `json:"codes"`
}

type RegenerateBackupCodesResponse struct {
	Codes []string `json:"codes"`
}

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

func cleanup2FARateLimit(identifier string) {
	twoFARateLimitMu.Lock()
	defer twoFARateLimitMu.Unlock()
	delete(twoFARateLimitMap, identifier)
}

func (h *CustomerHandler) Initiate2FA(c *gin.Context) {
	userID := middleware.GetUserID(c)

	customer, err := h.customerRepo.GetByID(c.Request.Context(), userID)
	if err != nil {
		h.logger.Warn("failed to get customer for 2FA initiation", "user_id", userID, "error", err)
		respondWithError(c, http.StatusInternalServerError, "GET_CUSTOMER_FAILED", err.Error())
		return
	}

	result, err := h.authService.Initiate2FA(c.Request.Context(), userID, customer.Email)
	if err != nil {
		h.logger.Warn("2FA initiation failed", "user_id", userID, "error", err)

		if err.Error() == "2FA is already enabled" {
			respondWithError(c, http.StatusBadRequest, "ALREADY_ENABLED", "2FA is already enabled for this account")
			return
		}

		respondWithError(c, http.StatusInternalServerError, "INITIATION_FAILED", err.Error())
		return
	}

	h.logger.Info("2FA setup initiated", "user_id", userID, "correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: Initiate2FAResponse{
		Secret:      result.Secret,
		QRURL:       result.QRURL,
		BackupCodes: result.BackupCodes,
	}})
}

func (h *CustomerHandler) Enable2FA(c *gin.Context) {
	var req Enable2FARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)
	ipAddress := c.ClientIP()

	if !check2FARateLimit(userID + ":" + ipAddress) {
		respondWithError(c, http.StatusTooManyRequests, "RATE_LIMITED", "Too many verification attempts. Please try again later.")
		return
	}

	err := h.authService.Enable2FA(c.Request.Context(), userID, req.Code)
	if err != nil {
		h.logger.Warn("2FA enable failed", "user_id", userID, "error", err)

		if sharederrors.Is(err, sharederrors.ErrUnauthorized) {
			respondWithError(c, http.StatusBadRequest, "INVALID_CODE", "Invalid TOTP code")
			return
		}

		if err.Error() == "2FA is already enabled" {
			respondWithError(c, http.StatusBadRequest, "ALREADY_ENABLED", "2FA is already enabled for this account")
			return
		}

		if err.Error() == "2FA setup not initiated" {
			respondWithError(c, http.StatusBadRequest, "NOT_INITIATED", "Please initiate 2FA setup first")
			return
		}

		respondWithError(c, http.StatusInternalServerError, "ENABLE_FAILED", err.Error())
		return
	}

	cleanup2FARateLimit(userID + ":" + ipAddress)

	h.logger.Info("2FA enabled", "user_id", userID, "correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: Enable2FAResponse{Enabled: true}})
}

func (h *CustomerHandler) Disable2FA(c *gin.Context) {
	var req Disable2FARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body: "+err.Error())
		return
	}

	userID := middleware.GetUserID(c)

	err := h.authService.Disable2FA(c.Request.Context(), userID, req.Password)
	if err != nil {
		h.logger.Warn("2FA disable failed", "user_id", userID, "error", err)

		if sharederrors.Is(err, sharederrors.ErrUnauthorized) {
			respondWithError(c, http.StatusBadRequest, "INVALID_PASSWORD", "Invalid password")
			return
		}

		if err.Error() == "2FA is not enabled" {
			respondWithError(c, http.StatusBadRequest, "NOT_ENABLED", "2FA is not enabled for this account")
			return
		}

		respondWithError(c, http.StatusInternalServerError, "DISABLE_FAILED", err.Error())
		return
	}

	h.logger.Info("2FA disabled", "user_id", userID, "correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: Disable2FAResponse{Enabled: false}})
}

func (h *CustomerHandler) Get2FAStatus(c *gin.Context) {
	userID := middleware.GetUserID(c)

	enabled, _, err := h.authService.Get2FAStatus(c.Request.Context(), userID)
	if err != nil {
		h.logger.Warn("2FA status check failed", "user_id", userID, "error", err)
		respondWithError(c, http.StatusInternalServerError, "STATUS_FAILED", err.Error())
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: Get2FAStatusResponse{
		Enabled: enabled,
	}})
}

func (h *CustomerHandler) GetBackupCodes(c *gin.Context) {
	userID := middleware.GetUserID(c)

	// Check if 2FA is enabled first
	enabled, _, err := h.authService.Get2FAStatus(c.Request.Context(), userID)
	if err != nil {
		h.logger.Warn("failed to get 2FA status for backup codes", "user_id", userID, "error", err)
		respondWithError(c, http.StatusInternalServerError, "STATUS_FAILED", err.Error())
		return
	}

	if !enabled {
		respondWithError(c, http.StatusBadRequest, "2FA_NOT_ENABLED", "2FA is not enabled for this account")
		return
	}

	// Get backup codes
	codes, alreadyShown, err := h.authService.GetBackupCodes(c.Request.Context(), userID)
	if err != nil {
		h.logger.Warn("failed to get backup codes", "user_id", userID, "error", err)
		respondWithError(c, http.StatusInternalServerError, "GET_BACKUP_CODES_FAILED", err.Error())
		return
	}

	// If codes have already been shown once, return error
	if alreadyShown {
		respondWithError(c, http.StatusBadRequest, "BACKUP_CODES_ALREADY_SHOWN", "Backup codes have already been displayed. Please regenerate to see new codes.")
		return
	}

	h.logger.Info("backup codes retrieved", "user_id", userID, "correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: GetBackupCodesResponse{
		Codes: codes,
	}})
}

func (h *CustomerHandler) RegenerateBackupCodes(c *gin.Context) {
	userID := middleware.GetUserID(c)
	ipAddress := c.ClientIP()

	// Check rate limit for regeneration
	if !check2FARateLimit("regen:" + userID + ":" + ipAddress) {
		respondWithError(c, http.StatusTooManyRequests, "RATE_LIMITED", "Too many regeneration attempts. Please try again later.")
		return
	}

	// Check if 2FA is enabled first
	enabled, _, err := h.authService.Get2FAStatus(c.Request.Context(), userID)
	if err != nil {
		h.logger.Warn("failed to get 2FA status for backup codes regeneration", "user_id", userID, "error", err)
		respondWithError(c, http.StatusInternalServerError, "STATUS_FAILED", err.Error())
		return
	}

	if !enabled {
		respondWithError(c, http.StatusBadRequest, "2FA_NOT_ENABLED", "2FA is not enabled for this account")
		return
	}

	// Regenerate backup codes
	codes, err := h.authService.RegenerateBackupCodes(c.Request.Context(), userID)
	if err != nil {
		h.logger.Warn("failed to regenerate backup codes", "user_id", userID, "error", err)
		respondWithError(c, http.StatusInternalServerError, "REGENERATE_BACKUP_CODES_FAILED", err.Error())
		return
	}

	h.logger.Info("backup codes regenerated", "user_id", userID, "correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: RegenerateBackupCodesResponse{
		Codes: codes,
	}})
}
