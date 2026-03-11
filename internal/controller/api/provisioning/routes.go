package provisioning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
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
func RegisterProvisioningRoutes(router *gin.RouterGroup, handler *ProvisioningHandler, apiKeyRepo *repository.ProvisioningKeyRepository) {
	// Create API key validator
	apiKeyValidator := APIKeyValidatorFunc(apiKeyRepo)

	// All provisioning routes require API key authentication
	provisioning := router.Group("/provisioning")
	provisioning.Use(middleware.APIKeyAuth(apiKeyValidator))
	{
		// VM CRUD operations
		vms := provisioning.Group("/vms")
		{
			// POST /vms - Create VM
			vms.POST("", handler.CreateVM)

			// GET /vms/:id - Get VM by ID
			vms.GET("/:id", handler.GetVMInfo)

			// GET /vms/by-service/:service_id - Get VM by WHMCS service ID
			vms.GET("/by-service/:service_id", handler.GetVMByWHMCSServiceID)

			// DELETE /vms/:id - Terminate VM
			vms.DELETE("/:id", handler.DeleteVM)

			// POST /vms/:id/suspend - Suspend VM
			vms.POST("/:id/suspend", handler.SuspendVM)

			// POST /vms/:id/unsuspend - Unsuspend VM
			vms.POST("/:id/unsuspend", handler.UnsuspendVM)

			// POST /vms/:id/resize - Resize VM
			vms.POST("/:id/resize", handler.ResizeVM)

			// POST /vms/:id/password - Set password
			vms.POST("/:id/password", handler.SetPassword)

			// POST /vms/:id/password/reset - Reset password
			vms.POST("/:id/password/reset", handler.ResetPassword)

			// POST /vms/:id/power - Power operations
			vms.POST("/:id/power", handler.PowerOperation)

			// GET /vms/:id/status - Get status
			vms.GET("/:id/status", handler.GetStatus)
		}

		// Task polling
		tasks := provisioning.Group("/tasks")
		{
			// GET /tasks/:id - Get task status
			tasks.GET("/:id", handler.GetTask)
		}
	}
}

// RegisterProvisioningRoutesSimple registers provisioning routes without API key validation.
// This is useful for development/testing or when API key validation is handled externally.
// WARNING: Do not use in production without additional authentication.
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