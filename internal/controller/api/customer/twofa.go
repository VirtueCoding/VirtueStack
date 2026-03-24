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

// Initiate2FAResponse contains the TOTP secret and QR code URL for 2FA setup.
// Backup codes are intentionally omitted here; they are returned only after
// 2FA is successfully enabled via Enable2FA (F-066).
type Initiate2FAResponse struct {
	Secret string `json:"secret"`
	QRURL  string `json:"qr_url"`
}

// Enable2FARequest contains the TOTP verification code to enable 2FA.
type Enable2FARequest struct {
	Code string `json:"code" validate:"required,len=6,numeric"`
}

// Enable2FAResponse confirms whether 2FA was successfully enabled and includes
// the one-time backup codes (F-066: codes are returned here, not during initiation).
type Enable2FAResponse struct {
	Enabled     bool     `json:"enabled"`
	BackupCodes []string `json:"backup_codes"`
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
	attempts  int
	firstTry  time.Time
	exhausted bool // F-075: marks a jti as permanently exhausted after max failures
}

var (
	twoFARateLimitMu     sync.Mutex
	twoFARateLimitMap    = make(map[string]*rateLimitEntry)
	twoFARateLimitMax    = 5
	twoFARateLimitWindow = 15 * time.Minute
)

func init() {
	// F-010: Start a background goroutine to evict stale entries from the
	// in-memory 2FA rate limit map so it does not grow without bound.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			evict2FARateLimitEntries()
		}
	}()
}

// evict2FARateLimitEntries removes entries whose rate-limit window has expired.
func evict2FARateLimitEntries() {
	twoFARateLimitMu.Lock()
	defer twoFARateLimitMu.Unlock()
	now := time.Now()
	for key, entry := range twoFARateLimitMap {
		if now.Sub(entry.firstTry) > twoFARateLimitWindow {
			delete(twoFARateLimitMap, key)
		}
	}
}

// checkVerify2FARateLimit implements jti-based rate limiting for the verify-2fa endpoint.
// F-075: After twoFARateLimitMax failed attempts for a given jti, the entry is marked
// exhausted and subsequent calls return false (the token is permanently invalidated).
// Returns true when the attempt is allowed, false when rate-limited or exhausted.
func checkVerify2FARateLimit(jti string) bool {
	twoFARateLimitMu.Lock()
	defer twoFARateLimitMu.Unlock()

	now := time.Now()
	entry, exists := twoFARateLimitMap["jti:"+jti]

	if !exists {
		twoFARateLimitMap["jti:"+jti] = &rateLimitEntry{attempts: 1, firstTry: now}
		return true
	}

	// Permanently exhausted jti — always deny.
	if entry.exhausted {
		return false
	}

	// Window expired — reset (the JWT itself may have already expired via its exp claim,
	// but we reset the counter defensively).
	if now.Sub(entry.firstTry) > twoFARateLimitWindow {
		twoFARateLimitMap["jti:"+jti] = &rateLimitEntry{attempts: 1, firstTry: now}
		return true
	}

	if entry.attempts >= twoFARateLimitMax {
		// Mark as exhausted so no further attempts are allowed.
		entry.exhausted = true
		return false
	}

	entry.attempts++
	return true
}

// recordVerify2FASuccess clears the rate-limit entry for a successfully verified jti.
func recordVerify2FASuccess(jti string) {
	twoFARateLimitMu.Lock()
	defer twoFARateLimitMu.Unlock()
	delete(twoFARateLimitMap, "jti:"+jti)
}

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

		if errors.Is(err, sharederrors.ErrTwoFAAlreadyEnabled) {
			middleware.RespondWithError(c, http.StatusBadRequest, "ALREADY_ENABLED", "2FA is already enabled for this account")
			return
		}

		middleware.RespondWithError(c, http.StatusInternalServerError, "INITIATION_FAILED", "Internal server error")
		return
	}

	h.logger.Info("2FA setup initiated", "user_id", userID, "correlation_id", middleware.GetCorrelationID(c))

	// F-066: Do not return backup codes during initiation.
	// Codes are returned only after 2FA is successfully enabled via Enable2FA.
	c.JSON(http.StatusOK, models.Response{Data: Initiate2FAResponse{
		Secret: result.Secret,
		QRURL:  result.QRURL,
	}})
}

// Enable2FA handles POST /2fa/enable - verifies the TOTP code and enables 2FA.
// Subject to rate limiting to prevent brute force attacks.
func (h *CustomerHandler) Enable2FA(c *gin.Context) {
	var req Enable2FARequest
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

	// F-150: Rate limit keyed on userID alone (not userID+IP).
	if !check2FARateLimit(userID) {
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

		if errors.Is(err, sharederrors.ErrTwoFAAlreadyEnabled) {
			middleware.RespondWithError(c, http.StatusBadRequest, "ALREADY_ENABLED", "2FA is already enabled for this account")
			return
		}

		if errors.Is(err, sharederrors.ErrTwoFASetupNotInitiated) {
			middleware.RespondWithError(c, http.StatusBadRequest, "NOT_INITIATED", "Please initiate 2FA setup first")
			return
		}

		middleware.RespondWithError(c, http.StatusInternalServerError, "ENABLE_FAILED", "Internal server error")
		return
	}

	// F-150: Clear rate limit by userID alone on success.
	cleanup2FARateLimit(userID)

	// F-066: Return backup codes here, after 2FA is successfully enabled.
	// Codes were generated and stored during Initiate2FA but not exposed then.
	backupCodes, _, err := h.authService.GetBackupCodes(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error("failed to retrieve backup codes after enabling 2FA", "user_id", userID, "error", err)
		// 2FA is enabled; failure to retrieve backup codes is non-fatal.
		backupCodes = nil
	}

	h.logger.Info("2FA enabled", "user_id", userID, "correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: Enable2FAResponse{
		Enabled:     true,
		BackupCodes: backupCodes,
	}})
}

// Disable2FA handles POST /2fa/disable - disables 2FA after password verification.
func (h *CustomerHandler) Disable2FA(c *gin.Context) {
	var req Disable2FARequest
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

	err := h.authService.Disable2FA(c.Request.Context(), userID, req.Password)
	if err != nil {
		h.logger.Warn("2FA disable failed", "user_id", userID, "error", err)

		if sharederrors.Is(err, sharederrors.ErrUnauthorized) {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_PASSWORD", "Invalid password")
			return
		}

		if errors.Is(err, sharederrors.ErrTwoFANotEnabled) {
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

	// F-150: Rate limit for regeneration keyed on userID alone.
	if !check2FARateLimit("regen:" + userID) {
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
