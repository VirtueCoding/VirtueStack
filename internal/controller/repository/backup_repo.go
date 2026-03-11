// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// BackupRepository provides database operations for backups and snapshots.
type BackupRepository struct {
	db DB
}

// NewBackupRepository creates a new BackupRepository with the given database connection.
func NewBackupRepository(db DB) *BackupRepository {
	return &BackupRepository{db: db}
}

// BackupListFilter holds filter parameters for listing backups.
type BackupListFilter struct {
	models.PaginationParams
	VMID   *string
	Status *string
	Type   *string // "full" or "incremental"
}

// SnapshotListFilter holds filter parameters for listing snapshots.
type SnapshotListFilter struct {
	models.PaginationParams
	VMID *string
}

// ============================================================================
// Backup Operations
// ============================================================================

// scanBackup scans a single backup row into a models.Backup struct.
func scanBackup(row pgx.Row) (models.Backup, error) {
	var b models.Backup
	err := row.Scan(
		&b.ID, &b.VMID, &b.Type, &b.RBDSnapshot,
		&b.DiffFromSnapshot, &b.StoragePath, &b.SizeBytes,
		&b.Status, &b.CreatedAt, &b.ExpiresAt,
	)
	return b, err
}

const backupSelectCols = `
	id, vm_id, type, rbd_snapshot,
	diff_from_snapshot, storage_path, size_bytes,
	status, created_at, expires_at`

// CreateBackup inserts a new backup record into the database.
// The backup's ID and CreatedAt are populated by the database.
func (r *BackupRepository) CreateBackup(ctx context.Context, backup *models.Backup) error {
	const q = `
		INSERT INTO backups (
			vm_id, type, rbd_snapshot, diff_from_snapshot,
			storage_path, size_bytes, status, expires_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING ` + backupSelectCols

	row := r.db.QueryRow(ctx, q,
		backup.VMID, backup.Type, backup.RBDSnapshot, backup.DiffFromSnapshot,
		backup.StoragePath, backup.SizeBytes, backup.Status, backup.ExpiresAt,
	)
	created, err := scanBackup(row)
	if err != nil {
		return fmt.Errorf("creating backup: %w", err)
	}
	*backup = created
	return nil
}

// GetBackupByID returns a backup by its UUID. Returns ErrNotFound if no backup matches.
func (r *BackupRepository) GetBackupByID(ctx context.Context, id string) (*models.Backup, error) {
	const q = `SELECT ` + backupSelectCols + ` FROM backups WHERE id = $1`
	backup, err := ScanRow(ctx, r.db, q, []any{id}, scanBackup)
	if err != nil {
		return nil, fmt.Errorf("getting backup %s: %w", id, err)
	}
	return &backup, nil
}

// ListBackups returns a paginated list of backups with optional filters and total count.
func (r *BackupRepository) ListBackups(ctx context.Context, filter BackupListFilter) ([]models.Backup, int, error) {
	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if filter.VMID != nil {
		where = append(where, fmt.Sprintf("vm_id = $%d", idx))
		args = append(args, *filter.VMID)
		idx++
	}
	if filter.Status != nil {
		where = append(where, fmt.Sprintf("status = $%d", idx))
		args = append(args, *filter.Status)
		idx++
	}
	if filter.Type != nil {
		where = append(where, fmt.Sprintf("type = $%d", idx))
		args = append(args, *filter.Type)
		idx++
	}

	clause := strings.Join(where, " AND ")
	countQ := "SELECT COUNT(*) FROM backups WHERE " + clause
	total, err := CountRows(ctx, r.db, countQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting backups: %w", err)
	}

	limit := filter.Limit()
	offset := filter.Offset()
	listQ := fmt.Sprintf(
		"SELECT %s FROM backups WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		backupSelectCols, clause, idx, idx+1,
	)
	args = append(args, limit, offset)

	backups, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.Backup, error) {
		return scanBackup(rows)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing backups: %w", err)
	}
	return backups, total, nil
}

