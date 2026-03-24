package customer

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	"google.golang.org/grpc"
)

// consoleTokenEntry stores a console token with its expiry and bound identifiers.
type consoleTokenEntry struct {
	customerID string
	vmID       string
	expiresAt  time.Time
}

// consoleTokenStore is a short-lived, single-use token cache for console access.
// Tokens are stored until first use or expiry.
//
// F-080: Multi-instance limitation — this is an in-memory, per-process store.
// In a horizontally scaled deployment where multiple controller instances sit
// behind a load balancer, a token issued by instance A cannot be validated by
// instance B. Options for multi-instance deployments:
//   - Encode the token as a signed JWT (verifiable without shared state), or
//   - Move the store to a shared Redis cache.
//
// TODO: Replace with JWT-based console tokens or a Redis-backed store before
// deploying behind multiple instances.
type consoleTokenStore struct {
	mu     sync.Mutex
	tokens map[string]consoleTokenEntry
}

func newConsoleTokenStore() *consoleTokenStore {
	return &consoleTokenStore{
		tokens: make(map[string]consoleTokenEntry),
	}
}

// Store inserts a token into the store. Any existing token with the same key is overwritten.
func (s *consoleTokenStore) Store(token, vmID, customerID string, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[token] = consoleTokenEntry{
		customerID: customerID,
		vmID:       vmID,
		expiresAt:  time.Now().Add(ttl),
	}
}

// Validate checks whether token is valid for the given vmID and customerID.
// If valid, the token is immediately invalidated (single-use).
// Returns false if the token is missing, expired, or bound to a different VM/customer.
func (s *consoleTokenStore) Validate(token, vmID, customerID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.tokens[token]
	if !ok {
		return false
	}
	// Always delete: whether valid or not, do not allow reuse.
	delete(s.tokens, token)
	if time.Now().After(entry.expiresAt) {
		return false
	}
	return entry.vmID == vmID && entry.customerID == customerID
}

type CustomerHandler struct {
	vmService       *services.VMService
	backupService   *services.BackupService
	authService     *services.AuthService
	templateService *services.TemplateService
	webhookService  *services.WebhookService
	customerService *services.CustomerService
	vmRepo          *repository.VMRepository
	nodeRepo        *repository.NodeRepository
	backupRepo      *repository.BackupRepository
	templateRepo    *repository.TemplateRepository
	customerRepo    *repository.CustomerRepository
	apiKeyRepo      *repository.CustomerAPIKeyRepository
	auditRepo       *repository.AuditRepository
	bandwidthRepo   *repository.BandwidthRepository
	ipRepo          *repository.IPRepository
	planRepo        *repository.PlanRepository
	rdnsService     *services.RDNSService
	nodeAgent       nodeAgentConnPool
	authConfig      middleware.AuthConfig
	encryptionKey   string
	consoleBaseURL  string
	isoStoragePath  string
	tokenStore      *consoleTokenStore
	logger          *slog.Logger
}

type nodeAgentConnPool interface {
	GetConnection(ctx context.Context, nodeID, address string) (*grpc.ClientConn, error)
}

// CustomerHandlerConfig holds all dependencies required to construct a CustomerHandler.
type CustomerHandlerConfig struct {
	VMService       *services.VMService
	BackupService   *services.BackupService
	AuthService     *services.AuthService
	TemplateService *services.TemplateService
	WebhookService  *services.WebhookService
	CustomerService *services.CustomerService
	VMRepo          *repository.VMRepository
	NodeRepo        *repository.NodeRepository
	BackupRepo      *repository.BackupRepository
	TemplateRepo    *repository.TemplateRepository
	CustomerRepo    *repository.CustomerRepository
	APIKeyRepo      *repository.CustomerAPIKeyRepository
	AuditRepo       *repository.AuditRepository
	BandwidthRepo   *repository.BandwidthRepository
	IPRepo          *repository.IPRepository
	PlanRepo        *repository.PlanRepository
	RDNSService     *services.RDNSService
	NodeAgent       nodeAgentConnPool
	JWTSecret       string
	Issuer          string
	EncryptionKey   string
	ConsoleBaseURL  string
	ISOStoragePath  string
	Logger          *slog.Logger
}

func NewCustomerHandler(cfg CustomerHandlerConfig) *CustomerHandler {
	return &CustomerHandler{
		vmService:       cfg.VMService,
		backupService:   cfg.BackupService,
		authService:     cfg.AuthService,
		templateService: cfg.TemplateService,
		webhookService:  cfg.WebhookService,
		customerService: cfg.CustomerService,
		vmRepo:          cfg.VMRepo,
		nodeRepo:        cfg.NodeRepo,
		backupRepo:      cfg.BackupRepo,
		templateRepo:    cfg.TemplateRepo,
		customerRepo:    cfg.CustomerRepo,
		apiKeyRepo:      cfg.APIKeyRepo,
		auditRepo:       cfg.AuditRepo,
		bandwidthRepo:   cfg.BandwidthRepo,
		ipRepo:          cfg.IPRepo,
		planRepo:        cfg.PlanRepo,
		rdnsService:     cfg.RDNSService,
		nodeAgent:       cfg.NodeAgent,
		authConfig:      middleware.AuthConfig{JWTSecret: cfg.JWTSecret, Issuer: cfg.Issuer},
		encryptionKey:   cfg.EncryptionKey,
		consoleBaseURL:  cfg.ConsoleBaseURL,
		isoStoragePath:  cfg.ISOStoragePath,
		tokenStore:      newConsoleTokenStore(),
		logger:          cfg.Logger.With("component", "customer-handler"),
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
