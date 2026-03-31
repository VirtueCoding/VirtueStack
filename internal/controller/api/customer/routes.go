package customer

import (
	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/gin-gonic/gin"
)

// Permission constants for customer API key authorization.
const (
	PermissionVMRead       = "vm:read"
	PermissionVMWrite      = "vm:write"
	PermissionVMPower      = "vm:power"
	PermissionBackupRead   = "backup:read"
	PermissionBackupWrite  = "backup:write"
	PermissionSnapshotRead = "snapshot:read"
	PermissionSnapshotWrite = "snapshot:write"
)

// BillingRoutesConfig controls conditional registration of billing and OAuth routes.
type BillingRoutesConfig struct {
	NativeBillingEnabled bool
	OAuthGoogleEnabled   bool
	OAuthGitHubEnabled   bool
}

// RegisterCustomerRoutes registers all customer API routes.
// These routes are for customer self-service operations.
//
// Base path: /api/v1/customer
// Authentication: JWT Bearer token OR X-API-Key header (when apiKeyRepo is provided)
//
// All endpoints enforce customer isolation - users can only access their own resources.
//
// When apiKeyRepo is provided, routes support dual authentication:
//   - JWT tokens (browser sessions via cookies or Authorization header)
//   - Customer API keys (programmatic access via X-API-Key header)
//
// Permission enforcement:
//   - JWT authentication: Full access to all endpoints
//   - API key authentication: Limited to granted permissions (vm:read, vm:write, etc.)
//
// Account management endpoints (profile, 2FA, webhooks, API keys) require JWT authentication.
// CSRF protection is applied only for JWT-authenticated requests; API key requests are exempt.
func RegisterCustomerRoutes(
	router *gin.RouterGroup,
	handler *CustomerHandler,
	notifyHandler *NotificationsHandler,
	inAppNotifHandler *InAppNotificationsHandler,
	apiKeyRepo *repository.CustomerAPIKeyRepository,
	allowSelfRegistration bool,
	billingCfg BillingRoutesConfig,
) {
	customer := router.Group("/customer")

	registerAuthRoutes(customer, handler, allowSelfRegistration)

	// Account management routes - JWT only, no API key access
	// These handle sensitive account-level operations that should require browser session.
	accountGroup := customer.Group("")
	accountGroup.Use(middleware.JWTAuth(handler.authConfig))
	accountGroup.Use(middleware.RequireUserType("customer"))
	accountGroup.Use(middleware.CSRF(middleware.DefaultCSRFConfig()))
	registerAccountRoutes(accountGroup, handler)
	registerAPIKeyRoutes(accountGroup, handler)
	registerWebhookRoutes(accountGroup, handler)
	register2FARoutes(accountGroup, handler)

	// VM/backup/snapshot routes - support both JWT and API key authentication
	protected := customer.Group("")
	if apiKeyRepo != nil {
		keyValidator := CustomerAPIKeyValidator(apiKeyRepo, handler.encryptionKey)
		protected.Use(middleware.JWTOrCustomerAPIKeyAuth(handler.authConfig, keyValidator))
	} else {
		protected.Use(middleware.JWTAuth(handler.authConfig))
	}
	protected.Use(middleware.RequireUserType("customer"))
	protected.Use(middleware.SkipCSRFForAPIKey(middleware.DefaultCSRFConfig()))
	// F-064: Apply global rate limiting to the protected customer group.
	// Uses method-based rate limiting to prevent write operations from consuming read quota.
	protected.Use(middleware.CustomerRateLimits())

	registerVMRoutes(protected, handler)
	registerBackupRoutes(protected, handler)
	registerSnapshotRoutes(protected, handler)

	// Task status polling - allows customers to track async operations
	protected.GET("/tasks/:id", handler.GetTaskStatus)

	protected.GET("/templates", handler.ListTemplates)
	protected.GET("/ws/vnc/:vmId", middleware.RequireVMScope(), handler.HandleVNCWebSocket)
	protected.GET("/ws/serial/:vmId", middleware.RequireVMScope(), handler.HandleSerialWebSocket)

	if notifyHandler != nil {
		RegisterNotificationRoutes(protected, notifyHandler)
	}

	if inAppNotifHandler != nil {
		registerInAppNotificationRoutes(customer, protected, inAppNotifHandler, handler.authConfig)
	}

	// Conditional billing routes (stubs for future phases)
	if billingCfg.NativeBillingEnabled {
		registerBillingRoutes(accountGroup, handler)
	}

	// Conditional OAuth routes (stubs for Phase 8)
	if billingCfg.OAuthGoogleEnabled {
		// Google OAuth routes will be added in Phase 8
	}
	if billingCfg.OAuthGitHubEnabled {
		// GitHub OAuth routes will be added in Phase 8
	}
}

