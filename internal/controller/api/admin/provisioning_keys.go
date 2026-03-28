package admin

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ListProvisioningKeys handles GET /provisioning-keys.
// @Tags Admin
// @Summary List provisioning keys
// @Description Manages provisioning API keys for external integrations.
// @Produce json
// @Security BearerAuth
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/provisioning-keys [get]
func (h *AdminHandler) ListProvisioningKeys(c *gin.Context) {
	includeRevoked := c.Query("include_revoked") == "true"

	keys, err := h.provisioningKeyRepo.List(c.Request.Context(), includeRevoked)
	if err != nil {
		h.logger.Error("failed to list provisioning keys",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"PROVISIONING_KEY_LIST_FAILED", "Failed to list provisioning keys")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: keys})
}

// GetProvisioningKey handles GET /provisioning-keys/:id.
// @Tags Admin
// @Summary Get provisioning key
// @Description Manages provisioning API keys for external integrations.
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provisioning key ID"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/provisioning-keys/{id} [get]
func (h *AdminHandler) GetProvisioningKey(c *gin.Context) {
	id, ok := validateUUIDParam(c, "id", "INVALID_KEY_ID", "Provisioning key ID must be a valid UUID")
	if !ok {
		return
	}

	key, err := h.provisioningKeyRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		if handleNotFoundError(c, err, "KEY_NOT_FOUND", "Provisioning key not found") {
			return
		}
		h.logger.Error("failed to get provisioning key",
			"key_id", id, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"PROVISIONING_KEY_GET_FAILED", "Failed to retrieve provisioning key")
		return
	}

	c.JSON(http.StatusOK, models.Response{Data: key})
}

// CreateProvisioningKey handles POST /provisioning-keys.
// @Tags Admin
// @Summary Create provisioning key
// @Description Manages provisioning API keys for external integrations.
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body object true "Request body"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/provisioning-keys [post]
func (h *AdminHandler) CreateProvisioningKey(c *gin.Context) {
	var req models.ProvisioningKeyCreateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest,
			"VALIDATION_ERROR", "Invalid request")
		return
	}

	// Override CreatedBy with the authenticated admin's ID.
	req.CreatedBy = middleware.GetUserID(c)

	rawKey, err := generateProvisioningAPIKey()
	if err != nil {
		h.logger.Error("failed to generate provisioning API key",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"KEY_GENERATION_FAILED", "Failed to generate API key")
		return
	}

	keyHash := hashProvisioningKey(rawKey)

	key := &models.ProvisioningKey{
		ID:          uuid.New().String(),
		Name:        req.Name,
		KeyHash:     keyHash,
		AllowedIPs:  req.AllowedIPs,
		CreatedBy:   req.CreatedBy,
		Description: req.Description,
		ExpiresAt:   req.ExpiresAt,
	}

	if err := h.provisioningKeyRepo.Create(c.Request.Context(), key); err != nil {
		h.logger.Error("failed to create provisioning key",
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"PROVISIONING_KEY_CREATE_FAILED", "Failed to create provisioning key")
		return
	}

	h.logAuditEvent(c, "provisioning_key.create", "provisioning_key", key.ID,
		map[string]any{"name": key.Name, "allowed_ips": key.AllowedIPs, "expires_at": key.ExpiresAt}, true)

	c.JSON(http.StatusCreated, models.Response{Data: models.ProvisioningKeyResponse{
		ID:        key.ID,
		Name:      key.Name,
		Key:       rawKey,
		CreatedAt: key.CreatedAt,
	}})
}

// UpdateProvisioningKey handles PUT /provisioning-keys/:id.
// @Tags Admin
// @Summary Update provisioning key
// @Description Manages provisioning API keys for external integrations.
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provisioning key ID"
// @Param request body object true "Request body"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/provisioning-keys/{id} [put]
func (h *AdminHandler) UpdateProvisioningKey(c *gin.Context) {
	id, ok := validateUUIDParam(c, "id", "INVALID_KEY_ID", "Provisioning key ID must be a valid UUID")
	if !ok {
		return
	}

	var req models.ProvisioningKeyUpdateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest,
			"VALIDATION_ERROR", "Invalid request")
		return
	}

	updated, err := h.provisioningKeyRepo.Update(c.Request.Context(), id, &req)
	if err != nil {
		if handleNotFoundError(c, err, "KEY_NOT_FOUND", "Provisioning key not found or already revoked") {
			return
		}
		h.logger.Error("failed to update provisioning key",
			"key_id", id, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"PROVISIONING_KEY_UPDATE_FAILED", "Failed to update provisioning key")
		return
	}

	h.logAuditEvent(c, "provisioning_key.update", "provisioning_key", id,
		map[string]any{"name": req.Name, "allowed_ips": req.AllowedIPs, "expires_at": req.ExpiresAt}, true)

	c.JSON(http.StatusOK, models.Response{Data: updated})
}

// RevokeProvisioningKey handles DELETE /provisioning-keys/:id.
// @Tags Admin
// @Summary Revoke provisioning key
// @Description Manages provisioning API keys for external integrations.
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provisioning key ID"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/provisioning-keys/{id} [delete]
func (h *AdminHandler) RevokeProvisioningKey(c *gin.Context) {
	id, ok := validateUUIDParam(c, "id", "INVALID_KEY_ID", "Provisioning key ID must be a valid UUID")
	if !ok {
		return
	}

	if err := h.provisioningKeyRepo.Revoke(c.Request.Context(), id); err != nil {
		if sharederrors.Is(err, sharederrors.ErrNoRowsAffected) {
			middleware.RespondWithError(c, http.StatusNotFound, "KEY_NOT_FOUND",
				"Provisioning key not found or already revoked")
			return
		}
		h.logger.Error("failed to revoke provisioning key",
			"key_id", id, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"PROVISIONING_KEY_REVOKE_FAILED", "Failed to revoke provisioning key")
		return
	}

	h.logAuditEvent(c, "provisioning_key.revoke", "provisioning_key", id, nil, true)

	c.Status(http.StatusNoContent)
}

// generateProvisioningAPIKey creates a 32-byte (64-hex-char) random API key.
func generateProvisioningAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating provisioning API key: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// hashProvisioningKey returns the SHA-256 hex digest of a raw provisioning API key.
func hashProvisioningKey(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}
