// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCustomerRepository is a mock implementation for testing
type mockCustomerRepository struct {
	lastReauthAt *time.Time
	err          error
}

func (m *mockCustomerRepository) GetSessionLastReauthAt(ctx context.Context, sessionID string) (*time.Time, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.lastReauthAt, nil
}

func (m *mockCustomerRepository) UpdateSessionLastReauthAt(ctx context.Context, sessionID string, timestamp time.Time) error {
	m.lastReauthAt = &timestamp
	return m.err
}

// Helper to create a test logger
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

// TestRequireReauthForDestructive tests the re-authentication check for destructive actions.
func TestRequireReauthForDestructive(t *testing.T) {
	logger := testLogger()
	ctx := context.Background()

	t.Run("NonDestructiveAction", func(t *testing.T) {
		mock := &mockCustomerRepository{}
		service := NewRBACService(nil, mock, logger)

		// Non-destructive actions should not require re-auth
		result, err := service.RequireReauthForDestructive(ctx, "session-123", ActionCreate)
		require.NoError(t, err)
		assert.False(t, result, "Non-destructive action should not require re-auth")

		result, err = service.RequireReauthForDestructive(ctx, "session-123", ActionRead)
		require.NoError(t, err)
		assert.False(t, result, "Non-destructive action should not require re-auth")

		result, err = service.RequireReauthForDestructive(ctx, "session-123", ActionUpdate)
		require.NoError(t, err)
		assert.False(t, result, "Non-destructive action should not require re-auth")
	})

	t.Run("DestructiveActionWithinWindow", func(t *testing.T) {
		// Re-authenticated 2 minutes ago (within 5-minute window)
		lastReauth := time.Now().Add(-2 * time.Minute)
		mock := &mockCustomerRepository{lastReauthAt: &lastReauth}
		service := NewRBACService(nil, mock, logger)

		result, err := service.RequireReauthForDestructive(ctx, "session-123", ActionDelete)
		require.NoError(t, err)
		assert.False(t, result, "Destructive action within 5-minute window should not require re-auth")
	})

	t.Run("DestructiveActionOutsideWindow", func(t *testing.T) {
		// Re-authenticated 10 minutes ago (outside 5-minute window)
		lastReauth := time.Now().Add(-10 * time.Minute)
		mock := &mockCustomerRepository{lastReauthAt: &lastReauth}
		service := NewRBACService(nil, mock, logger)

		result, err := service.RequireReauthForDestructive(ctx, "session-123", ActionDelete)
		require.NoError(t, err)
		assert.True(t, result, "Destructive action outside 5-minute window should require re-auth")
	})

	t.Run("DestructiveActionAtWindowBoundary", func(t *testing.T) {
		// Re-authenticated exactly 5 minutes ago (at boundary - should still be valid)
		lastReauth := time.Now().Add(-5 * time.Minute)
		mock := &mockCustomerRepository{lastReauthAt: &lastReauth}
		service := NewRBACService(nil, mock, logger)

		result, err := service.RequireReauthForDestructive(ctx, "session-123", ActionDelete)
		require.NoError(t, err)
		assert.False(t, result, "Destructive action at exactly 5-minute boundary should not require re-auth")
	})

	t.Run("DestructiveActionNoLastReauth", func(t *testing.T) {
		// No last_reauth_at recorded
		mock := &mockCustomerRepository{lastReauthAt: nil}
		service := NewRBACService(nil, mock, logger)

		result, err := service.RequireReauthForDestructive(ctx, "session-123", ActionDelete)
		require.NoError(t, err)
		assert.True(t, result, "Destructive action with no last_reauth_at should require re-auth")
	})

	t.Run("DestructiveActionSessionError", func(t *testing.T) {
		// Session lookup error
		mock := &mockCustomerRepository{err: errors.New("session not found")}
		service := NewRBACService(nil, mock, logger)

		result, err := service.RequireReauthForDestructive(ctx, "session-123", ActionDelete)
		require.Error(t, err)
		assert.True(t, result, "Destructive action with session error should require re-auth")
	})

	t.Run("AllDestructiveActions", func(t *testing.T) {
		// Test all destructive actions require re-auth when outside window
		lastReauth := time.Now().Add(-10 * time.Minute)
		mock := &mockCustomerRepository{lastReauthAt: &lastReauth}
		service := NewRBACService(nil, mock, logger)

		destructiveActions := []string{
			ActionDelete,
			ActionForceStop,
			ActionReinstall,
			ActionMigrate,
			ActionFailover,
		}

		for _, action := range destructiveActions {
			result, err := service.RequireReauthForDestructive(ctx, "session-123", action)
			require.NoError(t, err, "Action %s should not error", action)
			assert.True(t, result, "Destructive action %s should require re-auth when outside window", action)
		}
	})

	t.Run("AllDestructiveActionsWithinWindow", func(t *testing.T) {
		// Test all destructive actions don't require re-auth when within window
		lastReauth := time.Now().Add(-2 * time.Minute)
		mock := &mockCustomerRepository{lastReauthAt: &lastReauth}
		service := NewRBACService(nil, mock, logger)

		destructiveActions := []string{
			ActionDelete,
			ActionForceStop,
			ActionReinstall,
			ActionMigrate,
			ActionFailover,
		}

		for _, action := range destructiveActions {
			result, err := service.RequireReauthForDestructive(ctx, "session-123", action)
			require.NoError(t, err, "Action %s should not error", action)
			assert.False(t, result, "Destructive action %s should not require re-auth when within window", action)
		}
	})
}
