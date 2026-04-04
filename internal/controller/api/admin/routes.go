package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
)

// RegisterAdminRoutes registers all admin API routes.
// These routes are for administrative operations with elevated privileges.
//
// Base path: /api/v1/admin
// Authentication: JWT Bearer token with role=admin (mandatory 2FA)
//
// All endpoints enforce admin role verification.
//
// Endpoints:
//
//	Nodes:
//	  GET    /nodes              - List all nodes
//	  POST   /nodes              - Register new node
//	  GET    /nodes/:id          - Get node details
//	  PUT    /nodes/:id          - Update node
//	  DELETE /nodes/:id          - Delete node
//	  POST   /nodes/:id/drain    - Set draining status
//	  POST   /nodes/:id/failover - Set failed status
//
//	VMs:
//	  GET    /vms                - List all VMs (all customers)
//	  POST   /vms                - Manual VM create
//	  GET    /vms/:id            - Get VM details
//	  PUT    /vms/:id            - Update VM
//	  DELETE /vms/:id            - Delete VM
//	  POST   /vms/:id/migrate    - Migrate to another node
//
//	Plans:
//	  GET    /plans              - List all plans
//	  POST   /plans              - Create plan
//	  GET    /plans/:id/usage    - Get VM count for plan
//	  PUT    /plans/:id          - Update plan
//	  DELETE /plans/:id          - Delete plan
//
//	Templates:
//	  GET    /templates          - List all templates
//	  GET    /templates/:id      - Get template details
//	  POST   /templates          - Create template
//	  PUT    /templates/:id      - Update template
//	  DELETE /templates/:id      - Delete template
//	  POST   /templates/:id/import - Import OS image
//	  POST   /templates/build-from-iso - Build template from ISO
//
//	IP Sets:
//	  GET    /ip-sets              - List IP sets
//	  POST   /ip-sets              - Create IP set
//	  GET    /ip-sets/:id          - Get IP set details
//	  PUT    /ip-sets/:id          - Update IP set
//	  DELETE /ip-sets/:id          - Delete IP set
//	  GET    /ip-sets/:id/available - List available IPs
//
//	Customers:
//	  GET    /customers            - List customers
//	  POST   /customers            - Create customer
//	  GET    /customers/:id        - Get customer details
//	  PUT    /customers/:id        - Update customer
//	  DELETE /customers/:id        - Delete customer
//	  GET    /customers/:id/audit-logs - Customer audit trail
//
//	Audit:
//	  GET    /audit-logs           - List audit logs with filters
//
//	Settings:
//	  GET    /settings             - Get all settings
//	  PUT    /settings/:key        - Update setting
//
//	Backups:
//	  GET    /backups              - List all backups (all customers)
//	  POST   /backups/:id/restore  - Restore backup (admin override)
//
//	Backup Schedules:
//	  POST   /backup-schedules     - Create backup schedule
//	  GET    /backup-schedules     - List backup schedules (optional ?vm_id= filter)
//	  GET    /backup-schedules/:id - Get backup schedule by ID
//	  PUT    /backup-schedules/:id - Update backup schedule
//	  DELETE /backup-schedules/:id - Delete backup schedule
//
//	Admin Backup Schedules:
//	  POST   /admin-backup-schedules     - Create admin backup schedule
//	  GET    /admin-backup-schedules     - List admin backup schedules
//	  GET    /admin-backup-schedules/:id - Get admin backup schedule by ID
//	  PUT    /admin-backup-schedules/:id - Update admin backup schedule
//	  DELETE /admin-backup-schedules/:id - Delete admin backup schedule
//	  POST   /admin-backup-schedules/:id/run - Trigger immediate execution
//
//	Permission Management:
//	  GET    /auth/permissions           - List all available permissions
//	  PUT    /auth/permissions/:admin_id - Update admin permissions (super_admin only)
func RegisterAdminRoutes(router *gin.RouterGroup, handler *AdminHandler, inAppNotifHandler *AdminInAppNotificationsHandler) {
	admin := router.Group("/admin")

	auth := admin.Group("/auth")
	{
		auth.GET("/csrf", middleware.CSRF(middleware.DefaultCSRFConfig()), handler.CSRF)
		auth.POST("/login", middleware.LoginRateLimit(), handler.Login)
		auth.POST("/verify-2fa", middleware.LoginRateLimit(), handler.Verify2FA)
		auth.POST("/refresh", middleware.LoginRateLimit(), handler.RefreshToken)
	}

	registerAdminLogoutRoutes(admin, handler)

	// Protected auth endpoints - require valid JWT but not CSRF (read-only identity)
	protectedAuth := admin.Group("/auth")
	protectedAuth.Use(middleware.JWTAuth(handler.authConfig))
	protectedAuth.Use(middleware.RequireRole("admin", "super_admin"))
	{
		protectedAuth.GET("/me", handler.Me)
		protectedAuth.GET("/permissions", handler.ListPermissions)
		protectedAuth.POST("/reauth", handler.Reauth)
	}

	// Permission management - super_admin only
	permissionMgmt := admin.Group("/auth/permissions")
	permissionMgmt.Use(middleware.JWTAuth(handler.authConfig))
	permissionMgmt.Use(middleware.RequireRole("super_admin"))
	permissionMgmt.Use(middleware.AdminLoader(handler.authService.GetAdminByID))
	permissionMgmt.Use(middleware.CSRF(middleware.DefaultCSRFConfig()))
	{
		permissionMgmt.PUT("/:admin_id", handler.UpdateAdminPermissions)
	}

	// Admin management - super_admin only
	adminMgmt := admin.Group("/admins")
	adminMgmt.Use(middleware.JWTAuth(handler.authConfig))
	adminMgmt.Use(middleware.RequireRole("super_admin"))
	adminMgmt.Use(middleware.AdminLoader(handler.authService.GetAdminByID))
	adminMgmt.Use(middleware.CSRF(middleware.DefaultCSRFConfig()))
	{
		adminMgmt.GET("", handler.ListAdmins)
	}

	// adminAuditLogger bridges middleware.AuditEntry to the admin handler's auditRepo.
	// This enables the Audit middleware (F-157) to persist entries without modifying the
	// RegisterAdminRoutes function signature.
	adminAuditLogger := func(ctx context.Context, entry *middleware.AuditEntry) error {
		if handler.auditRepo == nil {
			return nil
		}
		auditLog := &models.AuditLog{
			ActorID:       &entry.ActorID,
			ActorType:     entry.ActorType,
			ActorIP:       &entry.ActorIP,
			Action:        entry.Action,
			ResourceType:  entry.ResourceType,
			ResourceID:    &entry.ResourceID,
			CorrelationID: &entry.CorrelationID,
			Success:       entry.Success,
		}
		if entry.ErrorMessage != "" {
			auditLog.ErrorMessage = &entry.ErrorMessage
		}
		if entry.Changes != nil {
			data, err := json.Marshal(entry.Changes)
			if err == nil {
				auditLog.Changes = data
			}
		}
		return handler.auditRepo.Append(ctx, auditLog)
	}

	protected := admin.Group("")
	protected.Use(middleware.JWTAuth(handler.authConfig))
	protected.Use(middleware.RequireRole("admin", "super_admin"))
	protected.Use(middleware.AdminLoader(handler.authService.GetAdminByID))
	protected.Use(middleware.CSRF(middleware.DefaultCSRFConfig()))
	protected.Use(middleware.AdminRateLimit())
	// Audit middleware covers POST, PUT, PATCH, and DELETE (F-157)
	protected.Use(middleware.Audit(adminAuditLogger))
	{
		// Node management
		nodes := protected.Group("/nodes")
		{
			nodes.GET("", middleware.RequireAdminPermission(models.PermissionNodesRead), handler.ListNodes)
			nodes.POST("", middleware.RequireAdminPermission(models.PermissionNodesWrite), handler.RegisterNode)
			nodes.GET("/:id", middleware.RequireAdminPermission(models.PermissionNodesRead), handler.GetNode)
			nodes.PUT("/:id", middleware.RequireAdminPermission(models.PermissionNodesWrite), handler.UpdateNode)
			nodes.DELETE("/:id", middleware.RequireAdminPermission(models.PermissionNodesDelete), RequireReAuth(handler.authConfig), handler.DeleteNode)
			nodes.POST("/:id/drain", middleware.RequireAdminPermission(models.PermissionNodesWrite), handler.DrainNode)
			nodes.POST("/:id/failover", middleware.RequireAdminPermission(models.PermissionNodesWrite), handler.FailoverNode)
			nodes.POST("/:id/undrain", middleware.RequireAdminPermission(models.PermissionNodesWrite), handler.UndrainNode)
		}

		// Failover requests
		failoverRequests := protected.Group("/failover-requests")
		{
			failoverRequests.GET("", middleware.RequireAdminPermission(models.PermissionNodesRead), handler.ListFailoverRequests)
			failoverRequests.GET("/:id", middleware.RequireAdminPermission(models.PermissionNodesRead), handler.GetFailoverRequest)
		}

		// VM management
		vms := protected.Group("/vms")
		{
			vms.GET("", middleware.RequireAdminPermission(models.PermissionVMsRead), handler.ListVMs)
			vms.POST("", middleware.RequireAdminPermission(models.PermissionVMsWrite), handler.CreateVM)
			vms.GET("/:id", middleware.RequireAdminPermission(models.PermissionVMsRead), handler.GetVM)
			vms.PUT("/:id", middleware.RequireAdminPermission(models.PermissionVMsWrite), handler.UpdateVM)
			vms.DELETE("/:id", middleware.RequireAdminPermission(models.PermissionVMsDelete), RequireReAuth(handler.authConfig), handler.DeleteVM)
			vms.POST("/:id/migrate", middleware.RequireAdminPermission(models.PermissionVMsWrite), handler.MigrateVM)
			vms.GET("/:id/ips", middleware.RequireAdminPermission(models.PermissionVMsRead), handler.GetVMIPs)
			vms.GET("/:id/ips/:ipId/rdns", middleware.RequireAdminPermission(models.PermissionVMsRead), handler.GetIPRDNS)
			vms.PUT("/:id/ips/:ipId/rdns", middleware.RequireAdminPermission(models.PermissionVMsWrite), handler.UpdateIPRDNS)
			vms.DELETE("/:id/ips/:ipId/rdns", middleware.RequireAdminPermission(models.PermissionVMsWrite), handler.DeleteIPRDNS)
		}

		// Plan management
		plans := protected.Group("/plans")
		{
			plans.GET("", middleware.RequireAdminPermission(models.PermissionPlansRead), handler.ListPlans)
			plans.POST("", middleware.RequireAdminPermission(models.PermissionPlansWrite), handler.CreatePlan)
			plans.GET("/:id/usage", middleware.RequireAdminPermission(models.PermissionPlansRead), handler.GetPlanUsage)
			plans.PUT("/:id", middleware.RequireAdminPermission(models.PermissionPlansWrite), handler.UpdatePlan)
			plans.DELETE("/:id", middleware.RequireAdminPermission(models.PermissionPlansDelete), handler.DeletePlan)
		}

		// Template management
		templates := protected.Group("/templates")
		{
			templates.GET("", middleware.RequireAdminPermission(models.PermissionTemplatesRead), handler.ListTemplates)
			templates.GET("/:id", middleware.RequireAdminPermission(models.PermissionTemplatesRead), handler.GetTemplate) // F-135
			templates.POST("", middleware.RequireAdminPermission(models.PermissionTemplatesWrite), handler.CreateTemplate)
			templates.PUT("/:id", middleware.RequireAdminPermission(models.PermissionTemplatesWrite), handler.UpdateTemplate)
			templates.DELETE("/:id", middleware.RequireAdminPermission(models.PermissionTemplatesWrite), handler.DeleteTemplate)
			templates.POST("/:id/import", middleware.RequireAdminPermission(models.PermissionTemplatesWrite), handler.ImportTemplate)
			templates.POST("/build-from-iso", middleware.RequireAdminPermission(models.PermissionTemplatesWrite), handler.BuildTemplateFromISO)
			templates.POST("/:id/distribute", middleware.RequireAdminPermission(models.PermissionTemplatesWrite), handler.DistributeTemplate)
			templates.GET("/:id/cache-status", middleware.RequireAdminPermission(models.PermissionTemplatesRead), handler.GetTemplateCacheStatus)
		}

		// IP Set management
		ipSets := protected.Group("/ip-sets")
		{
			ipSets.GET("", middleware.RequireAdminPermission(models.PermissionIPSetsRead), handler.ListIPSets)
			ipSets.POST("", middleware.RequireAdminPermission(models.PermissionIPSetsWrite), handler.CreateIPSet)
			ipSets.GET("/:id", middleware.RequireAdminPermission(models.PermissionIPSetsRead), handler.GetIPSet)
			ipSets.PUT("/:id", middleware.RequireAdminPermission(models.PermissionIPSetsWrite), handler.UpdateIPSet)
			ipSets.DELETE("/:id", middleware.RequireAdminPermission(models.PermissionIPSetsDelete), handler.DeleteIPSet)
			ipSets.GET("/:id/available", middleware.RequireAdminPermission(models.PermissionIPSetsRead), handler.ListAvailableIPs)
		}

		// Customer management
		customers := protected.Group("/customers")
		{
			customers.GET("", middleware.RequireAdminPermission(models.PermissionCustomersRead), handler.ListCustomers)
			customers.POST("", middleware.RequireAdminPermission(models.PermissionCustomersWrite), handler.CreateCustomer)
			customers.GET("/:id", middleware.RequireAdminPermission(models.PermissionCustomersRead), handler.GetCustomer)
			customers.PUT("/:id", middleware.RequireAdminPermission(models.PermissionCustomersWrite), handler.UpdateCustomer)
			customers.DELETE("/:id", middleware.RequireAdminPermission(models.PermissionCustomersDelete), RequireReAuth(handler.authConfig), handler.DeleteCustomer)
			customers.GET("/:id/audit-logs", middleware.RequireAdminPermission(models.PermissionCustomersRead), handler.GetCustomerAuditLogs)
		}

		// Audit logs - requires dedicated audit log permission (F-076)
		protected.GET("/audit-logs", middleware.RequireAdminPermission(models.PermissionAuditLogsRead), handler.ListAuditLogs)

		// Settings
		settings := protected.Group("/settings")
		{
			settings.GET("", middleware.RequireAdminPermission(models.PermissionSettingsRead), handler.GetSettings)
			settings.PUT("/:key", middleware.RequireAdminPermission(models.PermissionSettingsWrite), handler.UpdateSetting)
		}

		// Backup management
		backups := protected.Group("/backups")
		{
			backups.GET("", middleware.RequireAdminPermission(models.PermissionBackupsRead), handler.ListBackups)
			backups.POST("/:id/restore", middleware.RequireAdminPermission(models.PermissionBackupsWrite), handler.RestoreBackup)
		}

		// Backup schedule management
		backupSchedules := protected.Group("/backup-schedules")
		{
			backupSchedules.POST("", middleware.RequireAdminPermission(models.PermissionBackupsWrite), handler.CreateBackupSchedule)
			backupSchedules.GET("", middleware.RequireAdminPermission(models.PermissionBackupsRead), handler.ListBackupSchedules)
			backupSchedules.GET("/:id", middleware.RequireAdminPermission(models.PermissionBackupsRead), handler.GetBackupSchedule)
			backupSchedules.PUT("/:id", middleware.RequireAdminPermission(models.PermissionBackupsWrite), handler.UpdateBackupSchedule)
			backupSchedules.DELETE("/:id", middleware.RequireAdminPermission(models.PermissionBackupsWrite), handler.DeleteBackupSchedule)
		}

		// Admin backup schedule management (mass backup campaigns)
		adminBackupSchedules := protected.Group("/admin-backup-schedules")
		{
			adminBackupSchedules.POST("", middleware.RequireAdminPermission(models.PermissionBackupsWrite), handler.CreateAdminBackupSchedule)
			adminBackupSchedules.GET("", middleware.RequireAdminPermission(models.PermissionBackupsRead), handler.ListAdminBackupSchedules)
			adminBackupSchedules.GET("/:id", middleware.RequireAdminPermission(models.PermissionBackupsRead), handler.GetAdminBackupSchedule)
			adminBackupSchedules.PUT("/:id", middleware.RequireAdminPermission(models.PermissionBackupsWrite), handler.UpdateAdminBackupSchedule)
			adminBackupSchedules.DELETE("/:id", middleware.RequireAdminPermission(models.PermissionBackupsWrite), handler.DeleteAdminBackupSchedule)
			adminBackupSchedules.POST("/:id/run", middleware.RequireAdminPermission(models.PermissionBackupsWrite), handler.RunAdminBackupSchedule)
		}

		// Storage backend management
		storageBackends := protected.Group("/storage-backends")
		{
			storageBackends.GET("", middleware.RequireAdminPermission(models.PermissionStorageBackendsRead), handler.ListStorageBackends)
			storageBackends.POST("", middleware.RequireAdminPermission(models.PermissionStorageBackendsWrite), handler.CreateStorageBackend)
			storageBackends.GET("/:id", middleware.RequireAdminPermission(models.PermissionStorageBackendsRead), handler.GetStorageBackend)
			storageBackends.PUT("/:id", middleware.RequireAdminPermission(models.PermissionStorageBackendsWrite), handler.UpdateStorageBackend)
			storageBackends.DELETE("/:id", middleware.RequireAdminPermission(models.PermissionStorageBackendsDelete), RequireReAuth(handler.authConfig), handler.DeleteStorageBackend)
			storageBackends.GET("/:id/nodes", middleware.RequireAdminPermission(models.PermissionStorageBackendsRead), handler.GetStorageBackendNodes)
			storageBackends.POST("/:id/nodes", middleware.RequireAdminPermission(models.PermissionStorageBackendsWrite), handler.AssignStorageBackendNodes)
			storageBackends.DELETE("/:id/nodes/:nodeId", middleware.RequireAdminPermission(models.PermissionStorageBackendsWrite), handler.RemoveStorageBackendNode)
			storageBackends.GET("/:id/health", middleware.RequireAdminPermission(models.PermissionStorageBackendsRead), handler.GetStorageBackendHealth)
			storageBackends.POST("/:id/refresh", middleware.RequireAdminPermission(models.PermissionStorageBackendsWrite), handler.RefreshStorageBackendHealth)
		}

		// Provisioning key management (API key lifecycle for WHMCS integration)
		provisioningKeys := protected.Group("/provisioning-keys")
		{
			provisioningKeys.GET("", middleware.RequireAdminPermission(models.PermissionSettingsRead), handler.ListProvisioningKeys)
			provisioningKeys.POST("", middleware.RequireAdminPermission(models.PermissionSettingsWrite), handler.CreateProvisioningKey)
			provisioningKeys.GET("/:id", middleware.RequireAdminPermission(models.PermissionSettingsRead), handler.GetProvisioningKey)
			provisioningKeys.PUT("/:id", middleware.RequireAdminPermission(models.PermissionSettingsWrite), handler.UpdateProvisioningKey)
			provisioningKeys.DELETE("/:id", middleware.RequireAdminPermission(models.PermissionSettingsWrite), RequireReAuth(handler.authConfig), handler.RevokeProvisioningKey)
		}

		// System webhook management
		systemWebhooks := protected.Group("/system-webhooks")
		{
			systemWebhooks.GET("", middleware.RequireAdminPermission(models.PermissionSettingsRead), handler.ListSystemWebhooks)
			systemWebhooks.POST("", middleware.RequireAdminPermission(models.PermissionSettingsWrite), handler.CreateSystemWebhook)
			systemWebhooks.PUT("/:id", middleware.RequireAdminPermission(models.PermissionSettingsWrite), handler.UpdateSystemWebhook)
			systemWebhooks.DELETE("/:id", middleware.RequireAdminPermission(models.PermissionSettingsWrite), RequireReAuth(handler.authConfig), handler.DeleteSystemWebhook)
		}

		// Pre-action webhook management
		preActionWebhooks := protected.Group("/pre-action-webhooks")
		{
			preActionWebhooks.GET("", middleware.RequireAdminPermission(models.PermissionSettingsRead), handler.ListPreActionWebhooks)
			preActionWebhooks.POST("", middleware.RequireAdminPermission(models.PermissionSettingsWrite), handler.CreatePreActionWebhook)
			preActionWebhooks.PUT("/:id", middleware.RequireAdminPermission(models.PermissionSettingsWrite), handler.UpdatePreActionWebhook)
			preActionWebhooks.DELETE("/:id", middleware.RequireAdminPermission(models.PermissionSettingsWrite), RequireReAuth(handler.authConfig), handler.DeletePreActionWebhook)
		}

		// Billing management
		billingGroup := protected.Group("/billing")
		{
			billingGroup.GET("/transactions",
				middleware.RequireAdminPermission(models.PermissionBillingRead),
				handler.ListBillingTransactions)
			billingGroup.GET("/balance",
				middleware.RequireAdminPermission(models.PermissionBillingRead),
				handler.GetCustomerBalance)
			billingGroup.POST("/credit",
				middleware.RequireAdminPermission(models.PermissionBillingWrite),
				handler.AdminCreditAdjustment)
			billingGroup.GET("/payments",
				middleware.RequireAdminPermission(models.PermissionBillingRead),
				handler.ListPayments)
			billingGroup.POST("/refund/:paymentId",
				middleware.RequireAdminPermission(models.PermissionBillingWrite),
				handler.RefundPayment)
			billingGroup.GET("/config",
				middleware.RequireAdminPermission(models.PermissionBillingRead),
				handler.GetBillingConfig)
		}

		// Exchange rate management
		exchangeRates := protected.Group("/exchange-rates")
		{
			exchangeRates.GET("",
				middleware.RequireAdminPermission(models.PermissionBillingRead),
				handler.ListExchangeRates)
			exchangeRates.PUT("/:currency",
				middleware.RequireAdminPermission(models.PermissionBillingWrite),
				handler.UpdateExchangeRate)
		}

		// Invoices (billing)
		invoices := protected.Group("/invoices")
		{
			invoices.GET("",
				middleware.RequireAdminPermission(models.PermissionBillingRead),
				handler.ListInvoices)
			invoices.GET("/:id",
				middleware.RequireAdminPermission(models.PermissionBillingRead),
				handler.GetInvoice)
			invoices.GET("/:id/pdf",
				middleware.RequireAdminPermission(models.PermissionBillingRead),
				handler.DownloadInvoicePDF)
			invoices.POST("/:id/void",
				middleware.RequireAdminPermission(models.PermissionBillingWrite),
				handler.VoidInvoice)
		}
	}

	if inAppNotifHandler != nil {
		registerAdminInAppNotificationRoutes(admin, protected, inAppNotifHandler, handler.authConfig)
	}
}

