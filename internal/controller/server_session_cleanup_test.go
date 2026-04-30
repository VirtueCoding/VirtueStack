package controller

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestStartSessionCleanup_ExitsImmediatelyWithNilDBPool(t *testing.T) {
	// This test verifies the cleanup goroutine exits immediately when dbPool is nil
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a server with nil dbPool (cleanup should return immediately)
	s := &Server{
		dbPool: nil,
		logger: testLogger(),
	}

	// Should return immediately with nil dbPool
	done := make(chan struct{})
	go func() {
		s.startSessionCleanup(ctx)
		close(done)
	}()

	// With nil dbPool, it should exit immediately
	select {
	case <-done:
		// Expected: exited immediately
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected cleanup to exit immediately with nil dbPool")
	}
}

func TestStartSessionCleanup_ContextCancellation(t *testing.T) {
	// Skip if no database available
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create server with test database
	s, _, cleanup := setupTestServer(t)
	defer cleanup()

	done := make(chan struct{})
	go func() {
		s.startSessionCleanup(ctx)
		close(done)
	}()

	// Wait a moment for goroutine to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context
	cancel()

	// Should stop within reasonable time
	select {
	case <-done:
		// Expected: stopped cleanly
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup goroutine did not stop after context cancellation")
	}
}

func TestCleanupExpiredSessions(t *testing.T) {
	// Skip if no database available
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	s, dbPool, cleanup := setupTestServer(t)
	defer cleanup()

	customerRepo := repository.NewCustomerRepository(dbPool)

	// Call cleanup - should not panic even with no expired sessions
	s.cleanupExpiredSessions(ctx, customerRepo)
}

// Helper to create test logger
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

// setupTestServer creates a test server with database.
// Returns the server, db pool, and cleanup function.
// This is a placeholder - real implementation would use test containers or test DB.
func setupTestServer(t *testing.T) (*Server, *pgxpool.Pool, func()) {
	// Placeholder for integration test setup
	// In real implementation, this would:
	// 1. Start a test PostgreSQL container
	// 2. Run migrations
	// 3. Return server and cleanup function
	t.Skip("requires test database setup")
	return nil, nil, func() {}
}

// Prevent unused import warning for require (used in integration tests)
var _ = require.NotNil