// registerAuthRoutes registers authentication endpoints (no auth required).
// F-007: LoginRateLimit is applied to /login and /verify-2fa to protect against
// brute-force attacks. A separate refresh rate limit applies to /refresh.
func registerAuthRoutes(customer *gin.RouterGroup, handler *CustomerHandler, allowSelfRegistration bool) {
	auth := customer.Group("/auth")
	{
		// GET /auth/csrf sets the CSRF cookie and returns the token in the X-CSRF-Token header.
		// This endpoint is used by the customer portal frontend to bootstrap CSRF protection
		// before the user logs in (the login page needs CSRF protection too).
		auth.GET("/csrf", middleware.CSRF(middleware.DefaultCSRFConfig()), handler.CSRF)
		auth.POST("/login", middleware.LoginRateLimit(), handler.Login)
		auth.POST("/verify-2fa", middleware.LoginRateLimit(), handler.Verify2FA)
		auth.POST("/refresh", middleware.RefreshRateLimit(), handler.RefreshToken)
		auth.GET("/sso-exchange", middleware.LoginRateLimit(), handler.ExchangeSSOToken)

		// Password reset flow (no auth required, rate-limited to prevent abuse)
		auth.POST("/forgot-password", middleware.PasswordResetRateLimit(), handler.ForgotPassword)
		auth.POST("/reset-password", middleware.PasswordResetRateLimit(), handler.ResetPassword)
		if allowSelfRegistration {
			auth.POST("/register", middleware.RegistrationRateLimit(), handler.Register)
			auth.POST("/verify-email", middleware.RegistrationRateLimit(), handler.VerifyEmail)
		}
	}
}

// registerAccountRoutes registers account management endpoints (JWT-only, no API key).
func registerAccountRoutes(protected *gin.RouterGroup, handler *CustomerHandler) {
	protected.POST("/auth/logout", handler.Logout)
	protected.PUT("/password", middleware.PasswordChangeRateLimit(), handler.ChangePassword)
	protected.GET("/profile", handler.GetProfile)
	protected.PUT("/profile", handler.UpdateProfile)
}

// registerVMRoutes registers VM-related endpoints with permission enforcement.
func registerVMRoutes(protected *gin.RouterGroup, handler *CustomerHandler) {
	vms := protected.Group("/vms")

	// Read operations require vm:read
	vms.GET("", middleware.RequirePermission(PermissionVMRead), handler.ListVMs)
	vms.GET("/:id", middleware.RequirePermission(PermissionVMRead), middleware.RequireVMScope(), handler.GetVM)

	registerPowerRoutes(vms, handler)
	registerConsoleRoutes(vms, handler)
	registerMonitoringRoutes(vms, handler)
	registerRDNSRoutes(vms, handler)
	registerISORoutes(vms, handler)
}