// RequireReAuth returns a Gin middleware that enforces re-authentication for destructive operations.
// Admins must have re-authenticated within the last 15 minutes via a valid re-auth JWT token.
// The token must be supplied in the X-Reauth-Token header and is validated for signature, expiry,
// and purpose="reauth" claim. This protects destructive operations (DELETE nodes, VMs, customers,
// storage backends) from being performed with a stolen admin JWT alone.
func RequireReAuth(authConfig middleware.AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		reAuthToken := c.GetHeader("X-Reauth-Token")
		if reAuthToken == "" {
			middleware.RespondWithError(c, http.StatusForbidden, "REAUTH_REQUIRED",
				"re-authentication is required for this destructive operation; "+
					"obtain X-Reauth-Token from POST /api/v1/admin/auth/reauth")
			return
		}

		// Validate the re-auth token: must be a signed JWT with purpose="reauth".
		claims, err := middleware.ValidateReauthToken(authConfig, reAuthToken)
		if err != nil {
			middleware.RespondWithError(c, http.StatusForbidden, "REAUTH_REQUIRED",
				"re-authentication is required for this destructive operation; "+
					"obtain X-Reauth-Token from POST /api/v1/admin/auth/reauth")
			return
		}

		// Verify the re-auth token belongs to the current admin user.
		currentUserID := middleware.GetUserID(c)
		if claims.UserID != currentUserID {
			middleware.RespondWithError(c, http.StatusForbidden, "REAUTH_REQUIRED",
				"re-authentication is required for this destructive operation; "+
					"obtain X-Reauth-Token from POST /api/v1/admin/auth/reauth")
			return
		}

		c.Next()
	}
}

// registerAdminInAppNotificationRoutes registers in-app notification routes for admins.
func registerAdminInAppNotificationRoutes(
	adminGroup *gin.RouterGroup,
	protected *gin.RouterGroup,
	handler *AdminInAppNotificationsHandler,
	authCfg middleware.AuthConfig,
) {
	notifs := protected.Group("/notifications")
	{
		notifs.GET("", handler.ListNotifications)
		notifs.POST("/:id/read", handler.MarkAsRead)
		notifs.POST("/read-all", handler.MarkAllAsRead)
		notifs.GET("/unread-count", handler.GetUnreadCount)
	}

	sseGroup := adminGroup.Group("/notifications")
	sseGroup.Use(middleware.JWTAuth(authCfg))
	sseGroup.Use(middleware.RequireRole("admin", "super_admin"))
	{
		sseGroup.GET("/stream", handler.StreamNotifications)
	}
}
