package provisioning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/gin-gonic/gin"
)

const provisioningKeyLastUsedUpdateTimeout = 5 * time.Second

// APIKeyValidatorFunc creates an APIKeyValidator function using the repository.
// This function is used by the APIKeyAuth middleware to validate provisioning API keys.
// On successful validation it also updates the key's last_used_at timestamp.
// The metadata update is best-effort, runs on the request path with a bounded
// timeout, and failures are logged without rejecting an otherwise valid API key.
func APIKeyValidatorFunc(apiKeyRepo *repository.ProvisioningKeyRepository) middleware.APIKeyValidator {
	return func(ctx context.Context, keyHash string) (string, []string, error) {
		key, err := apiKeyRepo.GetByHash(ctx, keyHash)
		if err != nil {
			return "", nil, err
		}
		recordProvisioningKeyLastUsed(ctx, apiKeyRepo, key.ID)
		return key.ID, key.AllowedIPs, nil
	}
}

func recordProvisioningKeyLastUsed(ctx context.Context, apiKeyRepo *repository.ProvisioningKeyRepository, keyID string) {
	if apiKeyRepo == nil || keyID == "" {
		return
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, provisioningKeyLastUsedUpdateTimeout)
	defer cancel()

	if err := apiKeyRepo.UpdateLastUsed(timeoutCtx, keyID); err != nil {
		slog.Warn("failed to update provisioning key last used",
			"key_id", keyID,
			"error", err,
		)
	}
}

// HashAPIKey computes the SHA-256 hash of an API key.
// This is used to look up keys in the database without storing plaintext.
func HashAPIKey(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}

// ProvisioningAuditLogger creates an AuditLogger for provisioning API requests.
func ProvisioningAuditLogger(auditRepo *repository.AuditRepository) middleware.AuditLogger {
	return func(ctx context.Context, entry *middleware.AuditEntry) error {
		audit := &models.AuditLog{
			ActorID:       &entry.ActorID,
			ActorType:     entry.ActorType,
			ActorIP:       &entry.ActorIP,
			Action:        entry.Action,
			ResourceType:  entry.ResourceType,
			ResourceID:    &entry.ResourceID,
			CorrelationID: &entry.CorrelationID,
			Success:       entry.Success,
			ErrorMessage:  &entry.ErrorMessage,
		}
		if entry.Changes != nil {
			if data, err := json.Marshal(entry.Changes); err == nil {
				audit.Changes = data
			}
		}
		return auditRepo.Append(ctx, audit)
	}
}

// requireProvisioningAuth is a defense-in-depth middleware that verifies the
// provisioning API key has been authenticated and its ID is present in the
// request context. This guards against middleware ordering changes that could
// accidentally bypass the upstream APIKeyAuth middleware.
func requireProvisioningAuth(c *gin.Context) {
	keyID, exists := c.Get("api_key_id")
	if !exists || keyID == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error":   "MISSING_API_KEY",
			"message": "provisioning API key authentication is required",
		})
		return
	}
	c.Next()
}

// registerRoutes registers all provisioning VM and task routes onto the given group.
// This shared helper is called by both RegisterProvisioningRoutes and any future
// route registration variants to avoid duplication.
func registerRoutes(group *gin.RouterGroup, handler *ProvisioningHandler) {
	// Defense-in-depth: verify API key auth context is present regardless of
	// how this group was assembled. This is a secondary check; primary auth is
	// the APIKeyAuth middleware applied by RegisterProvisioningRoutes.
	group.Use(requireProvisioningAuth)

	vms := group.Group("/vms")
	{
		vms.POST("", handler.CreateVM)
		vms.GET("/:id", handler.GetVMInfo)
		vms.GET("/by-service/:service_id", handler.GetVMByExternalServiceID)
		vms.DELETE("/:id", handler.DeleteVM)
		vms.POST("/:id/suspend", handler.SuspendVM)
		vms.POST("/:id/unsuspend", handler.UnsuspendVM)
		vms.POST("/:id/resize", handler.ResizeVM)
		vms.POST("/:id/password", handler.SetPassword)
		vms.POST("/:id/password/reset", handler.ResetPassword)
		vms.POST("/:id/power", handler.PowerOperation)
		vms.GET("/:id/status", handler.GetStatus)
		vms.GET("/:id/usage", handler.GetVMUsage)
		vms.GET("/:id/rdns", handler.GetVMRDNS)
		vms.PUT("/:id/rdns", handler.SetVMRDNS)
	}

	tasks := group.Group("/tasks")
	{
		tasks.GET("/:id", handler.GetTask)
	}

	group.POST("/sso-tokens", handler.CreateSSOToken)
	group.POST("/customers", handler.CreateOrGetCustomer)

	plans := group.Group("/plans")
	{
		plans.GET("", handler.ListPlans)
		plans.GET("/:id", handler.GetPlan)
	}
}

// RegisterProvisioningRoutes registers all provisioning API routes.
// These routes are designed for billing module integration and use API key authentication.
//
// Base path: /api/v1/provisioning
// Authentication: X-API-Key header (validated against provisioning_keys table)
//
// Endpoints:
//
//	POST   /vms                    - Create a new VM (async, returns task_id)
//	GET    /vms/:id                - Get VM details by ID
//	GET    /vms/by-service/:id     - Get VM by external billing service ID
//	DELETE /vms/:id                - Terminate a VM (async, returns task_id)
//	POST   /vms/:id/suspend        - Suspend a VM (billing suspension)
//	POST   /vms/:id/unsuspend      - Unsuspend a VM
//	POST   /vms/:id/resize         - Resize VM resources
//	POST   /vms/:id/password       - Set root password
//	POST   /vms/:id/password/reset - Reset root password
//	POST   /vms/:id/power          - Power operations (start/stop/restart)
//	GET    /vms/:id/status         - Get VM status
//	GET    /vms/:id/usage          - Get VM resource usage (bandwidth, disk)
//	GET    /tasks/:id              - Get task status
//	POST   /sso-tokens             - Create SSO token
//	POST   /customers              - Create or get customer by email
//	GET    /plans                  - List all active plans
//	GET    /plans/:id              - Get plan details by ID
func RegisterProvisioningRoutes(router *gin.RouterGroup, handler *ProvisioningHandler, apiKeyRepo *repository.ProvisioningKeyRepository, auditRepo *repository.AuditRepository) {
	apiKeyValidator := APIKeyValidatorFunc(apiKeyRepo)

	provisioning := router.Group("/provisioning")
	provisioning.Use(middleware.APIKeyAuth(apiKeyValidator))
	provisioning.Use(middleware.ProvisioningRateLimit())
	provisioning.Use(middleware.Audit(ProvisioningAuditLogger(auditRepo)))

	registerRoutes(provisioning, handler)
}
