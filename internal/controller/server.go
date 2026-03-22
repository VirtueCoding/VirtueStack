// Package controller provides the VirtueStack Controller application.
package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/admin"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/customer"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/provisioning"
	controllermetrics "github.com/AbuGosok/VirtueStack/internal/controller/metrics"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	"github.com/AbuGosok/VirtueStack/internal/controller/tasks"
	"github.com/AbuGosok/VirtueStack/internal/shared/config"
	apierrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type fallbackTemplateStorage struct{}

func (fallbackTemplateStorage) ImportTemplate(ctx context.Context, name, sourcePath string) (string, string, error) {
	return "", "", fmt.Errorf("template storage backend is not configured")
}

func (fallbackTemplateStorage) DeleteTemplate(ctx context.Context, templateRef, snapshotRef string) error {
	return fmt.Errorf("template storage backend is not configured")
}

func (fallbackTemplateStorage) GetTemplateSize(ctx context.Context, templateRef, snapshotRef string) (int64, error) {
	return 0, fmt.Errorf("template storage backend is not configured")
}

func (fallbackTemplateStorage) GetStorageType() string {
	return "fallback"
}

// HTTP server constants.
const (
	ReadTimeout           = 10 * time.Second
	WriteTimeout          = 30 * time.Second
	IdleTimeout           = 120 * time.Second
	defaultISOStoragePath = "/var/lib/virtuestack/iso"
)

