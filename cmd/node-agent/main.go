// Package main is the entrypoint for the VirtueStack Node Agent.
// The Node Agent runs on bare-metal servers and manages VMs via libvirt.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/nodeagent"
	"github.com/AbuGosok/VirtueStack/internal/shared/logging"
)

// Default shutdown timeout for graceful termination (can be overridden via config).
const defaultShutdownTimeout = 30 * time.Second

func main() {
	os.Exit(run())
}

// run contains the main application logic. It returns an exit code.
// Using a separate function ensures deferred functions run before os.Exit.
func run() int {
	// Load configuration
	cfg, err := nodeagent.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		return 1
	}

	// Parse shutdown timeout from config (default to 30s)
	shutdownTimeout := defaultShutdownTimeout
	if cfg.ShutdownTimeout != "" {
		if parsed, err := time.ParseDuration(cfg.ShutdownTimeout); err == nil {
			shutdownTimeout = parsed
		}
	}

	// Setup logging
	logging.Setup(cfg.LogLevel)
	logger := slog.Default()

	logger.Info("VirtueStack Node Agent starting",
		"node_id", cfg.NodeID,
		"controller_addr", cfg.ControllerGRPCAddr,
	)

	// Create context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create the Node Agent server
	server, err := nodeagent.NewServer(cfg, logger)
	if err != nil {
		logger.Error("Failed to create server", "error", err)
		return 1
	}

	// Start the gRPC server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		if err := server.Start(ctx); err != nil {
			serverErr <- err
		}
	}()

	logger.Info("VirtueStack Node Agent started")

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

	// Stop the server. Stop() is synchronous and returns as soon as the gRPC
	// server has drained and all background goroutines have completed, so we
	// do not wait on shutdownCtx.Done() afterward. The context is kept only to
	// surface a timeout warning if Stop() somehow blocks past the deadline.
	stopDone := make(chan struct{})
	go func() {
		server.Stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
		// Server stopped cleanly before the timeout.
	case <-shutdownCtx.Done():
		logger.Warn("Shutdown timeout exceeded, forcing exit")
	}

	logger.Info("VirtueStack Node Agent stopped")
	return 0
}