// registerPowerRoutes registers VM power control endpoints (requires vm:power).
func registerPowerRoutes(vms *gin.RouterGroup, handler *CustomerHandler) {
	power := vms.Group("")
	power.Use(middleware.RequirePermission(PermissionVMPower))
	{
		power.POST("/:id/start", middleware.RequireVMScope(), handler.StartVM)
		power.POST("/:id/stop", middleware.RequireVMScope(), handler.StopVM)
		power.POST("/:id/restart", middleware.RequireVMScope(), handler.RestartVM)
		power.POST("/:id/force-stop", middleware.RequireVMScope(), handler.ForceStopVM)
	}
}

// registerConsoleRoutes registers console token endpoints (requires vm:power).
// Console access is considered a power operation as it gives interactive control.
func registerConsoleRoutes(vms *gin.RouterGroup, handler *CustomerHandler) {
	console := vms.Group("")
	console.Use(middleware.RequirePermission(PermissionVMPower))
	{
		console.POST("/:id/console-token", middleware.RequireVMScope(), handler.GetConsoleToken)
		console.POST("/:id/serial-token", middleware.RequireVMScope(), handler.GetSerialToken)
	}
}

// registerMonitoringRoutes registers VM monitoring endpoints (requires vm:read).
func registerMonitoringRoutes(vms *gin.RouterGroup, handler *CustomerHandler) {
	monitoring := vms.Group("")
	monitoring.Use(middleware.RequirePermission(PermissionVMRead))
	{
		monitoring.GET("/:id/metrics", middleware.RequireVMScope(), handler.GetMetrics)
		monitoring.GET("/:id/bandwidth", middleware.RequireVMScope(), handler.GetBandwidth)
		monitoring.GET("/:id/network", middleware.RequireVMScope(), handler.GetNetworkHistory)
	}
}

// registerRDNSRoutes registers rDNS management endpoints.
func registerRDNSRoutes(vms *gin.RouterGroup, handler *CustomerHandler) {
	rdns := vms.Group("")
	// Read operations require vm:read
	rdns.GET("/:id/ips", middleware.RequirePermission(PermissionVMRead), middleware.RequireVMScope(), handler.ListVMIPs)
	rdns.GET("/:id/ips/:ipId/rdns", middleware.RequirePermission(PermissionVMRead), middleware.RequireVMScope(), handler.GetRDNS)

	// Write operations require vm:write
	rdns.PUT("/:id/ips/:ipId/rdns",
		middleware.RequirePermission(PermissionVMWrite),
		middleware.RequireVMScope(),
		middleware.RDNSUpdateRateLimit(),
		handler.UpdateRDNS)
	rdns.DELETE("/:id/ips/:ipId/rdns",
		middleware.RequirePermission(PermissionVMWrite),
		middleware.RequireVMScope(),
		handler.DeleteRDNS)
}

// registerISORoutes registers ISO management endpoints (requires vm:write).
func registerISORoutes(vms *gin.RouterGroup, handler *CustomerHandler) {
	iso := vms.Group("")
	iso.Use(middleware.RequirePermission(PermissionVMWrite))
	{
		iso.POST("/:id/iso/upload", middleware.RequireVMScope(), handler.UploadISO)
		iso.GET("/:id/iso", middleware.RequireVMScope(), handler.ListISOs) // Also requires vm:write as it's VM modification context
		iso.DELETE("/:id/iso/:isoId", middleware.RequireVMScope(), handler.DeleteISO)
		iso.POST("/:id/iso/:isoId/attach", middleware.RequireVMScope(), handler.AttachISO)
		iso.POST("/:id/iso/:isoId/detach", middleware.RequireVMScope(), handler.DetachISO)
	}
}

// registerBackupRoutes registers backup management endpoints with permission enforcement.
func registerBackupRoutes(protected *gin.RouterGroup, handler *CustomerHandler) {
	backups := protected.Group("/backups")

	// Read operations require backup:read
	backups.GET("", middleware.RequirePermission(PermissionBackupRead), handler.ListBackups)
	backups.GET("/:id", middleware.RequirePermission(PermissionBackupRead), handler.GetBackup)

	// Write operations require backup:write
	backups.POST("", middleware.RequirePermission(PermissionBackupWrite), handler.CreateBackup)
	backups.DELETE("/:id", middleware.RequirePermission(PermissionBackupWrite), handler.DeleteBackup)
	backups.POST("/:id/restore", middleware.RequirePermission(PermissionBackupWrite), handler.RestoreBackup)
}