// Server represents the VirtueStack Controller HTTP server.
type Server struct {
	config     *config.ControllerConfig
	router     *gin.Engine
	httpServer *http.Server
	dbPool     *pgxpool.Pool
	powerDNSDB *sql.DB // MySQL connection to PowerDNS database
	natsConn   *nats.Conn
	jetstream  nats.JetStreamContext
	taskWorker *tasks.Worker
	logger     *slog.Logger
	nodeClient *NodeClient
	storage    services.TemplateStorage
	// Services
	vmService                 *services.VMService
	authService               *services.AuthService
	nodeService               *services.NodeService
	ipamService               *services.IPAMService
	planService               *services.PlanService
	templateService           *services.TemplateService
	customerService           *services.CustomerService
	backupService             *services.BackupService
	migrationService          *services.MigrationService
	failoverMonitor           *services.FailoverMonitor
	heartbeatChecker          *services.HeartbeatChecker
	rdnsService               *services.RDNSService
	bandwidthRepo             *repository.BandwidthRepository
	adminBackupScheduleService *services.AdminBackupScheduleService
	// Repositories needed for route registration
	customerAPIKeyRepo *repository.CustomerAPIKeyRepository
	// API Handlers
	provisioningHandler *provisioning.ProvisioningHandler
	customerHandler     *customer.CustomerHandler
	adminHandler        *admin.AdminHandler
	notifyHandler       *customer.NotificationsHandler
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
	router.Use(middleware.Metrics())
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
	apiKeyRepo := repository.NewCustomerAPIKeyRepository(s.dbPool)
	webhookRepo := repository.NewWebhookRepository(s.dbPool)
	bandwidthRepo := repository.NewBandwidthRepository(s.dbPool)
	settingsRepo := repository.NewSettingsRepository(s.dbPool)

	s.bandwidthRepo = bandwidthRepo
	s.customerAPIKeyRepo = apiKeyRepo

	// Create task publisher using the worker
	var taskPublisher services.TaskPublisher
	if s.taskWorker != nil {
		taskPublisher = services.NewDefaultTaskPublisher(taskRepo, s.logger)
	}

	var nodeAgentClient services.NodeAgentClient
	var backupNodeAgentClient services.BackupNodeAgentClient
	if s.nodeClient != nil {
		nodeAgentGRPCClient := services.NewNodeAgentGRPCClient(nodeRepo, vmRepo, s.nodeClient, &services.CephConfig{
			Monitors:   s.config.CephMonitors,
			User:       s.config.CephUser,
			SecretUUID: s.config.CephSecretUUID,
		}, s.logger)
		nodeAgentClient = nodeAgentGRPCClient
		backupNodeAgentClient = services.NewBackupNodeAgentAdapter(nodeAgentGRPCClient, vmRepo)
	}

	// Initialize services
	s.authService = services.NewAuthService(
		customerRepo,
		adminRepo,
		auditRepo,
		s.config.JWTSecret.Value(),
		"virtuestack", // issuer
		s.config.EncryptionKey.Value(),
		s.logger,
	)

	s.ipamService = services.NewIPAMService(ipRepo, nodeRepo, s.logger)

	s.vmService = services.NewVMService(services.VMServiceConfig{
		VMRepo:        vmRepo,
		NodeRepo:      nodeRepo,
		IPRepo:        ipRepo,
		PlanRepo:      planRepo,
		TemplateRepo:  templateRepo,
		TaskRepo:      taskRepo,
		TaskPublisher: taskPublisher,
		NodeAgent:     nodeAgentClient,
		IPAMService:   s.ipamService,
		EncryptionKey: s.config.EncryptionKey.Value(),
		Logger:        s.logger,
	})

	s.nodeService = services.NewNodeServiceWithDefaults(
		nodeRepo,
		vmRepo,
		nodeAgentClient,
		s.config.EncryptionKey.Value(),
		s.logger,
	)

	s.planService = services.NewPlanService(planRepo, s.logger)

	s.templateService = services.NewTemplateService(
		templateRepo,
		s.storage,
		s.logger,
	)

	s.customerService = services.NewCustomerService(customerRepo, auditRepo, s.logger)

	s.backupService = services.NewBackupService(services.BackupServiceConfig{
		BackupRepo:    backupRepo,
		SnapshotRepo:  backupRepo, // Same repo handles snapshots
		VMRepo:        vmRepo,
		NodeAgent:     backupNodeAgentClient,
		TaskPublisher: taskPublisher,
		Logger:        s.logger,
	})

	s.migrationService = services.NewMigrationService(
		vmRepo,
		nodeRepo,
		taskRepo,
		taskPublisher,
		nodeAgentClient,
		s.logger,
	)

	failoverRepo := repository.NewFailoverRepository(s.dbPool)
	failoverService := services.NewFailoverService(
		nodeRepo,
		vmRepo,
		nodeAgentClient,
		auditRepo,
		failoverRepo,
		s.config.EncryptionKey.Value(),
		s.logger,
	)

	s.failoverMonitor = services.NewFailoverMonitor(
		nodeRepo,
		failoverService,
		s.logger,
		services.DefaultFailoverMonitorConfig(),
	)

	heartbeatChecker := services.NewHeartbeatChecker(nodeRepo, s.logger, services.DefaultHeartbeatCheckerConfig())
	s.heartbeatChecker = heartbeatChecker

	webhookService := services.NewWebhookService(
		webhookRepo,
		taskPublisher,
		s.logger,
		s.config.EncryptionKey.Value(),
	)

	// Initialize PowerDNS rDNS service if MySQL connection is configured
	if s.config.PowerDNS.MySQLURL != "" {
		var err error
		s.powerDNSDB, err = sql.Open("mysql", s.config.PowerDNS.MySQLURL)
		if err != nil {
			s.logger.Warn("failed to connect to PowerDNS MySQL database", "error", err)
		} else {
			s.rdnsService = services.NewRDNSService(s.powerDNSDB, s.logger)
			s.logger.Info("PowerDNS rDNS service initialized")
		}
	}

	// Initialize handlers
	s.provisioningHandler = provisioning.NewProvisioningHandler(provisioning.ProvisioningHandlerConfig{
		VMService:     s.vmService,
		AuthService:   s.authService,
		TaskRepo:      taskRepo,
		VMRepo:        vmRepo,
		IPRepo:        ipRepo,
		PlanService:   s.planService,
		JWTSecret:     s.config.JWTSecret.Value(),
		Issuer:        "virtuestack",
		EncryptionKey: s.config.EncryptionKey.Value(),
		Logger:        s.logger,
	})

	isoStoragePath := s.config.FileStorage.ISOStoragePath
	if isoStoragePath == "" {
		isoStoragePath = defaultISOStoragePath
	}

	s.customerHandler = customer.NewCustomerHandler(customer.CustomerHandlerConfig{
		VMService:       s.vmService,
		BackupService:   s.backupService,
		AuthService:     s.authService,
		TemplateService: s.templateService,
		WebhookService:  webhookService,
		CustomerService: s.customerService,
		VMRepo:          vmRepo,
		NodeRepo:        nodeRepo,
		BackupRepo:      backupRepo,
		TemplateRepo:    templateRepo,
		CustomerRepo:    customerRepo,
		APIKeyRepo:      apiKeyRepo,
		AuditRepo:       auditRepo,
		BandwidthRepo:   bandwidthRepo,
		IPRepo:          ipRepo,
		PlanRepo:        planRepo,
		RDNSService:     s.rdnsService,
		NodeAgent:       s.nodeClient,
		JWTSecret:       s.config.JWTSecret.Value(),
		Issuer:          "virtuestack",
		EncryptionKey:   s.config.EncryptionKey.Value(),
		ConsoleBaseURL:  s.config.ConsoleBaseURL,
		ISOStoragePath:  isoStoragePath,
		Logger:          s.logger,
	})

	s.adminHandler = admin.NewAdminHandler(admin.AdminHandlerConfig{
		NodeService:             s.nodeService,
		VMService:               s.vmService,
		MigrationService:        s.migrationService,
		PlanService:             s.planService,
		TemplateService:         s.templateService,
		IPAMService:             s.ipamService,
		CustomerService:         s.customerService,
		BackupService:           s.backupService,
		AuthService:             s.authService,
		AuditRepo:               auditRepo,
		IPRepo:                  ipRepo,
		SettingsRepo:            settingsRepo,
		FailoverRepo:            failoverRepo,
		AdminBackupScheduleRepo: repository.NewAdminBackupScheduleRepository(s.dbPool),
		AdminRepo:               adminRepo,
		RDNSService:             s.rdnsService,
		JWTSecret:               s.config.JWTSecret.Value(),
		Issuer:                  "virtuestack",
		Logger:                  s.logger,
	})

	notificationPreferenceRepo := repository.NewNotificationPreferenceRepository(s.dbPool)
	notificationEventRepo := repository.NewNotificationEventRepository(s.dbPool)

	notifyService := services.NewNotificationService(
		nil,
		nil,
		notificationPreferenceRepo,
		customerRepo,
		services.NotificationConfig{
			EmailEnabled:    s.config.SMTP.Host != "",
			TelegramEnabled: s.config.Telegram.BotToken != "",
		},
		s.logger,
	)

	s.notifyHandler = customer.NewNotificationsHandler(
		notificationPreferenceRepo,
		notificationEventRepo,
		notifyService,
	)

	// Initialize admin backup schedule service for mass backup campaigns
	s.adminBackupScheduleService = services.NewAdminBackupScheduleService(services.AdminBackupScheduleServiceConfig{
		AdminBackupScheduleRepo: repository.NewAdminBackupScheduleRepository(s.dbPool),
		VMRepo:                  vmRepo,
		BackupRepo:              backupRepo,
		TaskPublisher:           taskPublisher,
		Logger:                  s.logger,
	})

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
	// CORS configuration - use configured origins or defaults
	allowOrigins := s.config.CORSOrigins
	if len(allowOrigins) == 0 {
		allowOrigins = []string{"https://virtuestack.com", "https://app.virtuestack.com"}
	}
	corsConfig := cors.Config{
		AllowOrigins:     allowOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID", "X-CSRF-Token"},
		ExposeHeaders:    []string{"Content-Length", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	s.router.Use(cors.New(corsConfig))

	// Health check endpoints
	s.router.GET("/health", s.healthHandler)
	s.router.GET("/ready", s.readinessHandler)

	// Metrics endpoint
	s.router.GET("/metrics", gin.WrapH(promhttp.Handler()))

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

	customer.RegisterCustomerRoutes(v1, s.customerHandler, s.notifyHandler, s.customerAPIKeyRepo)

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

		switch {
		case status >= 500:
			logger.Error("request completed with error")
		case status >= 400:
			logger.Warn("request completed with client error")
		default:
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

	if s.adminBackupScheduleService != nil {
		s.logger.Info("starting admin backup schedule scheduler")
		go s.adminBackupScheduleService.StartScheduler(ctx)
	}

	if s.failoverMonitor != nil {
		s.logger.Info("starting failover monitor")
		go s.failoverMonitor.Start(ctx)
	}

	if s.heartbeatChecker != nil {
		s.logger.Info("starting heartbeat checker")
		go s.heartbeatChecker.Start(ctx)
	}

	s.startMetricsCollector(ctx)

	if s.bandwidthRepo != nil && s.nodeClient != nil {
		go s.startBandwidthCollector(ctx)
	}

	go s.startSessionCleanup(ctx)
}

func (s *Server) startMetricsCollector(ctx context.Context) {
	if s.dbPool == nil {
		return
	}

	vmRepo := repository.NewVMRepository(s.dbPool)
	nodeRepo := repository.NewNodeRepository(s.dbPool)

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.collectControllerMetrics(ctx, vmRepo, nodeRepo)
			}
		}
	}()
}

