// Package controller provides the VirtueStack Controller application.
package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/admin"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/customer"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/provisioning"
	"github.com/AbuGosok/VirtueStack/internal/controller/metrics"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	controllerredis "github.com/AbuGosok/VirtueStack/internal/controller/redis"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	"github.com/AbuGosok/VirtueStack/internal/controller/tasks"
	"github.com/AbuGosok/VirtueStack/internal/shared/config"
	apierrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
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
	vmService                  *services.VMService
	authService                *services.AuthService
	nodeService                *services.NodeService
	ipamService                *services.IPAMService
	planService                *services.PlanService
	templateService            *services.TemplateService
	customerService            *services.CustomerService
	backupService              *services.BackupService
	migrationService           *services.MigrationService
	failoverMonitor            *services.FailoverMonitor
	heartbeatChecker           *services.HeartbeatChecker
	rdnsService                *services.RDNSService
	bandwidthRepo              *repository.BandwidthRepository
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

	var redisClient *controllerredis.Client
	if cfg.Redis.URL != "" {
		client, err := controllerredis.NewClient(context.Background(), cfg.Redis.URL)
		if err != nil {
			return nil, fmt.Errorf("initializing redis rate limit backend: %w", err)
		}
		redisClient = client
	}

	if err := middleware.ValidateDistributedRateLimitConfiguration(strings.EqualFold(cfg.Environment, "production"), redisClient != nil); err != nil {
		return nil, err
	}
	middleware.ConfigureDistributedRateLimitBackend(redisClient)

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

	metrics.RegisterDBPoolMetrics(dbPool)

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

	// Swagger UI is restricted to authenticated admins only.
	s.router.GET("/swagger/*any",
		middleware.JWTAuth(s.adminHandler.AuthConfig()),
		middleware.RequireRole("admin", "super_admin"),
		ginSwagger.WrapHandler(swaggerFiles.Handler))

	s.logger.Info("API routes registered")
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

	// Close the PowerDNS MySQL connection if it was opened.
	if s.powerDNSDB != nil {
		if err := s.powerDNSDB.Close(); err != nil {
			s.logger.Warn("failed to close PowerDNS MySQL connection", "error", err)
		} else {
			s.logger.Info("PowerDNS MySQL connection closed")
		}
	}

	s.logger.Info("HTTP server stopped")
	return nil
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
