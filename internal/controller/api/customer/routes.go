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
func RegisterCustomerRoutes(router *gin.RouterGroup, handler *CustomerHandler, notifyHandler *NotificationsHandler) {
	customer := router.Group("/customer")

	registerAuthRoutes(customer, handler)

	protected := customer.Group("")
	protected.Use(middleware.JWTAuth(handler.authConfig))
	protected.Use(middleware.RequireUserType("customer"))
	protected.Use(middleware.CSRF(middleware.DefaultCSRFConfig()))

	registerVMRoutes(protected, handler)
	registerBackupRoutes(protected, handler)
	registerSnapshotRoutes(protected, handler)
	registerAPIKeyRoutes(protected, handler)
	registerWebhookRoutes(protected, handler)
	register2FARoutes(protected, handler)

	protected.GET("/templates", handler.ListTemplates)
	protected.GET("/ws/vnc/:vmId", handler.HandleVNCWebSocket)
	protected.GET("/ws/serial/:vmId", handler.HandleSerialWebSocket)

	if notifyHandler != nil {
		RegisterNotificationRoutes(protected, notifyHandler)
	}
}

// registerAuthRoutes registers authentication endpoints (no JWT required).
func registerAuthRoutes(customer *gin.RouterGroup, handler *CustomerHandler) {
	auth := customer.Group("/auth")
	{
		auth.POST("/login", handler.Login)
		auth.POST("/verify-2fa", handler.Verify2FA)
		auth.POST("/refresh", handler.RefreshToken)
	}
}

// registerVMRoutes registers VM-related endpoints.
func registerVMRoutes(protected *gin.RouterGroup, handler *CustomerHandler) {
	protected.POST("/auth/logout", handler.Logout)
	protected.PUT("/password", middleware.PasswordChangeRateLimit(), handler.ChangePassword)
	protected.GET("/profile", handler.GetProfile)
	protected.PUT("/profile", handler.UpdateProfile)

	vms := protected.Group("/vms")
	{
		vms.GET("", handler.ListVMs)
		vms.GET("/:id", handler.GetVM)

		registerPowerRoutes(vms, handler)
		registerConsoleRoutes(vms, handler)
		registerMonitoringRoutes(vms, handler)
		registerRDNSRoutes(vms, handler)
		registerISORoutes(vms, handler)
	}
}

// registerPowerRoutes registers VM power control endpoints.
func registerPowerRoutes(vms *gin.RouterGroup, handler *CustomerHandler) {
	vms.POST("/:id/start", handler.StartVM)
	vms.POST("/:id/stop", handler.StopVM)
	vms.POST("/:id/restart", handler.RestartVM)
	vms.POST("/:id/force-stop", handler.ForceStopVM)
}

// registerConsoleRoutes registers console token endpoints.
func registerConsoleRoutes(vms *gin.RouterGroup, handler *CustomerHandler) {
	vms.POST("/:id/console-token", handler.GetConsoleToken)
	vms.POST("/:id/serial-token", handler.GetSerialToken)
}

// registerMonitoringRoutes registers VM monitoring endpoints.
func registerMonitoringRoutes(vms *gin.RouterGroup, handler *CustomerHandler) {
	vms.GET("/:id/metrics", handler.GetMetrics)
	vms.GET("/:id/bandwidth", handler.GetBandwidth)
	vms.GET("/:id/network", handler.GetNetworkHistory)
}

// registerRDNSRoutes registers rDNS management endpoints.
func registerRDNSRoutes(vms *gin.RouterGroup, handler *CustomerHandler) {
	vms.GET("/:id/ips", handler.ListVMIPs)
	vms.GET("/:id/ips/:ipId/rdns", handler.GetRDNS)
	vms.PUT("/:id/ips/:ipId/rdns", middleware.RDNSUpdateRateLimit(), handler.UpdateRDNS)
	vms.DELETE("/:id/ips/:ipId/rdns", handler.DeleteRDNS)
}

// registerISORoutes registers ISO management endpoints.
func registerISORoutes(vms *gin.RouterGroup, handler *CustomerHandler) {
	vms.POST("/:id/iso/upload", handler.UploadISO)
	vms.GET("/:id/iso", handler.ListISOs)
	vms.DELETE("/:id/iso/:isoId", handler.DeleteISO)
	vms.POST("/:id/iso/:isoId/attach", handler.AttachISO)
	vms.POST("/:id/iso/:isoId/detach", handler.DetachISO)
}

// registerBackupRoutes registers backup management endpoints.
func registerBackupRoutes(protected *gin.RouterGroup, handler *CustomerHandler) {
	backups := protected.Group("/backups")
	{
		backups.GET("", handler.ListBackups)
		backups.POST("", handler.CreateBackup)
		backups.GET("/:id", handler.GetBackup)
		backups.DELETE("/:id", handler.DeleteBackup)
		backups.POST("/:id/restore", handler.RestoreBackup)
	}
}

// registerSnapshotRoutes registers snapshot management endpoints.
func registerSnapshotRoutes(protected *gin.RouterGroup, handler *CustomerHandler) {
	snapshots := protected.Group("/snapshots")
	{
		snapshots.GET("", handler.ListSnapshots)
		snapshots.POST("", handler.CreateSnapshot)
		snapshots.DELETE("/:id", handler.DeleteSnapshot)
		snapshots.POST("/:id/restore", handler.RestoreSnapshot)
	}
}

// registerAPIKeyRoutes registers API key management endpoints.
func registerAPIKeyRoutes(protected *gin.RouterGroup, handler *CustomerHandler) {
	apiKeys := protected.Group("/api-keys")
	{
		apiKeys.GET("", handler.ListAPIKeys)
		apiKeys.POST("", handler.CreateAPIKey)
		apiKeys.POST("/:id/rotate", handler.RotateAPIKey)
		apiKeys.DELETE("/:id", handler.DeleteAPIKey)
	}
}

// registerWebhookRoutes registers webhook management endpoints.
func registerWebhookRoutes(protected *gin.RouterGroup, handler *CustomerHandler) {
	webhooks := protected.Group("/webhooks")
	{
		webhooks.GET("", handler.ListWebhooks)
		webhooks.POST("", handler.CreateWebhook)
		webhooks.GET("/:id", handler.GetWebhook)
		webhooks.PUT("/:id", handler.UpdateWebhook)
		webhooks.DELETE("/:id", handler.DeleteWebhook)
		webhooks.GET("/:id/deliveries", handler.ListWebhookDeliveries)
	}
}

// register2FARoutes registers two-factor authentication endpoints.
func register2FARoutes(protected *gin.RouterGroup, handler *CustomerHandler) {
	twofa := protected.Group("/2fa")
	{
		twofa.POST("/initiate", handler.Initiate2FA)
		twofa.POST("/enable", handler.Enable2FA)
		twofa.POST("/disable", handler.Disable2FA)
		twofa.GET("/status", handler.Get2FAStatus)
		twofa.GET("/backup-codes", handler.GetBackupCodes)
		twofa.POST("/backup-codes/regenerate", handler.RegenerateBackupCodes)
	}
}
