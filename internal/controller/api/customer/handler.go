// Package customer provides HTTP handlers for the Customer API.
// These endpoints are designed for customer self-service operations.
// All endpoints require JWT authentication and enforce customer isolation.
package customer

import (
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
)

// CustomerHandler handles customer-facing API requests.
// It provides endpoints for VM management, authentication, backups, and other
// self-service operations. All operations are authenticated via JWT and
// enforce customer isolation (users can only access their own resources).
type CustomerHandler struct {
	vmService       *services.VMService
	backupService   *services.BackupService
	authService     *services.AuthService
	templateService *services.TemplateService
	webhookService  *services.WebhookService
	vmRepo          *repository.VMRepository
	backupRepo      *repository.BackupRepository
	templateRepo    *repository.TemplateRepository
	customerRepo    *repository.CustomerRepository
	apiKeyRepo      *repository.CustomerAPIKeyRepository
	auditRepo       *repository.AuditRepository
	bandwidthRepo   *repository.BandwidthRepository
	authConfig      middleware.AuthConfig
	encryptionKey   string
	logger          *slog.Logger
}

// NewCustomerHandler creates a new CustomerHandler with the given dependencies.
func NewCustomerHandler(
	vmService *services.VMService,
	backupService *services.BackupService,
	authService *services.AuthService,
	templateService *services.TemplateService,
	webhookService *services.WebhookService,
	vmRepo *repository.VMRepository,
	backupRepo *repository.BackupRepository,
	templateRepo *repository.TemplateRepository,
	customerRepo *repository.CustomerRepository,
	apiKeyRepo *repository.CustomerAPIKeyRepository,
	auditRepo *repository.AuditRepository,
	bandwidthRepo *repository.BandwidthRepository,
	jwtSecret string,
	issuer string,
	encryptionKey string,
	logger *slog.Logger,
) *CustomerHandler {
	return &CustomerHandler{
		vmService:       vmService,
		backupService:   backupService,
		authService:     authService,
		templateService: templateService,
		webhookService:  webhookService,
		vmRepo:          vmRepo,
		backupRepo:      backupRepo,
		templateRepo:    templateRepo,
		customerRepo:    customerRepo,
		apiKeyRepo:      apiKeyRepo,
		auditRepo:       auditRepo,
		bandwidthRepo:   bandwidthRepo,
		authConfig:      middleware.AuthConfig{JWTSecret: jwtSecret, Issuer: issuer},
		encryptionKey:   encryptionKey,
		logger:          logger.With("component", "customer-handler"),
	}
}

// TaskResponse represents the response for async task operations.
type TaskResponse struct {
	TaskID string `json:"task_id"`
}

// ConsoleTokenResponse represents the response for console token requests.
type ConsoleTokenResponse struct {
	Token     string `json:"token"`
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
}

// BandwidthResponse represents bandwidth usage data.
type BandwidthResponse struct {
	UsedBytes     int64  `json:"used_bytes"`
	LimitBytes    int64  `json:"limit_bytes"`
	ResetAt       string `json:"reset_at"`
	PercentUsed   int    `json:"percent_used"`
}