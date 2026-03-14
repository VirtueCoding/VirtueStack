// Package controller provides the VirtueStack Controller application.
package controller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/admin"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/customer"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/provisioning"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	"github.com/AbuGosok/VirtueStack/internal/controller/tasks"
	"github.com/AbuGosok/VirtueStack/internal/shared/config"
	apierrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type fallbackTemplateStorage struct{}

func (fallbackTemplateStorage) ImportTemplate(ctx context.Context, name, sourcePath string) (string, string, error) {
	return "", "", fmt.Errorf("template storage backend is not configured")
}

func (fallbackTemplateStorage) DeleteTemplate(ctx context.Context, rbdImage, rbdSnapshot string) error {
	return fmt.Errorf("template storage backend is not configured")
}

func (fallbackTemplateStorage) GetTemplateSize(ctx context.Context, rbdImage, rbdSnapshot string) (int64, error) {
	return 0, fmt.Errorf("template storage backend is not configured")
}

// HTTP server constants.
const (
	ReadTimeout  = 10 * time.Second
	WriteTimeout = 30 * time.Second
	IdleTimeout  = 120 * time.Second
)

// Server represents the VirtueStack Controller HTTP server.
type Server struct {
	config     *config.ControllerConfig
	router     *gin.Engine
	httpServer *http.Server
	dbPool     *pgxpool.Pool
	natsConn   *nats.Conn
	jetstream  nats.JetStreamContext
	taskWorker *tasks.Worker
	logger     *slog.Logger
	nodeClient *NodeClient
	storage    services.TemplateStorage
	// Services
	vmService        *services.VMService
	authService      *services.AuthService
	nodeService      *services.NodeService
	ipamService      *services.IPAMService
	planService      *services.PlanService
	templateService  *services.TemplateService
	customerService  *services.CustomerService
	backupService    *services.BackupService
	migrationService *services.MigrationService
	failoverMonitor  *services.FailoverMonitor
	// API Handlers
	provisioningHandler *provisioning.ProvisioningHandler
	customerHandler     *customer.CustomerHandler
	adminHandler        *admin.AdminHandler
}

// NewServer creates a new Controller server.
func NewServer(cfg *config.ControllerConfig, dbPool *pgxpool.Pool, js nats.JetStreamContext, logger *slog.Logger) (*Server, error) {
	// Set Gin to release mode always
	gin.SetMode(gin.ReleaseMode)

	// Create router
	router := gin.New()

	s := &Server{
		config:    cfg,
		router:    router,
		dbPool:    dbPool,
		jetstream: js,
		logger:    logger.With("component", "controller"),
		storage:   fallbackTemplateStorage{},
	}

	// Setup middlewares
	router.Use(middleware.CorrelationID())
	router.Use(middleware.Recovery(logger))
	router.Use(s.requestLogger())

	// Setup routes
	s.setupRoutes()

	return s, nil
}

// SetTaskWorker sets the task worker for the server.
func (s *Server) SetTaskWorker(worker *tasks.Worker) {
	s.taskWorker = worker
}

// SetNodeClient sets the node gRPC client for the server.
func (s *Server) SetNodeClient(client *NodeClient) {
	s.nodeClient = client
}

// SetNATSConnection sets the NATS connection.
func (s *Server) SetNATSConnection(conn *nats.Conn) {
	s.natsConn = conn
}

