// Package main is the entrypoint for the VirtueStack Controller.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	shutdownTimeout   = 10 * time.Second
)

func main() {
	// Load configuration
	cfg, err := controller.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Setup logging
	logger := logging.NewLogger(cfg.LogLevel)
	logger.Info("VirtueStack Controller starting")

	// Create context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to PostgreSQL
	dbPool, err := connectDatabase(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		logger.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer closeDBPool(dbPool, logger)

	// Connect to NATS
	nc, js, err := connectNATS(cfg.NatsURL, logger)
	if err != nil {
		logger.Error("Failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer closeNATS(nc, logger)

	// Create task worker
	worker, err := tasks.NewWorker(js, dbPool, logger)
	if err != nil {
		logger.Error("Failed to create task worker", "error", err)
		os.Exit(1)
	}

	// Create HTTP server
	server, err := controller.NewServer(cfg.ControllerConfig, dbPool, js, logger)
	if err != nil {
		logger.Error("Failed to create server", "error", err)
		os.Exit(1)
	}

	var nodeClient *controller.NodeClient
	if tlsCAFile := os.Getenv("TLS_CA_FILE"); tlsCAFile != "" {
		nodeClient, err = controller.NewNodeClient(tlsCAFile, logger)
		if err != nil {
			logger.Error("Failed to create node client", "error", err, "tls_ca_file", tlsCAFile)
			os.Exit(1)
		}
		logger.Info("Configured secure node client", "tls_ca_file", tlsCAFile)
	} else {
		logger.Warn("TLS_CA_FILE is not set, using insecure node client")
		nodeClient = controller.InsecureNodeClient(logger)
	}
	server.SetNodeClient(nodeClient)

	server.SetTaskWorker(worker)
	server.SetNATSConnection(nc)

	// Initialize services
	if err := server.InitializeServices(); err != nil {
		logger.Error("Failed to initialize services", "error", err)
		os.Exit(1)
	}

	// Register task handlers
	handlerDeps := &tasks.HandlerDeps{
		VMRepo:         repository.NewVMRepository(dbPool),
		NodeRepo:       repository.NewNodeRepository(dbPool),
		IPRepo:         repository.NewIPRepository(dbPool),
		BackupRepo:     repository.NewBackupRepository(dbPool),
		TaskRepo:       repository.NewTaskRepository(dbPool),
		TemplateRepo:   repository.NewTemplateRepository(dbPool),
		IPAMService:    server.GetIPAMService(),
		NodeClient:     services.NewNodeAgentGRPCClient(repository.NewNodeRepository(dbPool), repository.NewVMRepository(dbPool), nodeClient, logger),
		DNSNameservers: cfg.DNSNameservers,
		CephUser:       cfg.CephUser,
		CephSecretUUID: cfg.CephSecretUUID,
		CephMonitors:   cfg.CephMonitors,
		Logger:         logger,
	}
	tasks.RegisterAllHandlers(worker, handlerDeps)

	// Register API routes after services are initialized
	server.RegisterAPIRoutes()

	// Start task worker
	if err := worker.Start(ctx, defaultNumWorkers); err != nil {
		logger.Error("Failed to start task worker", "error", err)
		os.Exit(1)
	}

	// Start background schedulers (backup scheduler)
	server.StartSchedulers(ctx)

	// Start HTTP server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		if err := server.Start(ctx); err != nil {
			serverErr <- err
		}
	}()

	logger.Info("VirtueStack Controller started",
		"address", cfg.ListenAddr,
		"workers", defaultNumWorkers,
	)

	// Wait for shutdown signal or server error
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-shutdownChan:
		logger.Info("Received shutdown signal", "signal", sig.String())
	case err := <-serverErr:
		logger.Error("Server error", "error", err)
	}

	// Initiate graceful shutdown
	logger.Info("Initiating graceful shutdown")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	// Stop task worker
	worker.Stop()

	// Stop HTTP server
	if err := server.Stop(shutdownCtx); err != nil {
		logger.Error("Error stopping HTTP server", "error", err)
	}

	logger.Info("VirtueStack Controller stopped")
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
		nats.MaxReconnects(10),
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
