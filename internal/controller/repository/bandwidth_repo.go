// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// BandwidthRepository provides database operations for bandwidth usage tracking.
type BandwidthRepository struct {
	db DB
}

// NewBandwidthRepository creates a new BandwidthRepository with the given database connection.
func NewBandwidthRepository(db DB) *BandwidthRepository {
	return &BandwidthRepository{db: db}
}

// scanBandwidthUsage scans a single bandwidth_usage row into a models.BandwidthUsage struct.
func scanBandwidthUsage(row pgx.Row) (models.BandwidthUsage, error) {
	var usage models.BandwidthUsage
	err := row.Scan(
		&usage.VMID, &usage.Year, &usage.Month,
		&usage.BytesIn, &usage.BytesOut,
		&usage.LimitBytes, &usage.Throttled,
		&usage.ThrottledAt, &usage.ResetAt,
	)
	return usage, err
}

const bandwidthUsageSelectCols = `
	vm_id, year, month,
	bytes_in, bytes_out,
	limit_bytes, throttled,
	throttled_at, reset_at`

// GetUsage retrieves bandwidth usage for a VM for a specific month.
// Returns ErrNotFound if no record exists.
func (r *BandwidthRepository) GetUsage(ctx context.Context, vmID string, year, month int) (*models.BandwidthUsage, error) {
	const q = `SELECT ` + bandwidthUsageSelectCols + ` FROM bandwidth_usage WHERE vm_id = $1 AND year = $2 AND month = $3`
	usage, err := ScanRow(ctx, r.db, q, []any{vmID, year, month}, scanBandwidthUsage)
	if err != nil {
		return nil, fmt.Errorf("getting bandwidth usage for VM %s: %w", vmID, err)
	}
	return &usage, nil
}

// GetOrCreateUsage retrieves bandwidth usage for a VM, creating a new record if none exists.
// The limitBytes is used when creating a new record.
func (r *BandwidthRepository) GetOrCreateUsage(ctx context.Context, vmID string, year, month int, limitBytes uint64) (*models.BandwidthUsage, error) {
	// Try to get existing record
	usage, err := r.GetUsage(ctx, vmID, year, month)
	if err == nil {
		return usage, nil
	}

	// If not found, create new record
	const q = `
		INSERT INTO bandwidth_usage (vm_id, year, month, limit_bytes)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (vm_id, year, month) DO NOTHING
		RETURNING ` + bandwidthUsageSelectCols

	row := r.db.QueryRow(ctx, q, vmID, year, month, limitBytes)
	newUsage, err := scanBandwidthUsage(row)
	if err != nil {
		// Race condition: another process created the record
		// Try to get it again
		return r.GetUsage(ctx, vmID, year, month)
	}
	return &newUsage, nil
}

// UpdateUsage updates the bandwidth counters for a VM.
// This is called periodically (e.g., daily) to record accumulated usage.
func (r *BandwidthRepository) UpdateUsage(ctx context.Context, vmID string, year, month int, bytesIn, bytesOut uint64) error {
	const q = `
		INSERT INTO bandwidth_usage (vm_id, year, month, bytes_in, bytes_out)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (vm_id, year, month) DO UPDATE SET
			bytes_in = bandwidth_usage.bytes_in + EXCLUDED.bytes_in,
			bytes_out = bandwidth_usage.bytes_out + EXCLUDED.bytes_out`

	_, err := r.db.Exec(ctx, q, vmID, year, month, bytesIn, bytesOut)
	if err != nil {
		return fmt.Errorf("updating bandwidth usage for VM %s: %w", vmID, err)
	}
	return nil
}

// SetThrottled marks a VM as throttled or unthrottled.
func (r *BandwidthRepository) SetThrottled(ctx context.Context, vmID string, year, month int, throttled bool) error {
	var throttledAt *time.Time
	if throttled {
		now := time.Now().UTC()
		throttledAt = &now
	}

	const q = `
		UPDATE bandwidth_usage SET throttled = $1, throttled_at = $2
		WHERE vm_id = $3 AND year = $4 AND month = $5`

	tag, err := r.db.Exec(ctx, q, throttled, throttledAt, vmID, year, month)
	if err != nil {
		return fmt.Errorf("setting throttled status for VM %s: %w", vmID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("setting throttled status for VM %s: %w", vmID, ErrNoRowsAffected)
	}
	return nil
}

// UpdateLimit updates the bandwidth limit for a VM's current billing period.
func (r *BandwidthRepository) UpdateLimit(ctx context.Context, vmID string, year, month int, limitBytes uint64) error {
	const q = `
		INSERT INTO bandwidth_usage (vm_id, year, month, limit_bytes)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (vm_id, year, month) DO UPDATE SET limit_bytes = EXCLUDED.limit_bytes`

	_, err := r.db.Exec(ctx, q, vmID, year, month, limitBytes)
	if err != nil {
		return fmt.Errorf("updating bandwidth limit for VM %s: %w", vmID, err)
	}
	return nil
}

// ListThrottled returns all VMs that are currently throttled.
func (r *BandwidthRepository) ListThrottled(ctx context.Context) ([]models.BandwidthUsage, error) {
	now := time.Now().UTC()
	const q = `SELECT ` + bandwidthUsageSelectCols + ` FROM bandwidth_usage WHERE throttled = true AND year = $1 AND month = $2`

	usages, err := ScanRows(ctx, r.db, q, []any{now.Year(), int(now.Month())}, func(rows pgx.Rows) (models.BandwidthUsage, error) {
		return scanBandwidthUsage(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing throttled VMs: %w", err)
	}
	return usages, nil
}

// ListAllUsageForMonth returns all bandwidth usage records for a specific month.
func (r *BandwidthRepository) ListAllUsageForMonth(ctx context.Context, year, month int) ([]models.BandwidthUsage, error) {
	const q = `SELECT ` + bandwidthUsageSelectCols + ` FROM bandwidth_usage WHERE year = $1 AND month = $2`

	usages, err := ScanRows(ctx, r.db, q, []any{year, month}, func(rows pgx.Rows) (models.BandwidthUsage, error) {
		return scanBandwidthUsage(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing bandwidth usage for %d-%02d: %w", year, month, err)
	}
	return usages, nil
}

// ResetMonthCounters resets bandwidth counters for a new billing month.
// This should be called at the start of each month (e.g., 1st at 00:00 UTC).
func (r *BandwidthRepository) ResetMonthCounters(ctx context.Context, year, month int) error {
	now := time.Now().UTC()
	const q = `
		UPDATE bandwidth_usage SET reset_at = $1
		WHERE year = $2 AND month = $3 AND reset_at IS NULL`

	_, err := r.db.Exec(ctx, q, now, year, month)
	if err != nil {
		return fmt.Errorf("resetting month counters for %d-%02d: %w", year, month, err)
	}
	return nil
}

// DeleteForVM removes bandwidth usage records for a VM (for cleanup on VM deletion).
func (r *BandwidthRepository) DeleteForVM(ctx context.Context, vmID string) error {
	const q = `DELETE FROM bandwidth_usage WHERE vm_id = $1`
	_, err := r.db.Exec(ctx, q, vmID)
	if err != nil {
		return fmt.Errorf("deleting bandwidth usage for VM %s: %w", vmID, err)
	}
	return nil
}