// InitializeServices initializes all services and handlers.
// This must be called after SetTaskWorker and SetNodeClient.
func (s *Server) InitializeServices() error {
	// Initialize repositories
	vmRepo := repository.NewVMRepository(s.dbPool)
	nodeRepo := repository.NewNodeRepository(s.dbPool)
	ipRepo := repository.NewIPRepository(s.dbPool)
	planRepo := repository.NewPlanRepository(s.dbPool)
	templateRepo := repository.NewTemplateRepository(s.dbPool)
	customerRepo := repository.NewCustomerRepository(s.dbPool)
	backupRepo := repository.NewBackupRepository(s.dbPool)
	auditRepo := repository.NewAuditRepository(s.dbPool)
	taskRepo := repository.NewTaskRepository(s.dbPool)
	adminRepo := repository.NewAdminRepository(s.dbPool)
	provisioningKeyRepo := repository.NewProvisioningKeyRepository(s.dbPool)
	apiKeyRepo := repository.NewCustomerAPIKeyRepository(s.dbPool)
	webhookRepo := repository.NewWebhookRepository(s.dbPool)
	bandwidthRepo := repository.NewBandwidthRepository(s.dbPool)
	settingsRepo := repository.NewSettingsRepository(s.dbPool)

	// Create task publisher using the worker
	var taskPublisher services.TaskPublisher
	if s.taskWorker != nil {
		taskPublisher = services.NewDefaultTaskPublisher(taskRepo, s.logger)
	}

	var nodeAgentClient services.NodeAgentClient
	var backupNodeAgentClient services.BackupNodeAgentClient
	if s.nodeClient != nil {
		nodeAgentGRPCClient := services.NewNodeAgentGRPCClient(nodeRepo, s.nodeClient, s.logger)
		nodeAgentClient = nodeAgentGRPCClient
		backupNodeAgentClient = services.NewBackupNodeAgentAdapter(nodeAgentGRPCClient, vmRepo)
	}

	// Suppress unused variable warning for provisioningKeyRepo (used in route registration)
	_ = provisioningKeyRepo

	// Initialize services
	s.authService = services.NewAuthService(
		customerRepo,
		adminRepo,
		auditRepo,
		s.config.JWTSecret,
		"virtuestack", // issuer
		s.config.EncryptionKey,
		s.logger,
	)

	s.ipamService = services.NewIPAMService(ipRepo, nodeRepo, s.logger)

	s.vmService = services.NewVMService(
		vmRepo,
		nodeRepo,
		ipRepo,
		planRepo,
		templateRepo,
		taskRepo,
		taskPublisher,
		nodeAgentClient,
		s.ipamService,
		s.config.EncryptionKey,
		s.logger,
	)

	s.nodeService = services.NewNodeServiceWithDefaults(
		nodeRepo,
		vmRepo,
		nodeAgentClient,
		s.config.EncryptionKey,
		s.logger,
	)

	s.planService = services.NewPlanService(planRepo, s.logger)

	s.templateService = services.NewTemplateService(
		templateRepo,
		s.storage,
		s.logger,
	)

	s.customerService = services.NewCustomerService(customerRepo, auditRepo, s.logger)

	s.backupService = services.NewBackupService(
		backupRepo,
		backupRepo, // Same repo handles snapshots
		vmRepo,
		backupNodeAgentClient,
		taskPublisher,
		s.logger,
	)

	s.migrationService = services.NewMigrationService(
		vmRepo,
		nodeRepo,
		taskRepo,
		taskPublisher,
		nodeAgentClient,
		s.logger,
	)

	failoverService := services.NewFailoverService(
		nodeRepo,
		vmRepo,
		nodeAgentClient,
		auditRepo,
		s.config.EncryptionKey,
		s.logger,
	)

	s.failoverMonitor = services.NewFailoverMonitor(
		nodeRepo,
		failoverService,
		s.logger,
		services.DefaultFailoverMonitorConfig(),
	)

	webhookService := services.NewWebhookService(
		webhookRepo,
		taskPublisher,
		s.logger,
		s.config.EncryptionKey,
	)

	// Initialize handlers
	s.provisioningHandler = provisioning.NewProvisioningHandler(
		s.vmService,
		s.authService,
		taskRepo,
		vmRepo,
		s.config.JWTSecret,
		"virtuestack", // issuer
		s.config.EncryptionKey,
		s.logger,
	)

	s.customerHandler = customer.NewCustomerHandler(
		s.vmService,
		s.backupService,
		s.authService,
		s.templateService,
		webhookService,
		s.customerService,
		vmRepo,
		backupRepo,
		templateRepo,
		customerRepo,
		apiKeyRepo,
		auditRepo,
		bandwidthRepo,
		s.nodeClient,
		s.config.JWTSecret,
		"virtuestack",
		s.config.EncryptionKey,
		s.config.ConsoleBaseURL,
		s.logger,
	)

	s.adminHandler = admin.NewAdminHandler(
		s.nodeService,
		s.vmService,
		s.migrationService,
		s.planService,
		s.templateService,
		s.ipamService,
		s.customerService,
		s.backupService,
		s.authService,
		auditRepo,
		ipRepo,
		settingsRepo,
		s.config.JWTSecret,
		"virtuestack", // issuer
		s.logger,
	)

	s.logger.Info("services initialized")

	return nil
}

// GetProvisioningKeyRepo returns the provisioning key repository for route registration.
func (s *Server) GetProvisioningKeyRepo() *repository.ProvisioningKeyRepository {
	return repository.NewProvisioningKeyRepository(s.dbPool)
}

// GetIPAMService returns the IPAM service for task handler dependencies.
func (s *Server) GetIPAMService() tasks.IPAMService {
	return s.ipamService
}

// setupRoutes configures all HTTP routes.
func (s *Server) setupRoutes() {
	// Health check endpoints
	s.router.GET("/health", s.healthHandler)
	s.router.GET("/ready", s.readinessHandler)

	// Metrics endpoint
	s.router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// API v1 routes
	_ = s.router.Group("/api/v1")

	// Provisioning API (WHMCS) - requires API key authentication
	// Note: Handlers are nil until InitializeServices is called
	// Routes will be registered in RegisterAPIRoutes after services are initialized
}

