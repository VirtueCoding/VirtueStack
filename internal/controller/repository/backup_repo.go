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

type BackupScheduleListFilter struct {
	models.PaginationParams
	VMID       *string
	CustomerID *string
	Active     *bool
}

// ============================================================================
// Backup Operations
// ============================================================================

// scanBackup scans a single backup row into a models.Backup struct.
func scanBackup(row pgx.Row) (models.Backup, error) {
	var b models.Backup
	err := row.Scan(
		&b.ID, &b.VMID, &b.Type, &b.StorageBackend, &b.RBDSnapshot,
		&b.FilePath, &b.DiffFromSnapshot, &b.StoragePath, &b.SizeBytes,
		&b.Status, &b.CreatedAt, &b.ExpiresAt,
	)
	return b, err
}

const backupSelectCols = `
	id, vm_id, type, storage_backend, rbd_snapshot,
	file_path, diff_from_snapshot, storage_path, size_bytes,
	status, created_at, expires_at`

// CreateBackup inserts a new backup record into the database.
// The backup's ID and CreatedAt are populated by the database.
func (r *BackupRepository) CreateBackup(ctx context.Context, backup *models.Backup) error {
	const q = `
		INSERT INTO backups (
			vm_id, type, storage_backend, rbd_snapshot, file_path,
			diff_from_snapshot, storage_path, size_bytes, status, expires_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING ` + backupSelectCols

	row := r.db.QueryRow(ctx, q,
		backup.VMID, backup.Type, backup.StorageBackend, backup.RBDSnapshot, backup.FilePath,
		backup.DiffFromSnapshot, backup.StoragePath, backup.SizeBytes, backup.Status, backup.ExpiresAt,
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

func (r *BackupRepository) ListBackupsByCustomer(ctx context.Context, customerID string, filter BackupListFilter) ([]models.Backup, int, error) {
	where := []string{"v.customer_id = $1"}
	args := []any{customerID}
	idx := 2

	if filter.VMID != nil {
		where = append(where, fmt.Sprintf("b.vm_id = $%d", idx))
		args = append(args, *filter.VMID)
		idx++
	}
	if filter.Status != nil {
		where = append(where, fmt.Sprintf("b.status = $%d", idx))
		args = append(args, *filter.Status)
		idx++
	}
	if filter.Type != nil {
		where = append(where, fmt.Sprintf("b.type = $%d", idx))
		args = append(args, *filter.Type)
		idx++
	}

	clause := strings.Join(where, " AND ")
	countQ := "SELECT COUNT(*) FROM backups b JOIN vms v ON v.id = b.vm_id WHERE " + clause
	total, err := CountRows(ctx, r.db, countQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting backups by customer: %w", err)
	}

	limit := filter.Limit()
	offset := filter.Offset()
	listQ := fmt.Sprintf(
		"SELECT %s FROM backups b JOIN vms v ON v.id = b.vm_id WHERE %s ORDER BY b.created_at DESC LIMIT $%d OFFSET $%d",
		prefixSelectCols(backupSelectCols, "b"), clause, idx, idx+1,
	)
	args = append(args, limit, offset)

	backups, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.Backup, error) {
		return scanBackup(rows)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing backups by customer: %w", err)
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

// CountBackupsByVM returns the number of backups for a specific VM.
func (r *BackupRepository) CountBackupsByVM(ctx context.Context, vmID string) (int, error) {
	const q = `SELECT COUNT(*) FROM backups WHERE vm_id = $1`
	count, err := CountRows(ctx, r.db, q, vmID)
	if err != nil {
		return 0, fmt.Errorf("counting backups for VM %s: %w", vmID, err)
	}
	return count, nil
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

func (r *BackupRepository) ListExpiredBackups(ctx context.Context, now time.Time) ([]models.Backup, error) {
	const q = `SELECT ` + backupSelectCols + ` FROM backups WHERE expires_at IS NOT NULL AND expires_at <= $1 ORDER BY expires_at ASC`
	backups, err := ScanRows(ctx, r.db, q, []any{now}, func(rows pgx.Rows) (models.Backup, error) {
		return scanBackup(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing expired backups: %w", err)
	}
	return backups, nil
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

func (r *BackupRepository) ListSnapshotsByCustomer(ctx context.Context, customerID string, filter SnapshotListFilter) ([]models.Snapshot, int, error) {
	where := []string{"v.customer_id = $1"}
	args := []any{customerID}
	idx := 2

	if filter.VMID != nil {
		where = append(where, fmt.Sprintf("s.vm_id = $%d", idx))
		args = append(args, *filter.VMID)
		idx++
	}

	clause := strings.Join(where, " AND ")
	countQ := "SELECT COUNT(*) FROM snapshots s JOIN vms v ON v.id = s.vm_id WHERE " + clause
	total, err := CountRows(ctx, r.db, countQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting snapshots by customer: %w", err)
	}

	limit := filter.Limit()
	offset := filter.Offset()
	listQ := fmt.Sprintf(
		"SELECT %s FROM snapshots s JOIN vms v ON v.id = s.vm_id WHERE %s ORDER BY s.created_at DESC LIMIT $%d OFFSET $%d",
		prefixSelectCols(snapshotSelectCols, "s"), clause, idx, idx+1,
	)
	args = append(args, limit, offset)

	snapshots, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.Snapshot, error) {
		return scanSnapshot(rows)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing snapshots by customer: %w", err)
	}
	return snapshots, total, nil
}

func prefixSelectCols(cols, alias string) string {
	parts := strings.Split(cols, ",")
	for i := range parts {
		parts[i] = alias + "." + strings.TrimSpace(parts[i])
	}
	return strings.Join(parts, ", ")
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

// CountSnapshotsByVM returns the number of snapshots for a specific VM.
func (r *BackupRepository) CountSnapshotsByVM(ctx context.Context, vmID string) (int, error) {
	const q = `SELECT COUNT(*) FROM snapshots WHERE vm_id = $1`
	count, err := CountRows(ctx, r.db, q, vmID)
	if err != nil {
		return 0, fmt.Errorf("counting snapshots for VM %s: %w", vmID, err)
	}
	return count, nil
}

func (r *BackupRepository) UpdateSnapshot(ctx context.Context, snapshot *models.Snapshot) error {
	const q = `
		UPDATE snapshots SET
			vm_id = $1,
			name = $2,
			rbd_snapshot = $3,
			size_bytes = $4
		WHERE id = $5
		RETURNING ` + snapshotSelectCols

	row := r.db.QueryRow(ctx, q,
		snapshot.VMID, snapshot.Name, snapshot.RBDSnapshot, snapshot.SizeBytes, snapshot.ID,
	)
	updated, err := scanSnapshot(row)
	if err != nil {
		return fmt.Errorf("updating snapshot %s: %w", snapshot.ID, err)
	}
	*snapshot = updated
	return nil
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

const backupScheduleSelectCols = `
	id, vm_id, customer_id, frequency, retention_count,
	active, next_run_at, created_at`

func scanBackupSchedule(row pgx.Row) (models.BackupSchedule, error) {
	var s models.BackupSchedule
	err := row.Scan(
		&s.ID, &s.VMID, &s.CustomerID, &s.Frequency, &s.RetentionCount,
		&s.Active, &s.NextRunAt, &s.CreatedAt,
	)
	return s, err
}

func (r *BackupRepository) CreateBackupSchedule(ctx context.Context, schedule *models.BackupSchedule) error {
	const q = `
		INSERT INTO backup_schedules (
			vm_id, customer_id, frequency, retention_count, active, next_run_at
		) VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING ` + backupScheduleSelectCols

	row := r.db.QueryRow(ctx, q,
		schedule.VMID, schedule.CustomerID, schedule.Frequency,
		schedule.RetentionCount, schedule.Active, schedule.NextRunAt,
	)
	created, err := scanBackupSchedule(row)
	if err != nil {
		return fmt.Errorf("creating backup schedule: %w", err)
	}
	*schedule = created
	return nil
}

func (r *BackupRepository) GetBackupScheduleByID(ctx context.Context, id string) (*models.BackupSchedule, error) {
	const q = `SELECT ` + backupScheduleSelectCols + ` FROM backup_schedules WHERE id = $1`
	schedule, err := ScanRow(ctx, r.db, q, []any{id}, scanBackupSchedule)
	if err != nil {
		return nil, fmt.Errorf("getting backup schedule %s: %w", id, err)
	}
	return &schedule, nil
}

func (r *BackupRepository) ListBackupSchedules(ctx context.Context, filter BackupScheduleListFilter) ([]models.BackupSchedule, int, error) {
	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if filter.VMID != nil {
		where = append(where, fmt.Sprintf("vm_id = $%d", idx))
		args = append(args, *filter.VMID)
		idx++
	}
	if filter.CustomerID != nil {
		where = append(where, fmt.Sprintf("customer_id = $%d", idx))
		args = append(args, *filter.CustomerID)
		idx++
	}
	if filter.Active != nil {
		where = append(where, fmt.Sprintf("active = $%d", idx))
		args = append(args, *filter.Active)
		idx++
	}

	clause := strings.Join(where, " AND ")
	countQ := "SELECT COUNT(*) FROM backup_schedules WHERE " + clause
	total, err := CountRows(ctx, r.db, countQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting backup schedules: %w", err)
	}

	limit := filter.Limit()
	offset := filter.Offset()
	listQ := fmt.Sprintf(
		"SELECT %s FROM backup_schedules WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		backupScheduleSelectCols, clause, idx, idx+1,
	)
	args = append(args, limit, offset)

	schedules, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.BackupSchedule, error) {
		return scanBackupSchedule(rows)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing backup schedules: %w", err)
	}

	return schedules, total, nil
}

func (r *BackupRepository) DeleteBackupSchedule(ctx context.Context, id string) error {
	const q = `DELETE FROM backup_schedules WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting backup schedule %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting backup schedule %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

func (r *BackupRepository) UpdateBackupScheduleActive(ctx context.Context, id string, active bool) error {
	const q = `UPDATE backup_schedules SET active = $1 WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, active, id)
	if err != nil {
		return fmt.Errorf("updating backup schedule %s active: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating backup schedule %s active: %w", id, ErrNoRowsAffected)
	}
	return nil
}

func (r *BackupRepository) UpdateBackupScheduleFrequency(ctx context.Context, id, frequency string, nextRunAt time.Time) error {
	const q = `UPDATE backup_schedules SET frequency = $1, next_run_at = $2 WHERE id = $3`
	tag, err := r.db.Exec(ctx, q, frequency, nextRunAt, id)
	if err != nil {
		return fmt.Errorf("updating backup schedule %s frequency: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating backup schedule %s frequency: %w", id, ErrNoRowsAffected)
	}
	return nil
}
