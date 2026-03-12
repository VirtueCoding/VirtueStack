// Package provisioning provides HTTP handlers for the WHMCS Provisioning API.
// These endpoints are designed for integration with billing systems like WHMCS.
// All endpoints require API key authentication via the X-API-Key header.
package provisioning

import (
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
)

// ProvisioningHandler handles WHMCS provisioning API requests.
// It provides endpoints for VM lifecycle operations that integrate with
// billing systems. All operations are authenticated via API keys.
type ProvisioningHandler struct {
	vmService     *services.VMService
	authService   *services.AuthService
	taskRepo      *repository.TaskRepository
	vmRepo        *repository.VMRepository
	authConfig    middleware.AuthConfig
	encryptionKey string
	logger        *slog.Logger
}

// NewProvisioningHandler creates a new ProvisioningHandler with the given dependencies.
func NewProvisioningHandler(
	vmService *services.VMService,
	authService *services.AuthService,
	taskRepo *repository.TaskRepository,
	vmRepo *repository.VMRepository,
	jwtSecret string,
	issuer string,
	encryptionKey string,
	logger *slog.Logger,
) *ProvisioningHandler {
	return &ProvisioningHandler{
		vmService:     vmService,
		authService:   authService,
		taskRepo:      taskRepo,
		vmRepo:        vmRepo,
		authConfig:    middleware.AuthConfig{JWTSecret: jwtSecret, Issuer: issuer},
		encryptionKey: encryptionKey,
		logger:        logger.With("component", "provisioning-handler"),
	}
}

// TaskResponse represents the response for async task operations.
type TaskResponse struct {
	TaskID string `json:"task_id"`
}

// CreateVMResponse represents the response for VM creation.
type CreateVMResponse struct {
	TaskID string `json:"task_id"`
	VMID   string `json:"vm_id"`
}

// TaskStatusResponse represents the response for task status queries.
type TaskStatusResponse struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Status    models.TaskStatus `json:"status"`
	Progress  int             `json:"progress"`
	Message   string          `json:"message,omitempty"`
	Result    any             `json:"result,omitempty"`
	CreatedAt string          `json:"created_at"`
}

// VMStatusResponse represents the response for VM status queries.
type VMStatusResponse struct {
	Status string `json:"status"`
	NodeID string `json:"node_id,omitempty"`
}

// ResizeRequest represents the request body for VM resize operations.
type ResizeRequest struct {
	VCPU     *int `json:"vcpu,omitempty"`
	MemoryMB *int `json:"memory_mb,omitempty"`
	DiskGB   *int `json:"disk_gb,omitempty"`
}

// PasswordRequest represents the request body for password operations.
type PasswordRequest struct {
	Password string `json:"password" validate:"required,min=8,max=128"`
}

// ProvisioningCreateVMRequest represents the WHMCS provisioning create VM request.
// This is specific to the provisioning API and differs from the customer-facing API.
type ProvisioningCreateVMRequest struct {
	CustomerID     string   `json:"customer_id" validate:"required,uuid"`
	PlanID         string   `json:"plan_id" validate:"required,uuid"`
	TemplateID     string   `json:"template_id" validate:"required,uuid"`
	Hostname       string   `json:"hostname" validate:"required,hostname_rfc1123,max=63"`
	SSHKeys        []string `json:"ssh_keys,omitempty" validate:"max=10,dive,max=4096"`
	WHMCSServiceID int      `json:"whmcs_service_id" validate:"required"`
	LocationID     string   `json:"location_id,omitempty" validate:"omitempty,uuid"`
}