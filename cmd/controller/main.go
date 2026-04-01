// Package main is the entrypoint for the VirtueStack Controller.
//
// @title VirtueStack API
// @version 1.0
// @description KVM/QEMU VM management platform API
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @securityDefinitions.apikey APIKeyAuth
// @in header
// @name X-API-Key
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/AbuGosok/VirtueStack/docs"
	"github.com/AbuGosok/VirtueStack/internal/controller"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	"github.com/AbuGosok/VirtueStack/internal/controller/tasks"
	"github.com/AbuGosok/VirtueStack/internal/shared/logging"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

// Default configuration values.
const (
	defaultNumWorkers = 4
	shutdownTimeout   = 30 * time.Second
)

// infrastructure holds the initialized external dependencies.
type infrastructure struct {
	dbPool *pgxpool.Pool
	nc     *nats.Conn
	js     nats.JetStreamContext
}

func main() {
	os.Exit(run())
}

// run contains the main application logic. It returns an exit code.
// Using a separate function ensures deferred functions run before os.Exit.
func run() int {
	// Load configuration
	cfg, err := controller.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		return 1
	}

	// Setup logging
	logging.Setup(cfg.LogLevel)
	logger := logging.NewLogger(cfg.LogLevel)
	logger.Info("VirtueStack Controller starting")

	// Create context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize infrastructure (database, NATS)
	infra, err := initializeInfrastructure(ctx, cfg, logger)
	if err != nil {
		logger.Error("Failed to initialize infrastructure", "error", err)
		return 1
	}
	defer closeDBPool(infra.dbPool, logger)
	defer closeNATS(infra.nc, logger)

	// Initialize server, services, task worker, and routes
	srv, worker, err := initializeServer(ctx, cfg, infra, logger)
	if err != nil {
		logger.Error("Failed to initialize server", "error", err)
		return 1
	}

	// Start task worker
	if err := worker.Start(ctx, defaultNumWorkers); err != nil {
		logger.Error("Failed to start task worker", "error", err)
		return 1
	}

	// Start background schedulers (backup scheduler)
	srv.StartSchedulers(ctx)

	// Start HTTP server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		if err := srv.Start(ctx); err != nil {
			serverErr <- err
		}
	}()

	logger.Info("VirtueStack Controller started",
		"address", cfg.ListenAddr,
		"workers", defaultNumWorkers,
	)

	// Wait for shutdown signal or server error, then shut down gracefully
	runShutdown(logger, serverErr, worker, srv)

	logger.Info("VirtueStack Controller stopped")
	return 0
}

