// Package controller provides the VirtueStack Controller application.
package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/admin"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/customer"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/provisioning"
	"github.com/AbuGosok/VirtueStack/internal/controller/api/webhooks"
	"github.com/AbuGosok/VirtueStack/internal/controller/metrics"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	paypalPayments "github.com/AbuGosok/VirtueStack/internal/controller/payments/paypal"
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
	config        *config.ControllerConfig
	router        *gin.Engine
	httpServer    *http.Server
	metricsServer *http.Server
	metricsLn     net.Listener
	metricsDone   chan struct{}
	dbPool        *pgxpool.Pool
	powerDNSDB    *sql.DB // MySQL connection to PowerDNS database
	natsConn      *nats.Conn
	jetstream     nats.JetStreamContext
	taskWorker    *tasks.Worker
	logger        *slog.Logger
	nodeClient    *NodeClient
	storage       services.TemplateStorage
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
	billingScheduler           *services.BillingScheduler
	invoiceService             *services.BillingInvoiceService
	// Repositories needed for route registration
	customerAPIKeyRepo *repository.CustomerAPIKeyRepository
	// API Handlers
	provisioningHandler       *provisioning.ProvisioningHandler
	customerHandler           *customer.CustomerHandler
	adminHandler              *admin.AdminHandler
	notifyHandler             *customer.NotificationsHandler
	customerInAppNotifHandler *customer.InAppNotificationsHandler
	adminInAppNotifHandler    *admin.AdminInAppNotificationsHandler
	sseHub                    *services.SSEHub
	inAppNotifService         *services.InAppNotificationService
	stripeWebhookHandler      *webhooks.StripeWebhookHandler
	paypalProvider            *paypalPayments.Provider
	paypalWebhookHandler      *webhooks.PayPalWebhookHandler
	cryptoWebhookHandler      *webhooks.CryptoWebhookHandler
	readinessDBPing           func(context.Context) error
	readinessNATSStatus       func() nats.Status
	serveHTTPFunc             func(*http.Server, net.Listener) error
	shutdownHTTPFunc          func(*http.Server, context.Context) error
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
	s.readinessDBPing = dbPool.Ping
	s.readinessNATSStatus = func() nats.Status {
		if s.natsConn == nil {
			return nats.CLOSED
		}
		return s.natsConn.Status()
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

	customer.RegisterCustomerRoutes(
		v1,
		s.customerHandler,
		s.notifyHandler,
		s.customerInAppNotifHandler,
		s.customerAPIKeyRepo,
		s.config.AllowSelfRegistration,
		customer.BillingRoutesConfig{
			NativeBillingEnabled: s.config.Billing.Providers.Native.Enabled,
			OAuthGoogleEnabled:   s.config.OAuth.Google.Enabled,
			OAuthGitHubEnabled:   s.config.OAuth.GitHub.Enabled,
		},
	)

	admin.RegisterAdminRoutes(v1, s.adminHandler, s.adminInAppNotifHandler)

	// Stripe webhook endpoint (unauthenticated — signature verified by handler)
	if s.stripeWebhookHandler != nil {
		v1.POST("/webhooks/stripe", s.stripeWebhookHandler.Handle)
	}

	// PayPal webhook endpoint (unauthenticated — signature verified via PayPal API)
	if s.paypalWebhookHandler != nil {
		v1.POST("/webhooks/paypal", s.paypalWebhookHandler.Handle)
	}

	// Crypto webhook endpoint (unauthenticated — HMAC signature verified by provider)
	if s.cryptoWebhookHandler != nil {
		v1.POST("/webhooks/crypto", s.cryptoWebhookHandler.HandleWebhook)
	}

	// Swagger UI is restricted to authenticated admins only.
	s.router.GET("/swagger/*any",
		middleware.JWTAuth(s.adminHandler.AuthConfig()),
		middleware.RequireRole("admin", "super_admin"),
		ginSwagger.WrapHandler(swaggerFiles.Handler))

	s.logBillingConfig()
	s.logger.Info("API routes registered")
}