// RegisterAPIRoutes registers all API routes after services are initialized.
// This must be called after InitializeServices.
func (s *Server) RegisterAPIRoutes() {
	if s.provisioningHandler == nil || s.customerHandler == nil || s.adminHandler == nil {
		s.logger.Warn("attempting to register routes without initialized handlers")
		return
	}

	v1 := s.router.Group("/api/v1")

	auditRepo := repository.NewAuditRepository(s.dbPool)

	provisioning.RegisterProvisioningRoutes(v1, s.provisioningHandler, s.GetProvisioningKeyRepo(), auditRepo)

	customer.RegisterCustomerRoutes(v1, s.customerHandler, nil)

	admin.RegisterAdminRoutes(v1, s.adminHandler)

	s.logger.Info("API routes registered")
}

// healthHandler returns a simple liveness check.
func (s *Server) healthHandler(c *gin.Context) {
	respondJSON(c, http.StatusOK, gin.H{"status": "ok"})
}

// readinessHandler checks if the server is ready to serve requests.
func (s *Server) readinessHandler(c *gin.Context) {
	ctx := c.Request.Context()

	// Check database connection
	if err := s.dbPool.Ping(ctx); err != nil {
		respondError(c, &apierrors.APIError{
			Code:       "DATABASE_UNAVAILABLE",
			Message:    "Database connection failed",
			HTTPStatus: http.StatusServiceUnavailable,
		})
		return
	}

	// Check NATS connection status
	natsStatus := "connected"
	nodeCount := 0

	if s.natsConn == nil || s.natsConn.Status() != nats.CONNECTED {
		natsStatus = "disconnected"
	}

	// Get node count from gRPC client
	if s.nodeClient != nil {
		nodeCount = s.nodeClient.ConnectionCount()
	}

	respondJSON(c, http.StatusOK, gin.H{
		"status":     "ready",
		"database":   "connected",
		"nats":       natsStatus,
		"node_count": nodeCount,
	})
}

// requestLogger returns a middleware that logs HTTP requests.
func (s *Server) requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		logger := s.logger.With(
			"method", method,
			"path", path,
			"status", status,
			"latency_ms", latency.Milliseconds(),
			"correlation_id", middleware.GetCorrelationID(c),
		)

		if status >= 500 {
			logger.Error("request completed with error")
		} else if status >= 400 {
			logger.Warn("request completed with client error")
		} else {
			logger.Debug("request completed")
		}
	}
}

// Start starts the HTTP server.
func (s *Server) Start(ctx context.Context) error {
	s.httpServer = &http.Server{
		Addr:         s.config.ListenAddr,
		Handler:      s.router,
		ReadTimeout:  ReadTimeout,
		WriteTimeout: WriteTimeout,
		IdleTimeout:  IdleTimeout,
	}

	s.logger.Info("starting HTTP server", "address", s.config.ListenAddr)

	err := s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("starting HTTP server: %w", err)
	}

	return nil
}

// Stop gracefully stops the HTTP server.
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}

	s.logger.Info("stopping HTTP server")

	// Create shutdown context with deadline
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down HTTP server: %w", err)
	}

	s.logger.Info("HTTP server stopped")
	return nil
}

// StartSchedulers starts background schedulers (e.g., backup scheduler).
// Each scheduler runs in its own goroutine and stops when the context is cancelled.
func (s *Server) StartSchedulers(ctx context.Context) {
	if s.backupService != nil {
		s.logger.Info("starting backup scheduler")
		go s.backupService.StartScheduler(ctx)
	}

	if s.failoverMonitor != nil {
		s.logger.Info("starting failover monitor")
		go s.failoverMonitor.Start(ctx)
	}
}

// respondJSON sends a JSON response with the standard format.
func respondJSON(c *gin.Context, status int, data any) {
	c.JSON(status, models.Response{Data: data})
}

// respondJSONWithMeta sends a JSON response with pagination metadata.
func respondJSONWithMeta(c *gin.Context, status int, data any, meta models.PaginationMeta) {
	c.JSON(status, models.ListResponse{
		Data: data,
		Meta: meta,
	})
}

// respondError sends an error response in the standard format.
func respondError(c *gin.Context, apiErr *apierrors.APIError) {
	correlationID := middleware.GetCorrelationID(c)

	resp := gin.H{
		"error": gin.H{
			"code":           apiErr.Code,
			"message":        apiErr.Message,
			"correlation_id": correlationID,
		},
	}

	// Add validation details if present
	if len(apiErr.Details) > 0 {
		resp["error"].(gin.H)["details"] = apiErr.Details
	}

	c.JSON(apiErr.HTTPStatus, resp)
}

// ParsePagination is a convenience wrapper for models.ParsePagination.
func ParsePagination(c *gin.Context) models.PaginationParams {
	return models.ParsePagination(c)
}
