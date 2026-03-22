package provisioning

import (
	"errors"
	"net/http"
	"unicode"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/alexedwards/argon2id"
	"github.com/gin-gonic/gin"
)

// SetPassword handles POST /vms/:id/password - sets the root password for a VM.
// This endpoint is called by WHMCS to set or change the root password.
func (h *ProvisioningHandler) SetPassword(c *gin.Context) {
	ctx := c.Request.Context()
	vmID := c.Param("id")

	vm, err := getValidVM(ctx, h.vmRepo, vmID, h.logger)
	if err != nil {
		respondWithValidationError(c, err)
		return
	}
	var req PasswordRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	if err := validatePasswordStrength(req.Password); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	hashedPassword, err := hashPassword(req.Password)
	if err != nil {
		h.logger.Error("failed to hash password", "vm_id", vmID, "error", err, "correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "PASSWORD_HASHING_FAILED", "Failed to hash password")
		return
	}

	if err := h.vmRepo.UpdatePassword(ctx, vmID, hashedPassword); err != nil {
		h.logger.Error("failed to update password in database", "vm_id", vmID, "error", err, "correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "PASSWORD_UPDATE_FAILED", "Failed to update password")
		return
	}

	h.logger.Info("VM password set via provisioning API", "vm_id", vmID, "customer_id", vm.CustomerID, "correlation_id", middleware.GetCorrelationID(c))
	c.JSON(http.StatusOK, models.Response{Data: gin.H{"vm_id": vmID, "message": "Password updated successfully"}})
}

// ResetPassword handles POST /vms/:id/password/reset - generates and sets a new random password.
func (h *ProvisioningHandler) ResetPassword(c *gin.Context) {
	ctx := c.Request.Context()
	vmID := c.Param("id")

	vm, err := getValidVM(ctx, h.vmRepo, vmID, h.logger)
	if err != nil {
		respondWithValidationError(c, err)
		return
	}

	newPassword := generateRandomPassword()

	hashedPassword, err := hashPassword(newPassword)
	if err != nil {
		h.logger.Error("failed to hash password during reset", "vm_id", vmID, "error", err, "correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "PASSWORD_HASHING_FAILED", "Failed to hash password")
		return
	}

	if err := h.vmRepo.UpdatePassword(ctx, vmID, hashedPassword); err != nil {
		h.logger.Error("failed to update password in database", "vm_id", vmID, "error", err, "correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "PASSWORD_UPDATE_FAILED", "Failed to update password")
		return
	}

	h.logger.Info("VM password reset via provisioning API", "vm_id", vmID, "customer_id", vm.CustomerID, "correlation_id", middleware.GetCorrelationID(c))
	c.JSON(http.StatusOK, models.Response{Data: gin.H{"vm_id": vmID, "password": newPassword, "message": "Password reset successfully"}})
}

var hashPasswordParams = &argon2id.Params{
	Memory:      65536,
	Iterations:  3,
	Parallelism: 4,
	SaltLength:  16,
	KeyLength:   32,
}

func hashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("password cannot be empty")
	}

	if err := validatePasswordStrength(password); err != nil {
		return "", err
	}

	hash, err := argon2id.CreateHash(password, hashPasswordParams)
	if err != nil {
		return "", err
	}
	return hash, nil
}

// validatePasswordStrength validates that a password meets security requirements.
// It returns a ValidationError with specific field and issue information.
func validatePasswordStrength(password string) error {
	if len(password) < 12 {
		return sharederrors.NewValidationError("password", "must be at least 12 characters long")
	}

	if len(password) > 128 {
		return sharederrors.NewValidationError("password", "must not exceed 128 characters")
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return sharederrors.NewValidationError("password", "must contain at least one uppercase letter")
	}
	if !hasLower {
		return sharederrors.NewValidationError("password", "must contain at least one lowercase letter")
	}
	if !hasDigit {
		return sharederrors.NewValidationError("password", "must contain at least one digit")
	}
	if !hasSpecial {
		return sharederrors.NewValidationError("password", "must contain at least one special character")
	}

	return nil
}
