package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AdvisoryLockDB wraps a pgx pool to provide PostgreSQL advisory lock operations.
type AdvisoryLockDB struct {
	pool *pgxpool.Pool
}

// NewAdvisoryLockDB creates a new AdvisoryLockDB.
func NewAdvisoryLockDB(pool *pgxpool.Pool) *AdvisoryLockDB {
	return &AdvisoryLockDB{pool: pool}
}

// TryAdvisoryLock attempts to acquire a session-level advisory lock.
// Returns true if the lock was acquired, false if held by another session.
func (d *AdvisoryLockDB) TryAdvisoryLock(ctx context.Context, lockID int64) (bool, error) {
	var acquired bool
	err := d.pool.QueryRow(ctx,
		"SELECT pg_try_advisory_lock($1)", lockID,
	).Scan(&acquired)
	if err != nil {
		return false, fmt.Errorf("try advisory lock: %w", err)
	}
	return acquired, nil
}

// ReleaseAdvisoryLock releases a session-level advisory lock.
func (d *AdvisoryLockDB) ReleaseAdvisoryLock(ctx context.Context, lockID int64) error {
	var released bool
	err := d.pool.QueryRow(ctx,
		"SELECT pg_advisory_unlock($1)", lockID,
	).Scan(&released)
	if err != nil {
		return fmt.Errorf("release advisory lock: %w", err)
	}
	return nil
}
