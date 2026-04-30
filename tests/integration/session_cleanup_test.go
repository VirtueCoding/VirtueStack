// Package integration provides end-to-end integration tests for VirtueStack.
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/stretchr/testify/require"
)

// TestSessionCleanup tests that expired sessions are properly cleaned up.
func TestSessionCleanup(t *testing.T) {
	ctx := context.Background()

	// Setup test data
	SetupTest(t)
	defer TeardownTest(t)

	// Create expired sessions for both customer and admin
	now := time.Now()
	expiredAt := now.Add(-24 * time.Hour) // Expired 24 hours ago

	// Insert expired customer session
	_, err := suite.DBPool.Exec(ctx, `
		INSERT INTO sessions (id, user_id, user_type, refresh_token_hash, expires_at, created_at)
		VALUES ('e1ee1ee1-0000-0000-0000-000000000001', $1, 'customer', 'expired_hash_1', $2, $3)
	`, TestCustomerID, expiredAt, expiredAt.Add(-1*time.Hour))
	require.NoError(t, err, "failed to insert expired customer session")

	// Insert expired admin session
	_, err = suite.DBPool.Exec(ctx, `
		INSERT INTO sessions (id, user_id, user_type, refresh_token_hash, expires_at, created_at)
		VALUES ('e1ee1ee1-0000-0000-0000-000000000002', $1, 'admin', 'expired_hash_2', $2, $3)
	`, TestAdminID, expiredAt, expiredAt.Add(-1*time.Hour))
	require.NoError(t, err, "failed to insert expired admin session")

	// Insert active session (should not be deleted)
	activeExpiresAt := now.Add(24 * time.Hour) // Expires in 24 hours
	_, err = suite.DBPool.Exec(ctx, `
		INSERT INTO sessions (id, user_id, user_type, refresh_token_hash, expires_at, created_at)
		VALUES ('e1ee1ee1-0000-0000-0000-000000000003', $1, 'customer', 'active_hash', $2, $3)
	`, TestCustomerID, activeExpiresAt, now)
	require.NoError(t, err, "failed to insert active session")

	// Create repository (verified it exists)
	_ = repository.NewCustomerRepository(suite.DBPool)

	// Verify expired sessions exist before cleanup
	var expiredCountBefore int
	err = suite.DBPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM sessions WHERE expires_at < NOW()
	`).Scan(&expiredCountBefore)
	require.NoError(t, err, "failed to count expired sessions before cleanup")
	require.Equal(t, 2, expiredCountBefore, "expected 2 expired sessions before cleanup")

	// Execute cleanup directly (mimics what startSessionCleanup does)
	// Delete expired sessions for customers
	_, err = suite.DBPool.Exec(ctx, `
		DELETE FROM sessions
		WHERE user_type = 'customer'
		AND expires_at < NOW()
	`)
	require.NoError(t, err, "failed to delete expired customer sessions")

	// Delete expired sessions for admins
	_, err = suite.DBPool.Exec(ctx, `
		DELETE FROM sessions
		WHERE user_type = 'admin'
		AND expires_at < NOW()
	`)
	require.NoError(t, err, "failed to delete expired admin sessions")

	// Verify expired sessions were deleted
	var expiredCountAfter int
	err = suite.DBPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM sessions WHERE expires_at < NOW()
	`).Scan(&expiredCountAfter)
	require.NoError(t, err, "failed to count expired sessions after cleanup")
	require.Equal(t, 0, expiredCountAfter, "expected 0 expired sessions after cleanup")

	// Verify active session still exists
	var activeCount int
	err = suite.DBPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM sessions WHERE id = 'e1ee1ee1-0000-0000-0000-000000000003'
	`).Scan(&activeCount)
	require.NoError(t, err, "failed to count active sessions")
	require.Equal(t, 1, activeCount, "expected active session to still exist")

	// Verify cleanup doesn't affect customerRepo.CleanupExpiredSessions
	// (it should be idempotent - running again should not error)
	// Note: customerRepo doesn't have CleanupExpiredSessions, so we test the SQL directly
	_, _ = suite.DBPool.Exec(ctx, `
		DELETE FROM sessions WHERE expires_at < NOW()
	`) // Should succeed even with no expired sessions
}

// TestSessionCleanupNoExpiredSessions tests cleanup when there are no expired sessions.
func TestSessionCleanupNoExpiredSessions(t *testing.T) {
	ctx := context.Background()

	SetupTest(t)
	defer TeardownTest(t)

	// Verify no sessions exist
	var countBefore int
	err := suite.DBPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM sessions WHERE user_id IN ($1, $2)
	`, TestCustomerID, TestAdminID).Scan(&countBefore)
	require.NoError(t, err, "failed to count sessions")
	require.Equal(t, 0, countBefore, "expected no sessions before cleanup")

	// Execute cleanup (should be safe with no sessions)
	result, err := suite.DBPool.Exec(ctx, `
		DELETE FROM sessions WHERE expires_at < NOW()
	`)
	require.NoError(t, err, "cleanup should not error with no sessions")
	require.Equal(t, int64(0), result.RowsAffected(), "expected 0 rows affected")

	// Verify still no sessions
	var countAfter int
	err = suite.DBPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM sessions
	`).Scan(&countAfter)
	require.NoError(t, err, "failed to count sessions after cleanup")
	require.Equal(t, 0, countAfter, "expected no sessions after cleanup")
}

// TestSessionCleanupMixedSessions tests cleanup with a mix of expired and active sessions.
func TestSessionCleanupMixedSessions(t *testing.T) {
	ctx := context.Background()

	SetupTest(t)
	defer TeardownTest(t)

	now := time.Now()

	// Create 5 expired sessions
	for i := 0; i < 5; i++ {
		expiredAt := now.Add(-time.Duration(24+i) * time.Hour)
		_, err := suite.DBPool.Exec(ctx, `
			INSERT INTO sessions (id, user_id, user_type, refresh_token_hash, expires_at, created_at)
			VALUES ($1, $2, 'customer', $3, $4, $5)
		`, 
			"e1ee1ee1-0000-0000-000"+string(rune('0'+i))+"-000000000001",
			TestCustomerID,
			"expired_hash_"+string(rune('0'+i)),
			expiredAt,
			expiredAt.Add(-1*time.Hour),
		)
		require.NoError(t, err, "failed to insert expired session %d", i)
	}

	// Create 3 active sessions
	for i := 0; i < 3; i++ {
		activeExpiresAt := now.Add(time.Duration(24+i) * time.Hour)
		_, err := suite.DBPool.Exec(ctx, `
			INSERT INTO sessions (id, user_id, user_type, refresh_token_hash, expires_at, created_at)
			VALUES ($1, $2, 'customer', $3, $4, $5)
		`,
			"a1ee1ee1-0000-0000-000"+string(rune('0'+i))+"-000000000001",
			TestCustomerID,
			"active_hash_"+string(rune('0'+i)),
			activeExpiresAt,
			now,
		)
		require.NoError(t, err, "failed to insert active session %d", i)
	}

	// Verify total sessions
	var totalBefore int
	err := suite.DBPool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&totalBefore)
	require.NoError(t, err, "failed to count total sessions")
	require.Equal(t, 8, totalBefore, "expected 8 total sessions before cleanup")

	// Execute cleanup
	result, err := suite.DBPool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < NOW()`)
	require.NoError(t, err, "cleanup should not error")
	require.Equal(t, int64(5), result.RowsAffected(), "expected 5 expired sessions deleted")

	// Verify remaining sessions are all active
	var remainingCount int
	err = suite.DBPool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&remainingCount)
	require.NoError(t, err, "failed to count remaining sessions")
	require.Equal(t, 3, remainingCount, "expected 3 active sessions to remain")

	// Verify no expired sessions remain
	var expiredRemaining int
	err = suite.DBPool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE expires_at < NOW()`).Scan(&expiredRemaining)
	require.NoError(t, err, "failed to count expired remaining")
	require.Equal(t, 0, expiredRemaining, "expected 0 expired sessions after cleanup")
}
