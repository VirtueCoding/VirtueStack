package repository

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

// AdvisoryLockDB wraps a pgx pool to provide PostgreSQL advisory lock operations.
// It pins a connection on lock acquire so that release uses the same connection.
type AdvisoryLockDB struct {
	pool *pgxpool.Pool
	mu   sync.Mutex
	conn *pgxpool.Conn
}

// NewAdvisoryLockDB creates a new AdvisoryLockDB.
func NewAdvisoryLockDB(pool *pgxpool.Pool) *AdvisoryLockDB {
	return &AdvisoryLockDB{pool: pool}
}

// TryAdvisoryLock acquires a dedicated connection from the pool and attempts
// a session-level advisory lock on it. The connection is retained so that
// ReleaseAdvisoryLock can unlock on the same connection.
func (d *AdvisoryLockDB) TryAdvisoryLock(ctx context.Context, lockID int64) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	conn, err := d.pool.Acquire(ctx)
	if err != nil {
		return false, fmt.Errorf("acquire connection for advisory lock: %w", err)
	}

	var acquired bool
	err = conn.QueryRow(ctx,
		"SELECT pg_try_advisory_lock($1)", lockID,
	).Scan(&acquired)
	if err != nil {
		conn.Release()
		return false, fmt.Errorf("try advisory lock: %w", err)
	}

	if !acquired {
		conn.Release()
		return false, nil
	}

	d.conn = conn
	return true, nil
}

// ReleaseAdvisoryLock releases the session-level advisory lock on the pinned
// connection that was used to acquire it, then returns the connection to the pool.
func (d *AdvisoryLockDB) ReleaseAdvisoryLock(ctx context.Context, lockID int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.conn == nil {
		return nil
	}

	var released bool
	err := d.conn.QueryRow(ctx,
		"SELECT pg_advisory_unlock($1)", lockID,
	).Scan(&released)
	d.conn.Release()
	d.conn = nil
	if err != nil {
		return fmt.Errorf("release advisory lock: %w", err)
	}
	return nil
}
