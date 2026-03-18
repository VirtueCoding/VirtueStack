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
func SetCustomerContext(ctx context.Context, tx pgx.Tx, customerID string) error {
	_, err := tx.Exec(ctx, "SELECT set_config('app.current_customer_id', $1, true)", customerID)
	if err != nil {
		return fmt.Errorf("setting customer context for RLS: %w", err)
	}
	return nil
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
