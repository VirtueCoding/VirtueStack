package customer

import (
	"net/http"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/google/uuid"

	"github.com/gin-gonic/gin"
)

// CreateAPIKeyRequest represents the request body for creating an API key.
type CreateAPIKeyRequest struct {
	Name        string   `json:"name" validate:"required,max=100"`
	Permissions []string `json:"permissions" validate:"required,min=1,dive,max=100"`
	ExpiresAt   *string  `json:"expires_at,omitempty"` // RFC3339 timestamp
}

// APIKeyResponse represents an API key in responses (includes the key on creation).
type APIKeyResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
	Key         string   `json:"key,omitempty"` // Only returned on creation
	IsActive    bool     `json:"is_active"`
	ExpiresAt   *string  `json:"expires_at,omitempty"`
	CreatedAt   string   `json:"created_at"`
}

// ListAPIKeys handles GET /api-keys - lists all API keys for the customer.
// Note: The actual key value is never returned (only metadata).
func (h *CustomerHandler) ListAPIKeys(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	// In a full implementation, this would query a customer_api_keys table
	// For now, return a placeholder response
	h.logger.Info("listing API keys",
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	// Placeholder response - in production, this would query the database
	keys := []APIKeyResponse{}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: keys,
		Meta: models.NewPaginationMeta(1, 20, 0),
	})
}

// CreateAPIKey handles POST /api-keys - creates a new API key for the customer.
// The key is returned once in the response and cannot be retrieved again.
func (h *CustomerHandler) CreateAPIKey(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body: "+err.Error())
		return
	}

	// Validate permissions
	validPermissions := map[string]bool{
		"vm:read":    true,
		"vm:write":   true,
		"vm:power":   true,
		"backup:read": true,
		"backup:write": true,
		"snapshot:read": true,
		"snapshot:write": true,
	}

	for _, perm := range req.Permissions {
		if !validPermissions[perm] {
			respondWithError(c, http.StatusBadRequest, "INVALID_PERMISSION", "Invalid permission: "+perm)
			return
		}
	}

	// Generate API key
	keyID := uuid.New().String()
	keyValue := "vs_" + uuid.New().String() // Prefix with "vs_" to identify VirtueStack keys

	// Parse expiration if provided
	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		parsed, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			respondWithError(c, http.StatusBadRequest, "INVALID_EXPIRATION", "Invalid expiration timestamp format")
			return
		}
		if parsed.Before(time.Now()) {
			respondWithError(c, http.StatusBadRequest, "INVALID_EXPIRATION", "Expiration must be in the future")
			return
		}
		expiresAt = &parsed
	}

	// In a full implementation, this would:
	// 1. Hash the key for storage
	// 2. Store the key metadata in customer_api_keys table
	// 3. Return the plaintext key once

	h.logger.Info("API key created",
		"key_id", keyID,
		"customer_id", customerID,
		"name", req.Name,
		"correlation_id", middleware.GetCorrelationID(c))

	var expiresAtStr *string
	if expiresAt != nil {
		s := expiresAt.Format(time.RFC3339)
		expiresAtStr = &s
	}

	resp := APIKeyResponse{
		ID:          keyID,
		Name:        req.Name,
		Permissions: req.Permissions,
		Key:         keyValue, // Only returned on creation
		IsActive:    true,
		ExpiresAt:   expiresAtStr,
		CreatedAt:   time.Now().Format(time.RFC3339),
	}

	c.JSON(http.StatusCreated, models.Response{Data: resp})
}

// DeleteAPIKey handles DELETE /api-keys/:id - revokes an API key.
func (h *CustomerHandler) DeleteAPIKey(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	keyID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(keyID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_KEY_ID", "API Key ID must be a valid UUID")
		return
	}

	// In a full implementation, this would:
	// 1. Verify the key belongs to the customer
	// 2. Delete or deactivate the key

	h.logger.Info("API key revoked",
		"key_id", keyID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "API key revoked successfully"}})
}