func (s *Server) collectControllerMetrics(ctx context.Context, vmRepo *repository.VMRepository, nodeRepo *repository.NodeRepository) {
	vmStatuses := []string{models.VMStatusRunning, models.VMStatusStopped, models.VMStatusProvisioning, models.VMStatusSuspended, models.VMStatusMigrating, models.VMStatusError}
	for _, status := range vmStatuses {
		_, total, err := vmRepo.List(ctx, models.VMListFilter{
			Status:           util.StringPtr(status),
			PaginationParams: models.PaginationParams{Page: 1, PerPage: 1},
		})
		count := 0
		if err == nil {
			count = total
		}
		controllermetrics.VMsTotal.WithLabelValues(status).Set(float64(count))
	}

	nodeStatuses := []string{models.NodeStatusOnline, models.NodeStatusOffline, models.NodeStatusDraining, models.NodeStatusDegraded, models.NodeStatusFailed}
	for _, status := range nodeStatuses {
		_, total, err := nodeRepo.List(ctx, models.NodeListFilter{
			Status:           &status,
			PaginationParams: models.PaginationParams{Page: 1, PerPage: 1},
		})
		count := 0
		if err == nil {
			count = total
		}
		controllermetrics.NodesTotal.WithLabelValues(status).Set(float64(count))
	}

	onlineNodes, _, err := nodeRepo.List(ctx, models.NodeListFilter{
		Status:           util.StringPtr(models.NodeStatusOnline),
		PaginationParams: models.PaginationParams{Page: 1, PerPage: models.MaxPerPage},
	})
	if err != nil {
		return
	}

	now := time.Now()
	for _, node := range onlineNodes {
		var age float64
		if node.LastHeartbeatAt != nil {
			age = now.Sub(*node.LastHeartbeatAt).Seconds()
		} else {
			age = 9999
		}
		controllermetrics.NodeHeartbeatAge.WithLabelValues(node.ID).Set(age)
	}
}

