package customer

import (
	"errors"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/notifications"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// ForgotPasswordRequest represents the request body for requesting a password reset.
type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email,max=254"`
}

// ResetPasswordRequest represents the request body for resetting a password.
type ResetPasswordRequest struct {
	Token       string `json:"token" validate:"required,min=1,max=256"`
	NewPassword string `json:"new_password" validate:"required,min=12,max=128"`
}

// ForgotPassword handles POST /auth/forgot-password - initiates a password reset.
// Returns 200 OK for valid requests regardless of whether the email exists (anti-enumeration).
// Internal errors are logged but return the same generic success response.
func (h *CustomerHandler) ForgotPassword(c *gin.Context) {
	var req ForgotPasswordRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Always return success to prevent email enumeration.
	// The actual token generation and email sending happen in the background.
	resetToken, err := h.authService.RequestPasswordReset(c.Request.Context(), req.Email)
	if err != nil {
		h.logger.Error("failed to request password reset",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		// Still return 200 to prevent enumeration
		c.JSON(http.StatusOK, models.Response{
			Data: gin.H{"message": "If an account with that email exists, a password reset link has been sent."},
		})
		return
	}

	// If a reset token was generated (email exists), send the email
	if resetToken != "" {
		h.sendPasswordResetEmail(c, req.Email, resetToken)
	}

	c.JSON(http.StatusOK, models.Response{
		Data: gin.H{"message": "If an account with that email exists, a password reset link has been sent."},
	})
}

// sendPasswordResetEmail sends a password reset email to the customer.
func (h *CustomerHandler) sendPasswordResetEmail(c *gin.Context, email, token string) {
	if h.emailProvider == nil || !h.emailProvider.IsEnabled() {
		h.logger.Warn("email provider not configured, cannot send password reset email",
			"correlation_id", middleware.GetCorrelationID(c))
		return
	}

	resetURL := h.buildPasswordResetURL(token)

	payload := &notifications.EmailPayload{
		To:       email,
		Subject:  "Reset Your Password",
		Template: "password-reset",
		Data: map[string]any{
			"reset_url": resetURL,
		},
	}

	if err := h.emailProvider.Send(c.Request.Context(), payload); err != nil {
		h.logger.Error("failed to send password reset email",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
	}
}

// buildPasswordResetURL constructs the frontend password reset URL with the token.
func (h *CustomerHandler) buildPasswordResetURL(token string) string {
	baseURL := h.passwordResetBaseURL
	if baseURL == "" {
		baseURL = "/reset-password"
	}
	return baseURL + "?token=" + token
}

// ResetPassword handles POST /auth/reset-password - resets the password using a token.
func (h *CustomerHandler) ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	err := h.authService.ResetPassword(c.Request.Context(), req.Token, req.NewPassword)
	if err != nil {
		h.logger.Warn("password reset failed",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))

		if sharederrors.Is(err, sharederrors.ErrUnauthorized) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_RESET_TOKEN", "Invalid or expired reset token")
			return
		}

		if sharederrors.Is(err, sharederrors.ErrValidation) {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_RESET_TOKEN", "Invalid or expired reset token")
			return
		}

		middleware.RespondWithError(c, http.StatusInternalServerError, "RESET_FAILED", "Internal server error")
		return
	}

	h.logger.Info("password reset completed via customer API",
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{
		Data: gin.H{"message": "Password has been reset successfully. Please login with your new password."},
	})
}
