package admin

import (
	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
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
//	  PUT    /plans/:id          - Update plan
//	  DELETE /plans/:id          - Delete plan
//
//	Templates:
//	  GET    /templates          - List all templates
//	  POST   /templates          - Create template
//	  PUT    /templates/:id      - Update template
//	  DELETE /templates/:id      - Delete template
//	  POST   /templates/:id/import - Import OS image
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
func RegisterAdminRoutes(router *gin.RouterGroup, handler *AdminHandler) {
	admin := router.Group("/admin")

	auth := admin.Group("/auth")
	{
		auth.POST("/login", handler.Login)
		auth.POST("/verify-2fa", handler.Verify2FA)
		auth.POST("/refresh", handler.RefreshToken)
	}

	protected := admin.Group("")
	protected.Use(middleware.JWTAuth(handler.authConfig))
	protected.Use(middleware.RequireRole("admin", "super_admin"))
	{
		// Node management
		nodes := protected.Group("/nodes")
		{
			nodes.GET("", handler.ListNodes)
			nodes.POST("", handler.RegisterNode)
			nodes.GET("/:id", handler.GetNode)
			nodes.PUT("/:id", handler.UpdateNode)
			nodes.DELETE("/:id", handler.DeleteNode)
			nodes.POST("/:id/drain", handler.DrainNode)
			nodes.POST("/:id/failover", handler.FailoverNode)
		}

		// VM management
		vms := protected.Group("/vms")
		{
			vms.GET("", handler.ListVMs)
			vms.POST("", handler.CreateVM)
			vms.GET("/:id", handler.GetVM)
			vms.PUT("/:id", handler.UpdateVM)
			vms.DELETE("/:id", handler.DeleteVM)
			vms.POST("/:id/migrate", handler.MigrateVM)
		}

		// Plan management
		plans := protected.Group("/plans")
		{
			plans.GET("", handler.ListPlans)
			plans.POST("", handler.CreatePlan)
			plans.PUT("/:id", handler.UpdatePlan)
			plans.DELETE("/:id", handler.DeletePlan)
		}

		// Template management
		templates := protected.Group("/templates")
		{
			templates.GET("", handler.ListTemplates)
			templates.POST("", handler.CreateTemplate)
			templates.PUT("/:id", handler.UpdateTemplate)
			templates.DELETE("/:id", handler.DeleteTemplate)
			templates.POST("/:id/import", handler.ImportTemplate)
		}

		// IP Set management
		ipSets := protected.Group("/ip-sets")
		{
			ipSets.GET("", handler.ListIPSets)
			ipSets.POST("", handler.CreateIPSet)
			ipSets.GET("/:id", handler.GetIPSet)
			ipSets.PUT("/:id", handler.UpdateIPSet)
			ipSets.DELETE("/:id", handler.DeleteIPSet)
			ipSets.GET("/:id/available", handler.ListAvailableIPs)
		}

		// Customer management
		customers := protected.Group("/customers")
		{
			customers.GET("", handler.ListCustomers)
			customers.GET("/:id", handler.GetCustomer)
			customers.PUT("/:id", handler.UpdateCustomer)
			customers.DELETE("/:id", handler.DeleteCustomer)
			customers.GET("/:id/audit-logs", handler.GetCustomerAuditLogs)
		}

		// Audit logs
		protected.GET("/audit-logs", handler.ListAuditLogs)

		// Settings
		settings := protected.Group("/settings")
		{
			settings.GET("", handler.GetSettings)
			settings.PUT("/:key", handler.UpdateSetting)
		}

		// Backup management
		backups := protected.Group("/backups")
		{
			backups.GET("", handler.ListBackups)
			backups.POST("/:id/restore", handler.RestoreBackup)
		}

		// Backup schedule management
		backupSchedules := protected.Group("/backup-schedules")
		{
			backupSchedules.POST("", handler.CreateBackupSchedule)
			backupSchedules.GET("", handler.ListBackupSchedules)
			backupSchedules.GET("/:id", handler.GetBackupSchedule)
			backupSchedules.PUT("/:id", handler.UpdateBackupSchedule)
			backupSchedules.DELETE("/:id", handler.DeleteBackupSchedule)
		}
	}
}
