package customer

import (
	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/gin-gonic/gin"
)

// RegisterCustomerRoutes registers all customer API routes.
// These routes are for customer self-service operations.
//
// Base path: /api/v1/customer
// Authentication: JWT Bearer token (validated via middleware.JWTAuth)
//
// All endpoints enforce customer isolation - users can only access their own resources.
//
// Endpoints:
//
// Authentication:
//
//	  POST /auth/login        - Email/password login (returns tokens or 2FA challenge)
//	  POST /auth/verify-2fa   - TOTP verification for 2FA-enabled accounts
//	  POST /auth/refresh      - Refresh access token using refresh token
//	  POST /auth/logout       - Logout current session
//	  PUT   /password         - Change password (requires auth, rate limited)
//
//	VMs:
//	  GET    /vms             - List customer's VMs
//	  GET    /vms/:id         - Get VM details
//
//	Power Control:
//	  POST   /vms/:id/start       - Start VM
//	  POST   /vms/:id/stop        - Graceful stop VM
//	  POST   /vms/:id/restart     - Restart VM
//	  POST   /vms/:id/force-stop  - Force power off
//
//	Console:
//	  POST   /vms/:id/console-token - Get NoVNC token (1hr expiry)
//	  POST   /vms/:id/serial-token  - Get serial console token
//
//	Monitoring:
//	  GET    /vms/:id/metrics    - Real-time CPU/memory/disk stats
//	  GET    /vms/:id/bandwidth  - Bandwidth usage (current period)
//	  GET    /vms/:id/network    - Network traffic history
//
//	Backups:
//	  GET    /backups            - List customer's backups
//	  POST   /backups            - Create backup (async)
//	  GET    /backups/:id        - Get backup details
//	  DELETE /backups/:id        - Delete backup
//	  POST   /backups/:id/restore - Restore backup (async)
//
//	Snapshots:
//	  GET    /snapshots          - List customer's snapshots
//	  POST   /snapshots          - Create snapshot
//	  DELETE /snapshots/:id      - Delete snapshot
//	  POST   /snapshots/:id/restore - Restore snapshot (async)
//
//	API Keys:
//	  GET    /api-keys           - List API keys
//	  POST   /api-keys           - Create API key
//	  POST   /api-keys/:id/rotate - Rotate API key (generate new key value)
//	  DELETE /api-keys/:id       - Revoke API key
//
//	Webhooks:
//	  GET    /webhooks           - List webhooks
//	  POST   /webhooks           - Create webhook
//	  PUT    /webhooks/:id       - Update webhook
//	  DELETE /webhooks/:id       - Delete webhook
//	  GET    /webhooks/:id/deliveries - List delivery attempts
//
//	Templates:
//	  GET    /templates          - List available OS templates
//
//	Notifications:
//	  GET    /notifications/preferences   - Get notification preferences
//	  PUT    /notifications/preferences   - Update notification preferences
//	  GET    /notifications/events        - List notification events
//	  GET    /notifications/events/types  - Get available event types
//
//	ISO Management:
//	  POST   /vms/:id/iso/upload            - Upload ISO file (multipart)
//	  GET    /vms/:id/iso                   - List uploaded ISOs
//	  DELETE /vms/:id/iso/:isoId            - Delete an ISO
//	  POST   /vms/:id/iso/:isoId/attach     - Attach ISO to VM
//	  POST   /vms/:id/iso/:isoId/detach     - Detach ISO from VM
//
//	2FA (Two-Factor Authentication):
//	  POST   /2fa/initiate  - Generate TOTP secret and QR URL
//	  POST   /2fa/enable    - Verify TOTP code and enable 2FA
//	  POST   /2fa/disable   - Disable 2FA (requires password)
//	  GET    /2fa/status    - Get 2FA status
//	  GET    /2fa/backup-codes          - Get backup codes (only once)
//	  POST   /2fa/backup-codes/regenerate - Regenerate new backup codes
func RegisterCustomerRoutes(router *gin.RouterGroup, handler *CustomerHandler, notifyHandler *NotificationsHandler) {
	// Create the customer API group
	customer := router.Group("/customer")

	// Auth endpoints - no authentication required for login/verify-2fa/refresh
	auth := customer.Group("/auth")
	{
		auth.POST("/login", handler.Login)
		auth.POST("/verify-2fa", handler.Verify2FA)
		auth.POST("/refresh", handler.RefreshToken)
	}

	// All other endpoints require JWT authentication
	protected := customer.Group("")
	protected.Use(middleware.JWTAuth(handler.authConfig))
	protected.Use(middleware.RequireUserType("customer"))
	protected.Use(middleware.CSRF(middleware.DefaultCSRFConfig()))
	{
		protected.POST("/auth/logout", handler.Logout)

		protected.PUT("/password", middleware.PasswordChangeRateLimit(), handler.ChangePassword)

		protected.GET("/profile", handler.GetProfile)
		protected.PUT("/profile", handler.UpdateProfile)

		// VM operations
		vms := protected.Group("/vms")
		{
			vms.GET("", handler.ListVMs)
			vms.GET("/:id", handler.GetVM)

			// Power control
			vms.POST("/:id/start", handler.StartVM)
			vms.POST("/:id/stop", handler.StopVM)
			vms.POST("/:id/restart", handler.RestartVM)
			vms.POST("/:id/force-stop", handler.ForceStopVM)

			// Console access
			vms.POST("/:id/console-token", handler.GetConsoleToken)
			vms.POST("/:id/serial-token", handler.GetSerialToken)

			// Monitoring
			vms.GET("/:id/metrics", handler.GetMetrics)
			vms.GET("/:id/bandwidth", handler.GetBandwidth)
			vms.GET("/:id/network", handler.GetNetworkHistory)

			// rDNS management (customer can only manage their own IPs)
			vms.GET("/:id/ips", handler.ListVMIPs)
			vms.GET("/:id/ips/:ipId/rdns", handler.GetRDNS)
			vms.PUT("/:id/ips/:ipId/rdns", middleware.RDNSUpdateRateLimit(), handler.UpdateRDNS)
			vms.DELETE("/:id/ips/:ipId/rdns", handler.DeleteRDNS)

			// ISO management
			vms.POST("/:id/iso/upload", handler.UploadISO)
			vms.GET("/:id/iso", handler.ListISOs)
			vms.DELETE("/:id/iso/:isoId", handler.DeleteISO)
			vms.POST("/:id/iso/:isoId/attach", handler.AttachISO)
			vms.POST("/:id/iso/:isoId/detach", handler.DetachISO)
		}

		// Backups
		backups := protected.Group("/backups")
		{
			backups.GET("", handler.ListBackups)
			backups.POST("", handler.CreateBackup)
			backups.GET("/:id", handler.GetBackup)
			backups.DELETE("/:id", handler.DeleteBackup)
			backups.POST("/:id/restore", handler.RestoreBackup)
		}

		// Snapshots
		snapshots := protected.Group("/snapshots")
		{
			snapshots.GET("", handler.ListSnapshots)
			snapshots.POST("", handler.CreateSnapshot)
			snapshots.DELETE("/:id", handler.DeleteSnapshot)
			snapshots.POST("/:id/restore", handler.RestoreSnapshot)
		}

		// API Keys
		apiKeys := protected.Group("/api-keys")
		{
			apiKeys.GET("", handler.ListAPIKeys)
			apiKeys.POST("", handler.CreateAPIKey)
			apiKeys.POST("/:id/rotate", handler.RotateAPIKey)
			apiKeys.DELETE("/:id", handler.DeleteAPIKey)
		}

		// Webhooks
		webhooks := protected.Group("/webhooks")
		{
			webhooks.GET("", handler.ListWebhooks)
			webhooks.POST("", handler.CreateWebhook)
			webhooks.GET("/:id", handler.GetWebhook)
			webhooks.PUT("/:id", handler.UpdateWebhook)
			webhooks.DELETE("/:id", handler.DeleteWebhook)
			webhooks.GET("/:id/deliveries", handler.ListWebhookDeliveries)
		}

		// Templates
		protected.GET("/templates", handler.ListTemplates)

		// 2FA (Two-Factor Authentication)
		twofa := protected.Group("/2fa")
		{
			twofa.POST("/initiate", handler.Initiate2FA)
			twofa.POST("/enable", handler.Enable2FA)
			twofa.POST("/disable", handler.Disable2FA)
			twofa.GET("/status", handler.Get2FAStatus)
			twofa.GET("/backup-codes", handler.GetBackupCodes)
			twofa.POST("/backup-codes/regenerate", handler.RegenerateBackupCodes)
		}

		// WebSocket console endpoints
		protected.GET("/ws/vnc/:vmId", handler.HandleVNCWebSocket)
		protected.GET("/ws/serial/:vmId", handler.HandleSerialWebSocket)

		// Notifications
		if notifyHandler != nil {
			RegisterNotificationRoutes(protected, notifyHandler)
		}
	}
}
