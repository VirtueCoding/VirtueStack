// Package admin provides HTTP handlers for the Admin API.
// These endpoints are designed for administrative operations.
// All endpoints require JWT authentication with role=admin and mandatory 2FA.
package admin

import (
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
)

// AdminHandler handles admin-facing API requests.
// It provides endpoints for full system management including nodes, VMs,
// plans, templates, IP pools, customers, audit logs, settings, and backups.
// All operations are authenticated via JWT with role=admin and require 2FA.
type AdminHandler struct {
	nodeService     *services.NodeService
	vmService       *services.VMService
	planService     *services.PlanService
	templateService *services.TemplateService
	ipamService     *services.IPAMService
	customerService *services.CustomerService
	backupService   *services.BackupService
	auditRepo       *repository.AuditRepository
	ipRepo          *repository.IPRepository
	authConfig      middleware.AuthConfig
	logger          *slog.Logger
}

// NewAdminHandler creates a new AdminHandler with the given dependencies.
func NewAdminHandler(
	nodeService *services.NodeService,
	vmService *services.VMService,
	planService *services.PlanService,
	templateService *services.TemplateService,
	ipamService *services.IPAMService,
	customerService *services.CustomerService,
	backupService *services.BackupService,
	auditRepo *repository.AuditRepository,
	ipRepo *repository.IPRepository,
	jwtSecret string,
	issuer string,
	logger *slog.Logger,
) *AdminHandler {
	return &AdminHandler{
		nodeService:     nodeService,
		vmService:       vmService,
		planService:     planService,
		templateService: templateService,
		ipamService:     ipamService,
		customerService: customerService,
		backupService:   backupService,
		auditRepo:       auditRepo,
		ipRepo:          ipRepo,
		authConfig:      middleware.AuthConfig{JWTSecret: jwtSecret, Issuer: issuer},
		logger:          logger.With("component", "admin-handler"),
	}
}

// TaskResponse represents the response for async task operations.
type TaskResponse struct {
	TaskID string `json:"task_id"`
}