// ListBackupsByVM returns all backups for a specific VM.
func (r *BackupRepository) ListBackupsByVM(ctx context.Context, vmID string) ([]models.Backup, error) {
	const q = `SELECT ` + backupSelectCols + ` FROM backups WHERE vm_id = $1 ORDER BY created_at DESC`
	backups, err := ScanRows(ctx, r.db, q, []any{vmID}, func(rows pgx.Rows) (models.Backup, error) {
		return scanBackup(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing backups for VM %s: %w", vmID, err)
	}
	return backups, nil
}

// UpdateBackupStatus updates the status field of a backup.
func (r *BackupRepository) UpdateBackupStatus(ctx context.Context, id, status string) error {
	const q = `UPDATE backups SET status = $1 WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, status, id)
	if err != nil {
		return fmt.Errorf("updating backup %s status: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating backup %s status: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// SetBackupExpiration sets the expiration timestamp for a backup.
func (r *BackupRepository) SetBackupExpiration(ctx context.Context, id string, expiresAt time.Time) error {
	const q = `UPDATE backups SET expires_at = $1 WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, expiresAt, id)
	if err != nil {
		return fmt.Errorf("setting backup %s expiration: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("setting backup %s expiration: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// DeleteBackup permanently removes a backup record from the database.
func (r *BackupRepository) DeleteBackup(ctx context.Context, id string) error {
	const q = `DELETE FROM backups WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting backup %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting backup %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// ============================================================================
// Snapshot Operations
// ============================================================================

// scanSnapshot scans a single snapshot row into a models.Snapshot struct.
func scanSnapshot(row pgx.Row) (models.Snapshot, error) {
	var s models.Snapshot
	err := row.Scan(
		&s.ID, &s.VMID, &s.Name, &s.RBDSnapshot,
		&s.SizeBytes, &s.CreatedAt,
	)
	return s, err
}

const snapshotSelectCols = `
	id, vm_id, name, rbd_snapshot,
	size_bytes, created_at`

// CreateSnapshot inserts a new snapshot record into the database.
// The snapshot's ID and CreatedAt are populated by the database.
func (r *BackupRepository) CreateSnapshot(ctx context.Context, snapshot *models.Snapshot) error {
	const q = `
		INSERT INTO snapshots (
			vm_id, name, rbd_snapshot, size_bytes
		) VALUES ($1,$2,$3,$4)
		RETURNING ` + snapshotSelectCols

	row := r.db.QueryRow(ctx, q,
		snapshot.VMID, snapshot.Name, snapshot.RBDSnapshot, snapshot.SizeBytes,
	)
	created, err := scanSnapshot(row)
	if err != nil {
		return fmt.Errorf("creating snapshot: %w", err)
	}
	*snapshot = created
	return nil
}

// GetSnapshotByID returns a snapshot by its UUID. Returns ErrNotFound if no snapshot matches.
func (r *BackupRepository) GetSnapshotByID(ctx context.Context, id string) (*models.Snapshot, error) {
	const q = `SELECT ` + snapshotSelectCols + ` FROM snapshots WHERE id = $1`
	snapshot, err := ScanRow(ctx, r.db, q, []any{id}, scanSnapshot)
	if err != nil {
		return nil, fmt.Errorf("getting snapshot %s: %w", id, err)
	}
	return &snapshot, nil
}

// ListSnapshots returns a paginated list of snapshots with optional filters and total count.
func (r *BackupRepository) ListSnapshots(ctx context.Context, filter SnapshotListFilter) ([]models.Snapshot, int, error) {
	where := "1=1"
	args := []any{}
	idx := 1

	if filter.VMID != nil {
		where += fmt.Sprintf(" AND vm_id = $%d", idx)
		args = append(args, *filter.VMID)
		idx++
	}

	total, err := CountRows(ctx, r.db, "SELECT COUNT(*) FROM snapshots WHERE "+where, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting snapshots: %w", err)
	}

	limit := filter.Limit()
	offset := filter.Offset()
	listQ := fmt.Sprintf(
		"SELECT %s FROM snapshots WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		snapshotSelectCols, where, idx, idx+1,
	)
	args = append(args, limit, offset)

	snapshots, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.Snapshot, error) {
		return scanSnapshot(rows)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing snapshots: %w", err)
	}
	return snapshots, total, nil
}

// ListSnapshotsByVM returns all snapshots for a specific VM.
func (r *BackupRepository) ListSnapshotsByVM(ctx context.Context, vmID string) ([]models.Snapshot, error) {
	const q = `SELECT ` + snapshotSelectCols + ` FROM snapshots WHERE vm_id = $1 ORDER BY created_at DESC`
	snapshots, err := ScanRows(ctx, r.db, q, []any{vmID}, func(rows pgx.Rows) (models.Snapshot, error) {
		return scanSnapshot(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing snapshots for VM %s: %w", vmID, err)
	}
	return snapshots, nil
}

// DeleteSnapshot permanently removes a snapshot record from the database.
func (r *BackupRepository) DeleteSnapshot(ctx context.Context, id string) error {
	const q = `DELETE FROM snapshots WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting snapshot %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting snapshot %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// ============================================================================
// Scheduler Support
// ============================================================================

// HasBackupInMonth checks if a VM has a completed backup within the specified month.
// This is used by the backup scheduler to determine if a monthly backup is needed.
func (r *BackupRepository) HasBackupInMonth(ctx context.Context, vmID string, year, month int) (bool, error) {
	// Calculate the start and end of the month
	startOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	endOfMonth := startOfMonth.AddDate(0, 1, 0) // First day of next month

	const q = `
		SELECT COUNT(*) > 0 FROM backups
		WHERE vm_id = $1
		  AND status = $2
		  AND created_at >= $3
		  AND created_at < $4`

	var exists bool
	err := r.db.QueryRow(ctx, q, vmID, models.BackupStatusCompleted, startOfMonth, endOfMonth).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking backup existence for VM %s in %d-%d: %w", vmID, year, month, err)
	}
	return exists, nil
}