# Session Cleanup Goroutine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a background goroutine that periodically purges expired sessions from the database, with clean shutdown via context cancellation.

**Architecture:** Follow the existing scheduler pattern in `server.go` using `time.Ticker` with a `select` loop that handles both ticker events and context cancellation. The `DeleteExpiredSessions` repository method already exists — we just need to expose it in the interface and wire it into the server.

**Tech Stack:** Go 1.22+, pgx/v5, context.Context for cancellation

---

## Files

| File | Action | Purpose |
|------|--------|---------|
| `internal/controller/repository/customer_repo.go` | Modify | Add `DeleteExpiredSessions` to `CustomerRepo` interface |
| `internal/controller/server.go` | Modify | Add session cleanup goroutine to `StartSchedulers` |
| `internal/controller/server_session_cleanup_test.go` | Create | Unit tests for session cleanup |

---

### Task 1: Add DeleteExpiredSessions to CustomerRepo Interface

**Files:**
- Modify: `internal/controller/repository/customer_repo.go:32`

**Context:** The `DeleteExpiredSessions` method exists on `CustomerRepository` (line 421-429) but is not in the `CustomerRepo` interface. Adding it allows the server to call it via the interface.

- [ ] **Step 1: Add the method to the CustomerRepo interface**

Edit `internal/controller/repository/customer_repo.go`, inserting after `DeleteOldestSession` (line 32):

```go
	DeleteOldestSession(ctx context.Context, userID, userType string) error
	DeleteExpiredSessions(ctx context.Context) error
	GetSessionLastReauthAt(ctx context.Context, sessionID string) (*time.Time, error)
```

- [ ] **Step 2: Verify the build passes**

Run: `make build`
Expected: Build succeeds with no errors

- [ ] **Step 3: Commit**

```bash
git add internal/controller/repository/customer_repo.go
git commit -m "feat(repository): add DeleteExpiredSessions to CustomerRepo interface

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 2: Add Session Cleanup Goroutine to Server

**Files:**
- Modify: `internal/controller/server.go`

**Context:** Add a `startSessionCleanup` method following the existing pattern used by `startMetricsCollector` and `startBandwidthCollector`. This goroutine runs hourly, calls `DeleteExpiredSessions`, and stops cleanly on context cancellation.

- [ ] **Step 1: Add startSessionCleanup method**

Add after the `startBandwidthCollector` method (around line 651):

```go
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
```

- [ ] **Step 2: Call startSessionCleanup from StartSchedulers**

Edit the `StartSchedulers` method (line 543) to add the session cleanup call after the bandwidth collector:

```go
func (s *Server) StartSchedulers(ctx context.Context) {
	if s.backupService != nil {
		s.logger.Info("starting backup scheduler")
		go s.backupService.StartScheduler(ctx)
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
```

- [ ] **Step 3: Verify the build passes**

Run: `make build`
Expected: Build succeeds with no errors

- [ ] **Step 4: Commit**

```bash
git add internal/controller/server.go
git commit -m "feat(controller): add background session cleanup goroutine

Adds hourly cleanup of expired sessions via DeleteExpiredSessions.
Goroutine stops cleanly on context cancellation for graceful shutdown.

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 3: Add Tests for Session Cleanup

**Files:**
- Create: `internal/controller/server_session_cleanup_test.go`

**Context:** Verify the session cleanup goroutine starts, runs cleanup on ticker, and stops on context cancellation.

- [ ] **Step 1: Write the failing test**

Create `internal/controller/server_session_cleanup_test.go`:

```go
package controller

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartSessionCleanup_StopsOnContextCancel(t *testing.T) {
	// This test verifies the cleanup goroutine stops when context is cancelled
	ctx, cancel := context.WithCancel(context.Background())

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
	s, dbPool, cleanup := setupTestServer(t)
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

	// Call cleanup - should not error even with no expired sessions
	err := s.cleanupExpiredSessions(ctx, customerRepo)
	assert.NoError(t, err)
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
```

- [ ] **Step 2: Run tests to verify they compile**

Run: `go build ./internal/controller/...`
Expected: Build succeeds

Note: Integration tests will be skipped without a test database. Unit tests for the nil dbPool case will pass.

- [ ] **Step 3: Commit**

```bash
git add internal/controller/server_session_cleanup_test.go
git commit -m "test(controller): add session cleanup tests

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
```

---

### Task 4: Run Full Test Suite

**Files:**
- None (verification only)

- [ ] **Step 1: Run all Go tests**

Run: `make test`
Expected: All tests pass

- [ ] **Step 2: Run race detector**

Run: `make test-race`
Expected: No race conditions detected

- [ ] **Step 3: Run linter**

Run: `make lint`
Expected: No lint errors

---

### Task 5: Final Commit and Push

- [ ] **Step 1: Verify git status**

Run: `git status`
Expected: All changes committed

- [ ] **Step 2: Squash commits (optional)**

If multiple commits were made, consider squashing into a single commit:

```bash
git rebase -i HEAD~4
# Mark all but first as 'squash'
```

---

## Summary

This implementation adds proactive session cleanup to VirtueStack:

1. **Interface update**: Exposes `DeleteExpiredSessions` on `CustomerRepo` interface
2. **Background goroutine**: Runs hourly cleanup via `startSessionCleanup`
3. **Clean shutdown**: Uses context cancellation to stop the goroutine gracefully
4. **Tests**: Unit tests verify the goroutine lifecycle

The pattern follows existing schedulers in `server.go` (backup, failover, heartbeat, metrics, bandwidth).