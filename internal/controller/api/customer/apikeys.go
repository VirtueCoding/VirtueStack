package customer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/audit"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/google/uuid"

	"github.com/gin-gonic/gin"
)

// CreateAPIKeyRequest represents the request body for creating an API key.
type CreateAPIKeyRequest struct {
	Name        string   `json:"name" validate:"required,max=100"`
	Permissions []string `json:"permissions" validate:"required,min=1,dive,max=100"`
	AllowedIPs  []string `json:"allowed_ips,omitempty" validate:"max=50,dive,ip|cidr"`
	ExpiresAt   *string  `json:"expires_at,omitempty"`
}

// RotateAPIKeyRequest represents the request body for rotating an API key.
type RotateAPIKeyRequest struct {
	Name string `json:"name,omitempty"`
}

// APIKeyResponse represents an API key in responses (includes the key on creation).
type APIKeyResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
	AllowedIPs  []string `json:"allowed_ips,omitempty"`
	Key         string   `json:"key,omitempty"`
	IsActive    bool     `json:"is_active"`
	ExpiresAt   *string  `json:"expires_at,omitempty"`
	CreatedAt   string   `json:"created_at"`
	LastUsedAt  *string  `json:"last_used_at,omitempty"`
}

// validPermissions defines the allowed permission scopes for customer API keys.
var validPermissions = map[string]bool{
	"vm:read":        true,
	"vm:write":       true,
	"vm:power":       true,
	"backup:read":    true,
	"backup:write":   true,
	"snapshot:read":  true,
	"snapshot:write": true,
}

// ListAPIKeys handles GET /api-keys - lists all API keys for the customer.
func (h *CustomerHandler) ListAPIKeys(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	correlationID := middleware.GetCorrelationID(c)

	h.logger.Debug("listing API keys",
		"customer_id", customerID,
		"correlation_id", correlationID)

	keys, err := h.apiKeyRepo.ListByCustomer(c.Request.Context(), customerID, false)
	if err != nil {
		h.logger.Error("failed to list API keys",
			"customer_id", customerID,
			"error", err,
			"correlation_id", correlationID)
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list API keys")
		return
	}

	resp := make([]APIKeyResponse, len(keys))
	for i, key := range keys {
		var expiresAtStr *string
		if key.ExpiresAt != nil {
			s := key.ExpiresAt.Format(time.RFC3339)
			expiresAtStr = &s
		}
		var lastUsedAtStr *string
		if key.LastUsedAt != nil {
			s := key.LastUsedAt.Format(time.RFC3339)
			lastUsedAtStr = &s
		}
		resp[i] = APIKeyResponse{
			ID:          key.ID,
			Name:        key.Name,
			Permissions: key.Permissions,
			AllowedIPs:  key.AllowedIPs,
			IsActive:    key.IsActive,
			ExpiresAt:   expiresAtStr,
			CreatedAt:   key.CreatedAt.Format(time.RFC3339),
			LastUsedAt:  lastUsedAtStr,
		}
	}

	// Meta is omitted: ListByCustomer returns all keys without pagination support.
	c.JSON(http.StatusOK, models.ListResponse{
		Data: resp,
	})
}

// CreateAPIKey handles POST /api-keys - creates a new API key for the customer.
func (h *CustomerHandler) CreateAPIKey(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	correlationID := middleware.GetCorrelationID(c)

	var req CreateAPIKeyRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	for _, perm := range req.Permissions {
		if !validPermissions[perm] {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_PERMISSION", "Invalid permission: "+perm)
			return
		}
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		parsed, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_EXPIRATION", "Invalid expiration timestamp format")
			return
		}
		if parsed.Before(time.Now()) {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_EXPIRATION", "Expiration must be in the future")
			return
		}
		expiresAt = &parsed
	}

	keyID := uuid.New().String()
	rawKey := "vs_" + uuid.New().String()
	keyHash := hashAPIKey(rawKey)

	key := &models.CustomerAPIKey{
		ID:          keyID,
		CustomerID:  customerID,
		Name:        req.Name,
		KeyHash:     keyHash,
		Permissions: req.Permissions,
		AllowedIPs:  req.AllowedIPs,
		ExpiresAt:   expiresAt,
	}

	if err := h.apiKeyRepo.Create(c.Request.Context(), key); err != nil {
		h.logger.Error("failed to create API key",
			"customer_id", customerID,
			"error", err,
			"correlation_id", correlationID)
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create API key")
		return
	}

	h.logAudit(c, "api_key.create", "api_key", keyID, map[string]any{
		"name":        req.Name,
		"permissions": req.Permissions,
	}, true)

	h.logger.Info("API key created",
		"key_id", keyID,
		"customer_id", customerID,
		"name", req.Name,
		"correlation_id", correlationID)

	var expiresAtStr *string
	if key.ExpiresAt != nil {
		s := key.ExpiresAt.Format(time.RFC3339)
		expiresAtStr = &s
	}

	resp := APIKeyResponse{
		ID:          keyID,
		Name:        req.Name,
		Permissions: req.Permissions,
		AllowedIPs:  req.AllowedIPs,
		Key:         rawKey,
		IsActive:    true,
		ExpiresAt:   expiresAtStr,
		CreatedAt:   key.CreatedAt.Format(time.RFC3339),
	}

	c.JSON(http.StatusCreated, models.Response{Data: resp})
}

