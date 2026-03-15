package customer

import (
	"context"
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	"google.golang.org/grpc"
)

type CustomerHandler struct {
	vmService       *services.VMService
	backupService   *services.BackupService
	authService     *services.AuthService
	templateService *services.TemplateService
	webhookService  *services.WebhookService
	customerService *services.CustomerService
	vmRepo          *repository.VMRepository
	backupRepo      *repository.BackupRepository
	templateRepo    *repository.TemplateRepository
	customerRepo    *repository.CustomerRepository
	apiKeyRepo      *repository.CustomerAPIKeyRepository
	auditRepo       *repository.AuditRepository
	bandwidthRepo   *repository.BandwidthRepository
	ipRepo          *repository.IPRepository
	rdnsService     *services.RDNSService
	nodeAgent       nodeAgentConnPool
	authConfig      middleware.AuthConfig
	encryptionKey   string
	consoleBaseURL  string
	logger          *slog.Logger
}

type nodeAgentConnPool interface {
	GetConnection(ctx context.Context, nodeID, address string) (*grpc.ClientConn, error)
}

func NewCustomerHandler(
	vmService *services.VMService,
	backupService *services.BackupService,
	authService *services.AuthService,
	templateService *services.TemplateService,
	webhookService *services.WebhookService,
	customerService *services.CustomerService,
	vmRepo *repository.VMRepository,
	backupRepo *repository.BackupRepository,
	templateRepo *repository.TemplateRepository,
	customerRepo *repository.CustomerRepository,
	apiKeyRepo *repository.CustomerAPIKeyRepository,
	auditRepo *repository.AuditRepository,
	bandwidthRepo *repository.BandwidthRepository,
	ipRepo *repository.IPRepository,
	rdnsService *services.RDNSService,
	nodeAgent nodeAgentConnPool,
	jwtSecret string,
	issuer string,
	encryptionKey string,
	consoleBaseURL string,
	logger *slog.Logger,
) *CustomerHandler {
	return &CustomerHandler{
		vmService:       vmService,
		backupService:   backupService,
		authService:     authService,
		templateService: templateService,
		webhookService:  webhookService,
		customerService: customerService,
		vmRepo:          vmRepo,
		backupRepo:      backupRepo,
		templateRepo:    templateRepo,
		customerRepo:    customerRepo,
		apiKeyRepo:      apiKeyRepo,
		auditRepo:       auditRepo,
		bandwidthRepo:   bandwidthRepo,
		ipRepo:          ipRepo,
		rdnsService:     rdnsService,
		nodeAgent:       nodeAgent,
		authConfig:      middleware.AuthConfig{JWTSecret: jwtSecret, Issuer: issuer},
		encryptionKey:   encryptionKey,
		consoleBaseURL:  consoleBaseURL,
		logger:          logger.With("component", "customer-handler"),
	}
}

type TaskResponse struct {
	TaskID string `json:"task_id"`
}

type ConsoleTokenResponse struct {
	Token     string `json:"token"`
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
}

type BandwidthResponse struct {
	UsedBytes   int64  `json:"used_bytes"`
	LimitBytes  int64  `json:"limit_bytes"`
	ResetAt     string `json:"reset_at"`
	PercentUsed int    `json:"percent_used"`
}