func (s *Server) startBandwidthCollector(ctx context.Context) {
	s.logger.Info("starting bandwidth collector")

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("bandwidth collector stopped")
			return
		case <-ticker.C:
			s.collectBandwidth(ctx)
		}
	}
}

func (s *Server) startSessionCleanup(ctx context.Context) {
	if s.dbPool == nil {
		return
	}

	s.logger.Info("starting session cleanup scheduler")

	customerRepo := repository.NewCustomerRepository(s.dbPool)
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("session cleanup scheduler stopped")
			return
		case <-ticker.C:
			s.cleanupExpiredSessions(ctx, customerRepo)
		}
	}
}

func (s *Server) cleanupExpiredSessions(ctx context.Context, customerRepo *repository.CustomerRepository) {
	if err := customerRepo.DeleteExpiredSessions(ctx); err != nil {
		s.logger.Warn("failed to delete expired sessions", "error", err)
		return
	}
	s.logger.Debug("expired sessions cleaned up")
}

func (s *Server) collectBandwidth(ctx context.Context) {
	vmRepo := repository.NewVMRepository(s.dbPool)

	vms, _, err := vmRepo.List(ctx, models.VMListFilter{
		Status:           util.StringPtr(models.VMStatusRunning),
		PaginationParams: models.PaginationParams{Page: 1, PerPage: models.MaxPerPage},
	})
	if err != nil {
		s.logger.Warn("failed to list running VMs for bandwidth collection", "error", err)
		return
	}

	for _, vm := range vms {
		if vm.NodeID == nil {
			continue
		}

		node, err := repository.NewNodeRepository(s.dbPool).GetByID(ctx, *vm.NodeID)
		if err != nil {
			continue
		}

		conn, err := s.nodeClient.GetConnection(ctx, *vm.NodeID, node.GRPCAddress)
		if err != nil {
			s.logger.Debug("failed to get connection for bandwidth", "vm_id", vm.ID, "node_id", *vm.NodeID, "error", err)
			continue
		}

		pbClient := nodeagentpb.NewNodeAgentServiceClient(conn)
		bwResp, err := pbClient.GetBandwidthUsage(ctx, &nodeagentpb.VMIdentifier{VmId: vm.ID})
		if err != nil {
			s.logger.Debug("failed to get bandwidth for VM", "vm_id", vm.ID, "error", err)
			continue
		}

		if bwResp.RxBytes > 0 || bwResp.TxBytes > 0 {
			controllermetrics.BandwidthBytesTotal.WithLabelValues(vm.ID, "rx").Set(float64(bwResp.RxBytes))
			controllermetrics.BandwidthBytesTotal.WithLabelValues(vm.ID, "tx").Set(float64(bwResp.TxBytes))
		}
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

	errData := gin.H{
		"code":           apiErr.Code,
		"message":        apiErr.Message,
		"correlation_id": correlationID,
	}

	// Add validation details if present
	if len(apiErr.Details) > 0 {
		errData["details"] = apiErr.Details
	}

	resp := gin.H{
		"error": errData,
	}

	c.JSON(apiErr.HTTPStatus, resp)
}

// ParsePagination is a convenience wrapper for models.ParsePagination.
func ParsePagination(c *gin.Context) models.PaginationParams {
	return models.ParsePagination(c)
}