// registerSnapshotRoutes registers snapshot management endpoints with permission enforcement.
func registerSnapshotRoutes(protected *gin.RouterGroup, handler *CustomerHandler) {
	snapshots := protected.Group("/snapshots")

	// Read operations require snapshot:read
	snapshots.GET("", middleware.RequirePermission(PermissionSnapshotRead), handler.ListSnapshots)

	// Write operations require snapshot:write
	snapshots.POST("", middleware.RequirePermission(PermissionSnapshotWrite), handler.CreateSnapshot)
	snapshots.DELETE("/:id", middleware.RequirePermission(PermissionSnapshotWrite), handler.DeleteSnapshot)
	snapshots.POST("/:id/restore", middleware.RequirePermission(PermissionSnapshotWrite), handler.RestoreSnapshot)
}

// registerAPIKeyRoutes registers API key management endpoints (JWT-only).
// API keys cannot be used to manage other API keys - this requires browser session.
func registerAPIKeyRoutes(protected *gin.RouterGroup, handler *CustomerHandler) {
	apiKeys := protected.Group("/api-keys")
	{
		apiKeys.GET("", handler.ListAPIKeys)
		apiKeys.POST("", handler.CreateAPIKey)
		apiKeys.POST("/:id/rotate", handler.RotateAPIKey)
		apiKeys.DELETE("/:id", handler.DeleteAPIKey)
	}
}

// registerWebhookRoutes registers webhook management endpoints (JWT-only).
// Webhooks are account-level configuration requiring browser session.
func registerWebhookRoutes(protected *gin.RouterGroup, handler *CustomerHandler) {
	webhooks := protected.Group("/webhooks")
	{
		webhooks.GET("", handler.ListWebhooks)
		webhooks.POST("", handler.CreateWebhook)
		webhooks.GET("/:id", handler.GetWebhook)
		webhooks.PUT("/:id", handler.UpdateWebhook)
		webhooks.DELETE("/:id", handler.DeleteWebhook)
		webhooks.POST("/:id/test", handler.TestWebhook)
		webhooks.GET("/:id/deliveries", handler.ListWebhookDeliveries)
	}
}

// register2FARoutes registers two-factor authentication endpoints (JWT-only).
// 2FA management requires browser session for security.
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

// registerInAppNotificationRoutes registers in-app notification routes.
// SSE stream requires JWT only (no API key). REST endpoints use the protected group.
func registerInAppNotificationRoutes(
	customer *gin.RouterGroup,
	protected *gin.RouterGroup,
	handler *InAppNotificationsHandler,
	authCfg middleware.AuthConfig,
) {
	notifs := protected.Group("/notifications")
	{
		notifs.GET("", handler.ListNotifications)
		notifs.POST("/:id/read", handler.MarkAsRead)
		notifs.POST("/read-all", handler.MarkAllAsRead)
		notifs.GET("/unread-count", handler.GetUnreadCount)
	}

	// SSE stream is JWT-only — separate from the protected group which may use API keys
	sseGroup := customer.Group("/notifications")
	sseGroup.Use(middleware.JWTAuth(authCfg))
	sseGroup.Use(middleware.RequireUserType("customer"))
	{
		sseGroup.GET("/stream", handler.StreamNotifications)
	}
}

// registerBillingRoutes registers customer billing endpoints (JWT-only).
func registerBillingRoutes(group *gin.RouterGroup, handler *CustomerHandler) {
	billingGroup := group.Group("/billing")
	{
		billingGroup.GET("/balance", handler.GetBillingBalance)
		billingGroup.GET("/transactions", handler.ListBillingTransactions)
		billingGroup.GET("/usage", handler.GetBillingUsage)
	}
}
