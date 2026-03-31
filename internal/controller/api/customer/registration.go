package customer

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/notifications"
	"github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// RegisterRequest represents customer self-registration payload.
type RegisterRequest struct {
	Email    string `json:"email" validate:"required,email,max=254"`
	Password string `json:"password" validate:"required,min=12,max=128"`
	Name     string `json:"name" validate:"required,max=255"`
	Phone    string `json:"phone,omitempty" validate:"omitempty,max=32"`
}

// VerifyEmailRequest represents email verification payload.
type VerifyEmailRequest struct {
	Token string `json:"token" validate:"required,min=32,max=256"`
}

type emailVerificationRepo interface {
	CreateEmailVerificationToken(ctx context.Context, token *models.EmailVerificationToken) error
	GetEmailVerificationTokenByHash(ctx context.Context, tokenHash string) (*models.EmailVerificationToken, error)
	DeleteEmailVerificationTokenByID(ctx context.Context, id string) error
}

// Register handles POST /auth/register.
func (h *CustomerHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	name := strings.TrimSpace(req.Name)
	if existing, err := h.customerRepo.GetByEmail(c.Request.Context(), email); err == nil && existing != nil {
		middleware.RespondWithError(c, http.StatusConflict, "EMAIL_ALREADY_EXISTS", "An account with this email already exists")
		return
	} else if err != nil && !sharederrors.Is(err, sharederrors.ErrNotFound) {
		h.logger.Error("failed to check existing customer for registration",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "REGISTRATION_FAILED", "Internal server error")
		return
	}

	passwordHash, err := h.authService.HashPassword(req.Password)
	if err != nil {
		h.logger.Error("failed to hash registration password",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "REGISTRATION_FAILED", "Internal server error")
		return
	}

	status := models.CustomerStatusActive
	if h.registrationEmailVerification {
		status = models.CustomerStatusPendingVerification
	}
	customer := &models.Customer{
		Email:        email,
		PasswordHash: passwordHash,
		Name:         name,
		Status:       status,
	}
	if phone := strings.TrimSpace(req.Phone); phone != "" {
		customer.Phone = &phone
	}

	created, err := h.customerService.Create(c.Request.Context(), "self-registration", c.ClientIP(), customer)
	if err != nil {
		h.logger.Error("failed to create customer from registration",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "REGISTRATION_FAILED", "Failed to create account")
		return
	}

	if h.registrationEmailVerification {
		token, tokenHash, err := generateVerificationToken()
		if err != nil {
			h.logger.Error("failed to generate email verification token",
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			middleware.RespondWithError(c, http.StatusInternalServerError, "REGISTRATION_FAILED", "Failed to create account")
			return
		}

		verificationToken := &models.EmailVerificationToken{
			CustomerID: created.ID,
			TokenHash:  tokenHash,
			ExpiresAt:  time.Now().Add(24 * time.Hour),
		}
		verificationRepo, ok := any(h.customerRepo).(emailVerificationRepo)
		if !ok {
			h.logger.Error("customer repository does not support email verification storage",
				"correlation_id", middleware.GetCorrelationID(c))
			middleware.RespondWithError(c, http.StatusInternalServerError, "REGISTRATION_FAILED", "Failed to create account")
			return
		}
		if err := verificationRepo.CreateEmailVerificationToken(c.Request.Context(), verificationToken); err != nil {
			h.logger.Error("failed to persist email verification token",
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			middleware.RespondWithError(c, http.StatusInternalServerError, "REGISTRATION_FAILED", "Failed to create account")
			return
		}
		h.sendVerificationEmail(c, created.Email, created.Name, token)
	}

	c.JSON(http.StatusCreated, models.Response{
		Data: gin.H{
			"id":                  created.ID,
			"email":               created.Email,
			"name":                created.Name,
			"requires_verification": h.registrationEmailVerification,
		},
	})
}

// VerifyEmail handles POST /auth/verify-email.
func (h *CustomerHandler) VerifyEmail(c *gin.Context) {
	var req VerifyEmailRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	tokenHash := hashToken(req.Token)
	verificationRepo, ok := any(h.customerRepo).(emailVerificationRepo)
	if !ok {
		middleware.RespondWithError(c, http.StatusInternalServerError, "VERIFY_EMAIL_FAILED", "Internal server error")
		return
	}
	record, err := verificationRepo.GetEmailVerificationTokenByHash(c.Request.Context(), tokenHash)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VERIFICATION_TOKEN", "Invalid or expired verification token")
			return
		}
		h.logger.Error("failed to fetch email verification token",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "VERIFY_EMAIL_FAILED", "Internal server error")
		return
	}
	if time.Now().After(record.ExpiresAt) {
		_ = verificationRepo.DeleteEmailVerificationTokenByID(c.Request.Context(), record.ID)
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VERIFICATION_TOKEN", "Invalid or expired verification token")
		return
	}

	if err := h.customerRepo.UpdateStatus(c.Request.Context(), record.CustomerID, models.CustomerStatusActive); err != nil {
		h.logger.Error("failed to activate customer during email verification",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "VERIFY_EMAIL_FAILED", "Internal server error")
		return
	}
	if err := verificationRepo.DeleteEmailVerificationTokenByID(c.Request.Context(), record.ID); err != nil {
		h.logger.Warn("failed to delete used email verification token",
			"token_id", record.ID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
	}

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "Email verified successfully"}})
}

func generateVerificationToken() (string, string, error) {
	token, err := crypto.GenerateRandomToken(32)
	if err != nil {
		return "", "", err
	}
	return token, hashToken(token), nil
}

func (h *CustomerHandler) sendVerificationEmail(c *gin.Context, email, customerName, token string) {
	if h.emailProvider == nil || !h.emailProvider.IsEnabled() {
		h.logger.Warn("email provider not configured, cannot send verification email",
			"correlation_id", middleware.GetCorrelationID(c))
		return
	}

	verifyURL := h.buildEmailVerificationURL(token)
	payload := &notifications.EmailPayload{
		To:           email,
		Subject:      "Verify Your Email Address",
		Template:     "default",
		CustomerName: customerName,
		Data: map[string]any{
			"verify_url": verifyURL,
		},
	}
	if err := h.emailProvider.Send(c.Request.Context(), payload); err != nil {
		h.logger.Error("failed to send verification email",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
	}
}

func (h *CustomerHandler) buildEmailVerificationURL(token string) string {
	baseURL := strings.TrimSuffix(h.consoleBaseURL, "/")
	if baseURL == "" {
		return "/verify-email?token=" + token
	}
	return baseURL + "/verify-email?token=" + token
}
