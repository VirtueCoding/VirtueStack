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

// AdminBackupScheduleRepository provides database operations for admin backup schedules.
type AdminBackupScheduleRepository struct {
	db DB
}

// NewAdminBackupScheduleRepository creates a new AdminBackupScheduleRepository with the given database connection.
func NewAdminBackupScheduleRepository(db DB) *AdminBackupScheduleRepository {
	return &AdminBackupScheduleRepository{db: db}
}

// AdminBackupScheduleListFilter holds filter parameters for listing admin backup schedules.
type AdminBackupScheduleListFilter struct {
	models.PaginationParams
	Active *bool
}

const adminBackupScheduleSelectCols = `
	id, name, description, frequency, retention_count,
	target_all, target_plan_ids, target_node_ids, target_customer_ids,
	active, next_run_at, last_run_at, created_by, created_at, updated_at`

func scanAdminBackupSchedule(row pgx.Row) (models.AdminBackupSchedule, error) {
	var s models.AdminBackupSchedule
	err := row.Scan(
		&s.ID, &s.Name, &s.Description, &s.Frequency, &s.RetentionCount,
		&s.TargetAll, &s.TargetPlanIDs, &s.TargetNodeIDs, &s.TargetCustomerIDs,
		&s.Active, &s.NextRunAt, &s.LastRunAt, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
	)
	return s, err
}

// Create inserts a new admin backup schedule into the database.
func (r *AdminBackupScheduleRepository) Create(ctx context.Context, schedule *models.AdminBackupSchedule) error {
	const q = `
		INSERT INTO admin_backup_schedules (
			name, description, frequency, retention_count,
			target_all, target_plan_ids, target_node_ids, target_customer_ids,
			active, next_run_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING ` + adminBackupScheduleSelectCols

	row := r.db.QueryRow(ctx, q,
		schedule.Name, schedule.Description, schedule.Frequency, schedule.RetentionCount,
		schedule.TargetAll, schedule.TargetPlanIDs, schedule.TargetNodeIDs, schedule.TargetCustomerIDs,
		schedule.Active, schedule.NextRunAt, schedule.CreatedBy,
	)
	created, err := scanAdminBackupSchedule(row)
	if err != nil {
		return fmt.Errorf("creating admin backup schedule: %w", err)
	}
	*schedule = created
	return nil
}

// GetByID returns an admin backup schedule by its UUID. Returns ErrNotFound if no schedule matches.
func (r *AdminBackupScheduleRepository) GetByID(ctx context.Context, id string) (*models.AdminBackupSchedule, error) {
	const q = `SELECT ` + adminBackupScheduleSelectCols + ` FROM admin_backup_schedules WHERE id = $1`
	schedule, err := ScanRow(ctx, r.db, q, []any{id}, scanAdminBackupSchedule)
	if err != nil {
		return nil, fmt.Errorf("getting admin backup schedule %s: %w", id, err)
	}
	return &schedule, nil
}

// List returns a paginated list of admin backup schedules with optional filters and total count.
func (r *AdminBackupScheduleRepository) List(ctx context.Context, filter AdminBackupScheduleListFilter) ([]models.AdminBackupSchedule, int, error) {
	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if filter.Active != nil {
		where = append(where, fmt.Sprintf("active = $%d", idx))
		args = append(args, *filter.Active)
		idx++
	}

	clause := strings.Join(where, " AND ")

	if filter.IsCursorBased() {
		cursor := filter.DecodeCursor()
		if cursor.LastID != "" {
			clause += fmt.Sprintf(" AND id < $%d", idx)
			args = append(args, cursor.LastID)
			idx++
		}
		listQ := fmt.Sprintf(
			"SELECT %s FROM admin_backup_schedules WHERE %s ORDER BY id DESC LIMIT $%d",
			adminBackupScheduleSelectCols, clause, idx,
		)
		args = append(args, filter.PerPage+1)
		schedules, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.AdminBackupSchedule, error) {
			return scanAdminBackupSchedule(rows)
		})
		if err != nil {
			return nil, 0, fmt.Errorf("listing admin backup schedules: %w", err)
		}
		return schedules, 0, nil
	}

	countQ := "SELECT COUNT(*) FROM admin_backup_schedules WHERE " + clause
	total, err := CountRows(ctx, r.db, countQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting admin backup schedules: %w", err)
	}

	limit := filter.Limit()
	offset := filter.Offset()
	listQ := fmt.Sprintf(
		"SELECT %s FROM admin_backup_schedules WHERE %s ORDER BY next_run_at ASC LIMIT $%d OFFSET $%d",
		adminBackupScheduleSelectCols, clause, idx, idx+1,
	)
	args = append(args, limit, offset)

	schedules, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.AdminBackupSchedule, error) {
		return scanAdminBackupSchedule(rows)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing admin backup schedules: %w", err)
	}

	return schedules, total, nil
}