// initializeInfrastructure connects to PostgreSQL and NATS.
func initializeInfrastructure(ctx context.Context, cfg *controller.Config, logger *slog.Logger) (*infrastructure, error) {
	dbPool, err := connectDatabase(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	nc, js, err := connectNATS(cfg.NATS.URL, logger)
	if err != nil {
		dbPool.Close()
		return nil, fmt.Errorf("connecting to NATS: %w", err)
	}

	return &infrastructure{dbPool: dbPool, nc: nc, js: js}, nil
}

// initializeServer creates and wires up the HTTP server, node client, services,
// task handlers, and API routes. It returns the server and task worker, ready to start.
func initializeServer(ctx context.Context, cfg *controller.Config, infra *infrastructure, logger *slog.Logger) (*controller.Server, *tasks.Worker, error) {
	worker, err := tasks.NewWorker(infra.js, infra.dbPool, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("creating task worker: %w", err)
	}

	server, err := controller.NewServer(cfg.ControllerConfig, infra.dbPool, infra.js, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("creating server: %w", err)
	}

	nodeClient, err := buildNodeClient(cfg, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("building node client: %w", err)
	}
	server.SetNodeClient(nodeClient)
	server.SetTaskWorker(worker)
	server.SetNATSConnection(infra.nc)

	if err := server.InitializeServices(); err != nil {
		return nil, nil, fmt.Errorf("initializing services: %w", err)
	}

	handlerDeps := &tasks.HandlerDeps{
		VMRepo:            repository.NewVMRepository(infra.dbPool),
		NodeRepo:          repository.NewNodeRepository(infra.dbPool),
		IPRepo:            repository.NewIPRepository(infra.dbPool),
		BackupRepo:        repository.NewBackupRepository(infra.dbPool),
		TaskRepo:          repository.NewTaskRepository(infra.dbPool),
		TemplateRepo:      repository.NewTemplateRepository(infra.dbPool),
		TemplateCacheRepo: repository.NewTemplateCacheRepository(infra.dbPool),
		IPAMService:       server.GetIPAMService(),
		NodeClient: services.NewNodeAgentGRPCClient(repository.NewNodeRepository(infra.dbPool), repository.NewVMRepository(infra.dbPool), nodeClient, &services.CephConfig{
			Monitors:   cfg.CephMonitors,
			User:       cfg.CephUser,
			SecretUUID: cfg.CephSecretUUID,
		}, logger),
		DNSNameservers: cfg.DNSNameservers,
		CephUser:       cfg.CephUser,
		CephSecretUUID: cfg.CephSecretUUID,
		CephMonitors:   cfg.CephMonitors,
		Logger:         logger,
	}
	tasks.RegisterAllHandlers(worker, handlerDeps)

	// RegisterAPIRoutes sets up HTTP routes. Context is obtained from the HTTP request
	// at runtime in the middleware handlers, not during route registration.
	//nolint:contextcheck // Route registration does not require context; middleware obtains it from HTTP request
	server.RegisterAPIRoutes()

	return server, worker, nil
}

// buildNodeClient creates a node client using TLS if TLS_CA_FILE is set,
// or an insecure client for non-production environments.
func buildNodeClient(cfg *controller.Config, logger *slog.Logger) (*controller.NodeClient, error) {
	if tlsCAFile := os.Getenv("TLS_CA_FILE"); tlsCAFile != "" {
		client, err := controller.NewNodeClient(tlsCAFile, logger)
		if err != nil {
			return nil, fmt.Errorf("creating secure node client (tls_ca_file=%s): %w", tlsCAFile, err)
		}
		logger.Info("Configured secure node client", "tls_ca_file", tlsCAFile)
		return client, nil
	}

	if cfg.Environment == "production" {
		return nil, fmt.Errorf("TLS_CA_FILE must be set in production; refusing to start with insecure node client")
	}

	logger.Warn("TLS_CA_FILE is not set, using insecure node client")
	return controller.InsecureNodeClient(logger), nil
}

// runShutdown blocks until a SIGINT/SIGTERM is received or the server reports
// an error, then performs a graceful shutdown within shutdownTimeout.
func runShutdown(logger *slog.Logger, serverErr <-chan error, worker *tasks.Worker, server *controller.Server) {
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-shutdownChan:
		logger.Info("Received shutdown signal", "signal", sig.String())
	case err := <-serverErr:
		logger.Error("Server error", "error", err)
	}

	logger.Info("Initiating graceful shutdown")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	worker.Stop()

	if err := server.Stop(shutdownCtx); err != nil {
		logger.Error("Error stopping HTTP server", "error", err)
	}
}

// connectDatabase creates a connection pool to PostgreSQL.
func connectDatabase(ctx context.Context, databaseURL string, logger *slog.Logger) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database URL: %w", err)
	}

	// Configure pool
	poolConfig.MaxConns = 25
	poolConfig.MinConns = 5
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	logger.Info("Connected to PostgreSQL")
	return pool, nil
}

// connectNATS connects to NATS and returns the connection and JetStream context.
func connectNATS(natsURL string, logger *slog.Logger) (*nats.Conn, nats.JetStreamContext, error) {
	nc, err := nats.Connect(natsURL,
		nats.Name("VirtueStack-Controller"),
		nats.ReconnectWait(2*time.Second),
		nats.MaxReconnects(-1),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			if err != nil {
				logger.Warn("NATS disconnected", "error", err)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			logger.Info("NATS reconnected", "url", nc.ConnectedUrl())
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			logger.Warn("NATS connection closed")
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to NATS: %w", err)
	}

	// Get JetStream context
	js, err := nc.JetStream(nats.MaxWait(30 * time.Second))
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("getting JetStream context: %w", err)
	}

	logger.Info("Connected to NATS", "url", nc.ConnectedUrl())
	return nc, js, nil
}

// closeDBPool closes the database connection pool.
func closeDBPool(pool *pgxpool.Pool, logger *slog.Logger) {
	logger.Info("Closing database connection pool")
	pool.Close()
}

// closeNATS closes the NATS connection.
func closeNATS(nc *nats.Conn, logger *slog.Logger) {
	logger.Info("Closing NATS connection")
	nc.Close()
}
