package provisioning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/gin-gonic/gin"
)

// APIKeyValidatorFunc creates an APIKeyValidator function using the repository.
// This function is used by the APIKeyAuth middleware to validate provisioning API keys.
func APIKeyValidatorFunc(apiKeyRepo *repository.ProvisioningKeyRepository) middleware.APIKeyValidator {
	return func(ctx context.Context, keyHash string) (string, []string, error) {
		key, err := apiKeyRepo.GetByHash(ctx, keyHash)
		if err != nil {
			return "", nil, err
		}
		return key.ID, key.AllowedIPs, nil
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

// RegisterProvisioningRoutes registers all provisioning API routes.
// These routes are designed for WHMCS integration and use API key authentication.
//
// Base path: /api/v1/provisioning
// Authentication: X-API-Key header (validated against provisioning_keys table)
//
// Endpoints:
//   POST   /vms                    - Create a new VM (async, returns task_id)
//   GET    /vms/:id                - Get VM details by ID
//   GET    /vms/by-service/:id     - Get VM by WHMCS service ID
//   DELETE /vms/:id                - Terminate a VM (async, returns task_id)
//   POST   /vms/:id/suspend        - Suspend a VM (billing suspension)
//   POST   /vms/:id/unsuspend      - Unsuspend a VM
//   POST   /vms/:id/resize         - Resize VM resources
//   POST   /vms/:id/password       - Set root password
//   POST   /vms/:id/password/reset - Reset root password
//   POST   /vms/:id/power          - Power operations (start/stop/restart)
//   GET    /vms/:id/status         - Get VM status
//   GET    /tasks/:id              - Get task status
func RegisterProvisioningRoutes(router *gin.RouterGroup, handler *ProvisioningHandler, apiKeyRepo *repository.ProvisioningKeyRepository, auditRepo *repository.AuditRepository) {
	apiKeyValidator := APIKeyValidatorFunc(apiKeyRepo)

	provisioning := router.Group("/provisioning")
	provisioning.Use(middleware.APIKeyAuth(apiKeyValidator))
	provisioning.Use(middleware.ProvisioningRateLimit())
	provisioning.Use(middleware.Audit(ProvisioningAuditLogger(auditRepo)))
	{
		vms := provisioning.Group("/vms")
		{
			vms.POST("", handler.CreateVM)
			vms.GET("/:id", handler.GetVMInfo)
			vms.GET("/by-service/:service_id", handler.GetVMByWHMCSServiceID)
			vms.DELETE("/:id", handler.DeleteVM)
			vms.POST("/:id/suspend", handler.SuspendVM)
			vms.POST("/:id/unsuspend", handler.UnsuspendVM)
			vms.POST("/:id/resize", handler.ResizeVM)
			vms.POST("/:id/password", handler.SetPassword)
			vms.POST("/:id/password/reset", handler.ResetPassword)
			vms.POST("/:id/power", handler.PowerOperation)
			vms.GET("/:id/status", handler.GetStatus)
		}

		tasks := provisioning.Group("/tasks")
		{
			tasks.GET("/:id", handler.GetTask)
		}
	}
}

// RegisterProvisioningRoutesSimple registers provisioning routes without API key validation.
// This is intended only for development/testing environments.
func RegisterProvisioningRoutesSimple(router *gin.RouterGroup, handler *ProvisioningHandler) {
	provisioning := router.Group("/provisioning")
	{
		vms := provisioning.Group("/vms")
		{
			vms.POST("", handler.CreateVM)
			vms.GET("/:id", handler.GetVMInfo)
			vms.GET("/by-service/:service_id", handler.GetVMByWHMCSServiceID)
			vms.DELETE("/:id", handler.DeleteVM)
			vms.POST("/:id/suspend", handler.SuspendVM)
			vms.POST("/:id/unsuspend", handler.UnsuspendVM)
			vms.POST("/:id/resize", handler.ResizeVM)
			vms.POST("/:id/password", handler.SetPassword)
			vms.POST("/:id/password/reset", handler.ResetPassword)
			vms.POST("/:id/power", handler.PowerOperation)
			vms.GET("/:id/status", handler.GetStatus)
		}

		tasks := provisioning.Group("/tasks")
		{
			tasks.GET("/:id", handler.GetTask)
		}
	}
}