// ListDueSchedules returns all active schedules that are due to run.
func (r *AdminBackupScheduleRepository) ListDueSchedules(ctx context.Context, now time.Time) ([]models.AdminBackupSchedule, error) {
	const q = `
		SELECT ` + adminBackupScheduleSelectCols + `
		FROM admin_backup_schedules
		WHERE active = TRUE AND next_run_at <= $1
		ORDER BY next_run_at ASC`

	schedules, err := ScanRows(ctx, r.db, q, []any{now}, func(rows pgx.Rows) (models.AdminBackupSchedule, error) {
		return scanAdminBackupSchedule(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing due admin backup schedules: %w", err)
	}
	return schedules, nil
}

// Update updates an existing admin backup schedule.
func (r *AdminBackupScheduleRepository) Update(ctx context.Context, schedule *models.AdminBackupSchedule) error {
	const q = `
		UPDATE admin_backup_schedules SET
			name = $1, description = $2, frequency = $3, retention_count = $4,
			target_all = $5, target_plan_ids = $6, target_node_ids = $7, target_customer_ids = $8,
			active = $9, next_run_at = $10, updated_at = NOW()
		WHERE id = $11
		RETURNING ` + adminBackupScheduleSelectCols

	row := r.db.QueryRow(ctx, q,
		schedule.Name, schedule.Description, schedule.Frequency, schedule.RetentionCount,
		schedule.TargetAll, schedule.TargetPlanIDs, schedule.TargetNodeIDs, schedule.TargetCustomerIDs,
		schedule.Active, schedule.NextRunAt, schedule.ID,
	)
	updated, err := scanAdminBackupSchedule(row)
	if err != nil {
		return fmt.Errorf("updating admin backup schedule %s: %w", schedule.ID, err)
	}
	*schedule = updated
	return nil
}

// UpdateNextRunAt updates the next run time and last run time for a schedule.
func (r *AdminBackupScheduleRepository) UpdateNextRunAt(ctx context.Context, id string, nextRunAt, lastRunAt time.Time) error {
	const q = `
		UPDATE admin_backup_schedules
		SET next_run_at = $1, last_run_at = $2, updated_at = NOW()
		WHERE id = $3`
	tag, err := r.db.Exec(ctx, q, nextRunAt, lastRunAt, id)
	if err != nil {
		return fmt.Errorf("updating admin backup schedule %s next run time: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating admin backup schedule %s next run time: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// Delete permanently removes an admin backup schedule from the database.
func (r *AdminBackupScheduleRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM admin_backup_schedules WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting admin backup schedule %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting admin backup schedule %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// CountByStorageBackend returns the count of active admin backup schedules that
// could affect VMs using the given storage backend. A schedule is counted if:
// - It targets all VMs (target_all = true) and the storage backend has assigned nodes
// - It targets specific nodes that have this storage backend assigned
// - It targets plans whose VMs use this storage backend (via node_assignment)
func (r *AdminBackupScheduleRepository) CountByStorageBackend(ctx context.Context, storageBackendID string) (int, error) {
	const q = `
		SELECT COUNT(DISTINCT abs.id)
		FROM admin_backup_schedules abs
		WHERE abs.active = true
		  AND (
			-- Schedules targeting all VMs - check if any nodes have this storage backend
			(abs.target_all = true AND EXISTS (
				SELECT 1 FROM node_storage ns WHERE ns.storage_backend_id = $1
			))
			OR
			-- Schedules targeting specific nodes with this storage backend
			(SELECT COUNT(*) FROM jsonb_array_elements_text(abs.target_node_ids) AS node_id
			 WHERE node_id IN (
				SELECT ns.node_id::text FROM node_storage ns WHERE ns.storage_backend_id = $1
			 )) > 0
		  )
	`
	var count int
	err := r.db.QueryRow(ctx, q, storageBackendID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting admin backup schedules by storage backend %s: %w", storageBackendID, err)
	}
	return count, nil
}