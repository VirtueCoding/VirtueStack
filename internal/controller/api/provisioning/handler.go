// Package provisioning provides HTTP handlers for the Provisioning API.
// These endpoints are designed for integration with external billing systems (WHMCS, Blesta, etc.).
// All endpoints require API key authentication via the X-API-Key header.
package provisioning

import (
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
)

// ProvisioningHandler handles provisioning API requests from external billing systems.
// It provides endpoints for VM lifecycle operations that integrate with
// billing systems. All operations are authenticated via API keys.
type ProvisioningHandler struct {
	vmService        *services.VMService
	authService      *services.AuthService
	customerService  *services.CustomerService
	bandwidthService *services.BandwidthService
	customerRepo     *repository.CustomerRepository
	taskRepo         *repository.TaskRepository
	vmRepo           *repository.VMRepository
	ipRepo           *repository.IPRepository
	ssoTokenRepo     *repository.SSOTokenRepository
	auditRepo        *repository.AuditRepository
	planService      *services.PlanService
	authConfig       middleware.AuthConfig
	encryptionKey    string
	logger           *slog.Logger
}

// ProvisioningHandlerConfig holds all dependencies required to construct a ProvisioningHandler.
type ProvisioningHandlerConfig struct {
	VMService        *services.VMService
	AuthService      *services.AuthService
	CustomerService  *services.CustomerService
	BandwidthService *services.BandwidthService
	CustomerRepo     *repository.CustomerRepository
	TaskRepo         *repository.TaskRepository
	VMRepo           *repository.VMRepository
	IPRepo           *repository.IPRepository
	SSOTokenRepo     *repository.SSOTokenRepository
	AuditRepo        *repository.AuditRepository
	PlanService      *services.PlanService
	JWTSecret        string
	Issuer           string
	EncryptionKey    string
	Logger           *slog.Logger
}

// NewProvisioningHandler creates a new ProvisioningHandler with the given dependencies.
func NewProvisioningHandler(cfg ProvisioningHandlerConfig) *ProvisioningHandler {
	return &ProvisioningHandler{
		vmService:        cfg.VMService,
		authService:      cfg.AuthService,
		customerService:  cfg.CustomerService,
		bandwidthService: cfg.BandwidthService,
		customerRepo:     cfg.CustomerRepo,
		taskRepo:         cfg.TaskRepo,
		vmRepo:           cfg.VMRepo,
		ipRepo:           cfg.IPRepo,
		ssoTokenRepo:     cfg.SSOTokenRepo,
		auditRepo:        cfg.AuditRepo,
		planService:      cfg.PlanService,
		authConfig:       middleware.AuthConfig{JWTSecret: cfg.JWTSecret, Issuer: cfg.Issuer},
		encryptionKey:    cfg.EncryptionKey,
		logger:           cfg.Logger.With("component", "provisioning-handler"),
	}
}

// TaskResponse represents the response for async task operations.
type TaskResponse struct {
	TaskID string `json:"task_id"`
}

// CreateVMResponse represents the response for VM creation.
type CreateVMResponse struct {
	TaskID         string `json:"task_id"`
	VMID           string `json:"vm_id"`
	StorageBackend string `json:"storage_backend,omitempty"`
	DiskPath       string `json:"disk_path,omitempty"`
}

// TaskStatusResponse represents the response for task status queries.
type TaskStatusResponse struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Status   models.TaskStatus `json:"status"`
	Progress int               `json:"progress"`
	Message  string            `json:"message,omitempty"`
	// Result holds task-type-specific output (e.g. CreateVMResponse for vm.create tasks).
	// It is populated only when Status == TaskStatusCompleted and the task produced output.
	// Callers should type-assert or unmarshal based on the Type field.
	Result    any    `json:"result,omitempty"`
	CreatedAt string `json:"created_at"`
}

// VMStatusResponse represents the response for VM status queries.
type VMStatusResponse struct {
	Status string `json:"status"`
	NodeID string `json:"node_id,omitempty"`
}

// ResizeRequest represents the request body for VM resize operations.
// PlanID is optional - when provided, the VM's plan is updated and resources
// are validated against the new plan. Raw resource values can still be provided
// for backwards compatibility and advanced use cases.
type ResizeRequest struct {
	PlanID   string `json:"plan_id,omitempty" validate:"omitempty,uuid"`
	VCPU     *int   `json:"vcpu,omitempty" validate:"omitempty,gt=0"`
	MemoryMB *int   `json:"memory_mb,omitempty" validate:"omitempty,gt=0"`
	DiskGB   *int   `json:"disk_gb,omitempty" validate:"omitempty,gt=0"`
}

// PasswordRequest represents the request body for password operations.
type PasswordRequest struct {
	Password string `json:"password" validate:"required,min=12,max=128"`
}

// ProvisioningCreateVMRequest represents the provisioning create VM request.
// This is specific to the provisioning API and differs from the customer-facing API.
type ProvisioningCreateVMRequest struct {
	CustomerID        string   `json:"customer_id" validate:"required,uuid"`
	PlanID            string   `json:"plan_id" validate:"required,uuid"`
	TemplateID        string   `json:"template_id" validate:"required,uuid"`
	Hostname          string   `json:"hostname" validate:"required,hostname_rfc1123,max=63"`
	SSHKeys           []string `json:"ssh_keys,omitempty" validate:"max=10,dive,max=4096"`
	ExternalServiceID int      `json:"external_service_id" validate:"required,gt=0"`
	LocationID        string   `json:"location_id,omitempty" validate:"omitempty,uuid"`
}
