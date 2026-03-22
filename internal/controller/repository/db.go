// Package repository provides PostgreSQL database operations for VirtueStack Controller.
// All operations use parameterized queries via pgx/v5 to prevent SQL injection.
package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	apierrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// DB abstracts pgx pool operations for testability.
// Both *pgxpool.Pool and pgx.Tx implement this interface.
type DB interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

// ScanRow executes a query expected to return a single row and scans it using the provided scanner.
// Maps pgx.ErrNoRows to apierrors.ErrNotFound.
func ScanRow[T any](ctx context.Context, db DB, query string, args []any, scanner func(pgx.Row) (T, error)) (T, error) {
	row := db.QueryRow(ctx, query, args...)
	result, err := scanner(row)
	if err != nil {
		var zero T
		if errors.Is(err, pgx.ErrNoRows) {
			return zero, apierrors.ErrNotFound
		}
		return zero, err
	}
	return result, nil
}

// ScanRows executes a query returning multiple rows, applying scanner to each row.
// Returns an empty slice (not nil) if no rows are found.
func ScanRows[T any](ctx context.Context, db DB, query string, args []any, scanner func(pgx.Rows) (T, error)) ([]T, error) {
	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []T
	for rows.Next() {
		item, err := scanner(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if results == nil {
		results = []T{}
	}
	return results, nil
}

// CountRows executes a COUNT query and returns the integer result.
func CountRows(ctx context.Context, db DB, query string, args ...any) (int, error) {
	var count int
	row := db.QueryRow(ctx, query, args...)
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("counting rows: %w", err)
	}
	return count, nil
}

// SetCustomerContext sets the app.current_customer_id session variable for Row Level Security.
// Must be called within a transaction before any RLS-protected queries.
// The 'local=true' parameter ensures the setting is scoped to the current transaction only.
func SetCustomerContext(ctx context.Context, tx pgx.Tx, customerID string) error {
	_, err := tx.Exec(ctx, "SELECT set_config('app.current_customer_id', $1, true)", customerID)
	if err != nil {
		return fmt.Errorf("setting customer context for RLS: %w", err)
	}
	return nil
}

// CustomerScopedDB wraps a transaction with RLS context for customer-scoped operations.
// It provides a DB interface that can be used by repositories for queries that should
// be restricted to a specific customer's data via Row Level Security.
type CustomerScopedDB struct {
	tx pgx.Tx
}

// QueryRow executes a query expected to return a single row.
func (c *CustomerScopedDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return c.tx.QueryRow(ctx, sql, args...)
}

// Query executes a query returning multiple rows.
func (c *CustomerScopedDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return c.tx.Query(ctx, sql, args...)
}

// Exec executes a query that doesn't return rows.
func (c *CustomerScopedDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return c.tx.Exec(ctx, sql, args...)
}

// Begin starts a nested transaction (savepoint).
func (c *CustomerScopedDB) Begin(ctx context.Context) (pgx.Tx, error) {
	return c.tx.Begin(ctx)
}

// Commit commits the underlying transaction.
func (c *CustomerScopedDB) Commit(ctx context.Context) error {
	return c.tx.Commit(ctx)
}

// Rollback rolls back the underlying transaction.
func (c *CustomerScopedDB) Rollback(ctx context.Context) error {
	return c.tx.Rollback(ctx)
}

// WithCustomerContext begins a transaction and sets the RLS context for customer-scoped operations.
// The returned CustomerScopedDB provides a DB interface that enforces Row Level Security.
// The caller is responsible for calling Commit or Rollback on the returned CustomerScopedDB.
//
// Usage:
//
//	cdb, err := repository.WithCustomerContext(ctx, db, customerID)
//	if err != nil {
//	    return err
//	}
//	defer cdb.Rollback(ctx) //nolint:errcheck
//
//	// Use cdb as DB for all customer queries
//	vm, err := vmRepo.GetByID(ctx, cdb, vmID)
//
//	if err := cdb.Commit(ctx); err != nil {
//	    return err
//	}
func WithCustomerContext(ctx context.Context, db DB, customerID string) (*CustomerScopedDB, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction for RLS: %w", err)
	}

	if err := SetCustomerContext(ctx, tx, customerID); err != nil {
		_ = tx.Rollback(ctx) //nolint:errcheck
		return nil, err
	}

	return &CustomerScopedDB{tx: tx}, nil
}

// WithCustomerContextFunc executes fn within a transaction with RLS context set.
// This is a convenience wrapper that handles transaction lifecycle automatically.
// The function receives a DB interface that enforces Row Level Security.
//
// Usage:
//
//	err := repository.WithCustomerContextFunc(ctx, db, customerID, func(txDB DB) error {
//	    vm, err := vmRepo.GetByID(ctx, txDB, vmID)
//	    if err != nil {
//	        return err
//	    }
//	    // ... other operations
//	    return nil
//	})
func WithCustomerContextFunc(ctx context.Context, db DB, customerID string, fn func(DB) error) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction for RLS: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := SetCustomerContext(ctx, tx, customerID); err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ErrNoRowsAffected is returned when an UPDATE or DELETE affects zero rows.
// It is defined in internal/shared/errors and re-exported here for use within
// the repository package.
var ErrNoRowsAffected = apierrors.ErrNoRowsAffected

// mapNotFound converts pgx.ErrNoRows to apierrors.ErrNotFound.
// All other errors are returned as-is.
func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apierrors.ErrNotFound
	}
	return err
}
