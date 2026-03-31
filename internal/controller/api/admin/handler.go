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

// AdminHandlerConfig holds all dependencies required to construct an AdminHandler.
type AdminHandlerConfig struct {
	NodeService             *services.NodeService
	VMService               *services.VMService
	MigrationService        *services.MigrationService
	PlanService             *services.PlanService
	TemplateService         *services.TemplateService
	IPAMService             *services.IPAMService
	CustomerService         *services.CustomerService
	BackupService           *services.BackupService
	AuthService             *services.AuthService
	AuditRepo               *repository.AuditRepository
	IPRepo                  *repository.IPRepository
	SettingsRepo            *repository.SettingsRepository
	FailoverRepo            *repository.FailoverRepository
	AdminBackupScheduleRepo *repository.AdminBackupScheduleRepository
	AdminRepo               *repository.AdminRepository
	StorageBackendRepo      *repository.StorageBackendRepository
	NodeStorageRepo         *repository.NodeStorageRepository
	NodeRepo                *repository.NodeRepository
	VMRepo                  *repository.VMRepository
	ProvisioningKeyRepo     *repository.ProvisioningKeyRepository
	SystemWebhookRepo       *repository.SystemWebhookRepository
	PreActionWebhookRepo    *repository.PreActionWebhookRepository
	CustomerRepo            *repository.CustomerRepository
	RDNSService             *services.RDNSService
	JWTSecret               string
	Issuer                  string
	Logger                  *slog.Logger
}

// AdminHandler handles admin-facing API requests.
// It provides endpoints for full system management including nodes, VMs,
// plans, templates, IP pools, customers, audit logs, settings, and backups.
// All operations are authenticated via JWT with role=admin and require 2FA.
type AdminHandler struct {
	nodeService             *services.NodeService
	vmService               *services.VMService
	migrationService        *services.MigrationService
	planService             *services.PlanService
	templateService         *services.TemplateService
	ipamService             *services.IPAMService
	customerService         *services.CustomerService
	backupService           *services.BackupService
	authService             *services.AuthService
	auditRepo               *repository.AuditRepository
	ipRepo                  *repository.IPRepository
	settingsRepo            *repository.SettingsRepository
	failoverRepo            *repository.FailoverRepository
	adminBackupScheduleRepo *repository.AdminBackupScheduleRepository
	adminRepo               *repository.AdminRepository
	storageBackendRepo      *repository.StorageBackendRepository
	nodeStorageRepo         *repository.NodeStorageRepository
	nodeRepo                *repository.NodeRepository
	vmRepo                  *repository.VMRepository
	provisioningKeyRepo     *repository.ProvisioningKeyRepository
	systemWebhookRepo       *repository.SystemWebhookRepository
	preActionWebhookRepo    *repository.PreActionWebhookRepository
	customerRepo            *repository.CustomerRepository
	rdnsService             *services.RDNSService
	authConfig              middleware.AuthConfig
	logger                  *slog.Logger
}

// AuthConfig returns the admin handler authentication configuration.
func (h *AdminHandler) AuthConfig() middleware.AuthConfig {
	return h.authConfig
}

// NewAdminHandler creates a new AdminHandler with the given dependencies.
func NewAdminHandler(cfg AdminHandlerConfig) *AdminHandler {
	return &AdminHandler{
		nodeService:             cfg.NodeService,
		vmService:               cfg.VMService,
		migrationService:        cfg.MigrationService,
		planService:             cfg.PlanService,
		templateService:         cfg.TemplateService,
		ipamService:             cfg.IPAMService,
		customerService:         cfg.CustomerService,
		backupService:           cfg.BackupService,
		authService:             cfg.AuthService,
		auditRepo:               cfg.AuditRepo,
		ipRepo:                  cfg.IPRepo,
		settingsRepo:            cfg.SettingsRepo,
		failoverRepo:            cfg.FailoverRepo,
		adminBackupScheduleRepo: cfg.AdminBackupScheduleRepo,
		adminRepo:               cfg.AdminRepo,
		storageBackendRepo:      cfg.StorageBackendRepo,
		nodeStorageRepo:         cfg.NodeStorageRepo,
		nodeRepo:                cfg.NodeRepo,
		vmRepo:                  cfg.VMRepo,
		provisioningKeyRepo:     cfg.ProvisioningKeyRepo,
		systemWebhookRepo:       cfg.SystemWebhookRepo,
		preActionWebhookRepo:    cfg.PreActionWebhookRepo,
		customerRepo:            cfg.CustomerRepo,
		rdnsService:             cfg.RDNSService,
		authConfig:              middleware.AuthConfig{JWTSecret: cfg.JWTSecret, Issuer: cfg.Issuer},
		logger:                  cfg.Logger.With("component", "admin-handler"),
	}
}

// TaskResponse represents the response for async task operations.
type TaskResponse struct {
	TaskID string `json:"task_id"`
}