// RotateAPIKey handles POST /api-keys/:id/rotate - rotates an existing API key.
func (h *CustomerHandler) RotateAPIKey(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	keyID := c.Param("id")
	correlationID := middleware.GetCorrelationID(c)

	if _, err := uuid.Parse(keyID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_KEY_ID", "API Key ID must be a valid UUID")
		return
	}

	existingKey, err := h.apiKeyRepo.GetByIDAndCustomer(c.Request.Context(), keyID, customerID)
	if err != nil {
		if errors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "NOT_FOUND", "API key not found")
			return
		}
		h.logger.Error("failed to get API key for rotation",
			"key_id", keyID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", correlationID)
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to rotate API key")
		return
	}

	if !existingKey.IsActive {
		middleware.RespondWithError(c, http.StatusBadRequest, "KEY_REVOKED", "Cannot rotate a revoked API key")
		return
	}

	if existingKey.ExpiresAt != nil && existingKey.ExpiresAt.Before(time.Now()) {
		middleware.RespondWithError(c, http.StatusBadRequest, "KEY_EXPIRED", "Cannot rotate an expired API key")
		return
	}

	newRawKey := "vs_" + uuid.New().String()
	newKeyHash := hashAPIKey(newRawKey)

	if err := h.apiKeyRepo.Rotate(c.Request.Context(), keyID, customerID, newKeyHash); err != nil {
		h.logger.Error("failed to rotate API key",
			"key_id", keyID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", correlationID)
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to rotate API key")
		return
	}

	h.logAudit(c, "api_key.rotate", "api_key", keyID, map[string]any{
		"name": existingKey.Name,
	}, true)

	h.logger.Info("API key rotated",
		"key_id", keyID,
		"customer_id", customerID,
		"correlation_id", correlationID)

	var expiresAtStr *string
	if existingKey.ExpiresAt != nil {
		s := existingKey.ExpiresAt.Format(time.RFC3339)
		expiresAtStr = &s
	}

	resp := APIKeyResponse{
		ID:          keyID,
		Name:        existingKey.Name,
		Permissions: existingKey.Permissions,
		AllowedIPs:  existingKey.AllowedIPs,
		Key:         newRawKey,
		IsActive:    true,
		ExpiresAt:   expiresAtStr,
		CreatedAt:   existingKey.CreatedAt.Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, models.Response{Data: resp})
}

// DeleteAPIKey handles DELETE /api-keys/:id - revokes an API key.
func (h *CustomerHandler) DeleteAPIKey(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	keyID := c.Param("id")
	correlationID := middleware.GetCorrelationID(c)

	if _, err := uuid.Parse(keyID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_KEY_ID", "API Key ID must be a valid UUID")
		return
	}

	existingKey, err := h.apiKeyRepo.GetByIDAndCustomer(c.Request.Context(), keyID, customerID)
	if err != nil {
		if errors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "NOT_FOUND", "API key not found")
			return
		}
		h.logger.Error("failed to get API key for revocation",
			"key_id", keyID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", correlationID)
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to revoke API key")
		return
	}

	if !existingKey.IsActive {
		middleware.RespondWithError(c, http.StatusBadRequest, "KEY_REVOKED", "API key is already revoked")
		return
	}

	if err := h.apiKeyRepo.Revoke(c.Request.Context(), keyID, customerID); err != nil {
		h.logger.Error("failed to revoke API key",
			"key_id", keyID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", correlationID)
		middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to revoke API key")
		return
	}

	h.logAudit(c, "api_key.revoke", "api_key", keyID, map[string]any{
		"name": existingKey.Name,
	}, true)

	h.logger.Info("API key revoked",
		"key_id", keyID,
		"customer_id", customerID,
		"correlation_id", correlationID)

	c.Status(http.StatusNoContent)
}

// hashAPIKey returns the hex-encoded SHA-256 hash of a raw API key.
func hashAPIKey(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}

// logAudit creates an audit log entry for customer operations.
// resourceType identifies the kind of entity being acted upon (e.g. "api_key", "ip_address").
// Sensitive fields are automatically masked before logging.
func (h *CustomerHandler) logAudit(c *gin.Context, action, resourceType, resourceID string, changes map[string]any, success bool) {
	if h.auditRepo == nil {
		return
	}

	customerID := middleware.GetUserID(c)
	actorIP := c.ClientIP()
	correlationID := middleware.GetCorrelationID(c)

	// Mask sensitive fields before logging
	maskedChanges := audit.MaskSensitiveFields(changes)
	changesJSON, _ := json.Marshal(maskedChanges)

	auditLog := &models.AuditLog{
		ActorID:       &customerID,
		ActorType:     models.AuditActorCustomer,
		ActorIP:       &actorIP,
		Action:        action,
		ResourceType:  resourceType,
		ResourceID:    &resourceID,
		Changes:       changesJSON,
		CorrelationID: &correlationID,
		Success:       success,
	}

	if err := h.auditRepo.Append(c.Request.Context(), auditLog); err != nil {
		h.logger.Error("failed to write audit log",
			"action", action,
			"resource_id", resourceID,
			"error", err)
	}
}

// CustomerAPIKeyValidator returns a function that validates customer API keys.
// It returns the key ID, customer ID, permissions, allowed IPs, and VM IDs for valid keys.
// Returns an error if the key is not found, revoked, or expired.
func CustomerAPIKeyValidator(repo *repository.CustomerAPIKeyRepository) middleware.CustomerAPIKeyValidator {
	return func(ctx context.Context, keyHash string) (middleware.CustomerAPIKeyInfo, error) {
		key, err := repo.GetByHash(ctx, keyHash)
		if err != nil {
			return middleware.CustomerAPIKeyInfo{}, err
		}

		// Check expiration if set
		if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
			return middleware.CustomerAPIKeyInfo{}, errors.New("API key has expired")
		}

		return middleware.CustomerAPIKeyInfo{
			KeyID:       key.ID,
			CustomerID:  key.CustomerID,
			Permissions: key.Permissions,
			AllowedIPs:  key.AllowedIPs,
			VMIDs:       key.VMIDs,
		}, nil
	}
}