// logBillingConfig logs a summary of the billing, payment, and OAuth configuration.
func (s *Server) logBillingConfig() {
	cfg := s.config
	primary := cfg.PrimaryBillingProvider()
	if primary == "" {
		primary = "none"
	}

	s.logger.Info("billing config",
		"whmcs_enabled", cfg.Billing.Providers.WHMCS.Enabled,
		"native_enabled", cfg.Billing.Providers.Native.Enabled,
		"blesta_enabled", cfg.Billing.Providers.Blesta.Enabled,
		"primary", primary,
	)

	var gateways []string
	if cfg.Stripe.SecretKey != "" {
		gateways = append(gateways, "stripe")
	}
	if cfg.PayPal.ClientID != "" {
		gateways = append(gateways, "paypal")
	}
	if cfg.Crypto.Provider != "" && cfg.Crypto.Provider != "disabled" {
		gateways = append(gateways, "crypto/"+cfg.Crypto.Provider)
	}
	if len(gateways) == 0 {
		gateways = []string{"none"}
	}
	s.logger.Info("payment gateways", "configured", gateways)

	s.logger.Info("oauth config",
		"google_enabled", cfg.OAuth.Google.Enabled,
		"github_enabled", cfg.OAuth.GitHub.Enabled,
	)
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

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("listening on HTTP address %s: %w", s.config.ListenAddr, err)
	}

	if err := s.startMetricsHTTPServer(ctx); err != nil {
		if closeErr := listener.Close(); closeErr != nil {
			s.logger.Warn("failed to close HTTP listener after metrics setup failure", "error", closeErr)
		}
		return err
	}

	s.logger.Info("starting HTTP server", "address", s.config.ListenAddr)

	err = s.serveHTTP(listener)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		if closeErr := listener.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			s.logger.Warn("failed to close HTTP listener after startup failure", "error", closeErr)
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if shutdownErr := s.shutdownMetricsServer(shutdownCtx); shutdownErr != nil {
			s.logger.Error("failed to stop metrics server after HTTP server startup failure", "error", shutdownErr)
		}
		return fmt.Errorf("starting HTTP server: %w", err)
	}

	return nil
}

func (s *Server) startMetricsHTTPServer(ctx context.Context) error {
	if s.config == nil || s.config.MetricsAddr == "" {
		return nil
	}

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.config.MetricsAddr)
	if err != nil {
		return fmt.Errorf("listening on metrics address %s: %w", s.config.MetricsAddr, err)
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	metricsServer := &http.Server{
		Addr:         s.config.MetricsAddr,
		Handler:      mux,
		ReadTimeout:  ReadTimeout,
		WriteTimeout: WriteTimeout,
		IdleTimeout:  IdleTimeout,
	}
	metricsDone := make(chan struct{})

	s.metricsServer = metricsServer
	s.metricsLn = listener
	s.metricsDone = metricsDone

	s.logger.Info("starting metrics server", "address", s.config.MetricsAddr)

	go func(server *http.Server, ln net.Listener, done chan struct{}) {
		defer close(done)
		if serveErr := server.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			s.logger.Error("metrics server stopped unexpectedly", "error", serveErr)
		}
	}(metricsServer, listener, metricsDone)

	return nil
}

func (s *Server) shutdownMetricsServer(ctx context.Context) error {
	server := s.metricsServer
	listener := s.metricsLn
	done := s.metricsDone

	s.metricsServer = nil
	s.metricsLn = nil
	s.metricsDone = nil

	if server == nil {
		return nil
	}

	var shutdownErr error
	if err := server.Shutdown(ctx); err != nil {
		shutdownErr = fmt.Errorf("shutting down metrics server: %w", err)
	}

	if listener != nil {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("closing metrics listener: %w", err))
		}
	}

	if done != nil && ctx.Err() == nil {
		select {
		case <-done:
		case <-ctx.Done():
			shutdownErr = errors.Join(shutdownErr, fmt.Errorf("waiting for metrics server shutdown: %w", ctx.Err()))
		}
	}

	return shutdownErr
}

func (s *Server) serveHTTP(listener net.Listener) error {
	if s.serveHTTPFunc != nil {
		return s.serveHTTPFunc(s.httpServer, listener)
	}
	return s.httpServer.Serve(listener)
}

func (s *Server) shutdownHTTP(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	if s.shutdownHTTPFunc != nil {
		return s.shutdownHTTPFunc(s.httpServer, ctx)
	}
	return s.httpServer.Shutdown(ctx)
}

// Stop gracefully stops the HTTP server.
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer == nil && s.metricsServer == nil {
		return nil
	}

	s.logger.Info("stopping HTTP server")

	var shutdownErr error

	if s.httpServer != nil {
		httpShutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		httpErr := s.shutdownHTTP(httpShutdownCtx)
		cancel()
		if httpErr != nil {
			shutdownErr = fmt.Errorf("shutting down HTTP server: %w", httpErr)
		}
	}

	if s.metricsServer != nil {
		metricsShutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		metricsErr := s.shutdownMetricsServer(metricsShutdownCtx)
		cancel()
		if metricsErr != nil {
			shutdownErr = errors.Join(shutdownErr, metricsErr)
		}
	}

	// Close the PowerDNS MySQL connection if it was opened.
	if s.powerDNSDB != nil {
		if err := s.powerDNSDB.Close(); err != nil {
			s.logger.Warn("failed to close PowerDNS MySQL connection", "error", err)
		} else {
			s.logger.Info("PowerDNS MySQL connection closed")
		}
	}

	if shutdownErr != nil {
		return shutdownErr
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
