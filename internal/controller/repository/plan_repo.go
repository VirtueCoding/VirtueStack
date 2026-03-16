// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// PlanRepository provides database operations for service plans.
type PlanRepository struct {
	db DB
}

// NewPlanRepository creates a new PlanRepository with the given database connection.
func NewPlanRepository(db DB) *PlanRepository {
	return &PlanRepository{db: db}
}

// PlanListFilter holds filter options for listing plans.
type PlanListFilter struct {
	models.PaginationParams
	IsActive *bool
}

// scanPlan scans a single plan row into a models.Plan struct.
func scanPlan(row pgx.Row) (models.Plan, error) {
	var p models.Plan
	err := row.Scan(
		&p.ID, &p.Name, &p.Slug, &p.VCPU, &p.MemoryMB,
		&p.DiskGB, &p.BandwidthLimitGB, &p.PortSpeedMbps,
		&p.PriceMonthly, &p.PriceHourly, &p.IsActive,
		&p.SortOrder, &p.CreatedAt, &p.UpdatedAt,
		&p.SnapshotLimit, &p.BackupLimit, &p.ISOUploadLimit,
	)
	return p, err
}

const planSelectCols = `
	id, name, slug, vcpu, memory_mb,
	disk_gb, bandwidth_limit_gb, port_speed_mbps,
	price_monthly, price_hourly, is_active,
	sort_order, created_at, updated_at,
	snapshot_limit, backup_limit, iso_upload_limit`

// Create inserts a new plan record into the database.
// The plan's ID, CreatedAt, and UpdatedAt are populated by the database.
func (r *PlanRepository) Create(ctx context.Context, plan *models.Plan) error {
	const q = `
		INSERT INTO plans (
			name, slug, vcpu, memory_mb, disk_gb,
			bandwidth_limit_gb, port_speed_mbps,
			price_monthly, price_hourly, is_active, sort_order,
			snapshot_limit, backup_limit, iso_upload_limit
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING ` + planSelectCols

	row := r.db.QueryRow(ctx, q,
		plan.Name, plan.Slug, plan.VCPU, plan.MemoryMB, plan.DiskGB,
		plan.BandwidthLimitGB, plan.PortSpeedMbps,
		plan.PriceMonthly, plan.PriceHourly, plan.IsActive, plan.SortOrder,
		plan.SnapshotLimit, plan.BackupLimit, plan.ISOUploadLimit,
	)
	created, err := scanPlan(row)
	if err != nil {
		return fmt.Errorf("creating plan: %w", err)
	}
	*plan = created
	return nil
}

// GetByID returns a plan by its UUID. Returns ErrNotFound if no plan matches.
func (r *PlanRepository) GetByID(ctx context.Context, id string) (*models.Plan, error) {
	const q = `SELECT ` + planSelectCols + ` FROM plans WHERE id = $1`
	plan, err := ScanRow(ctx, r.db, q, []any{id}, scanPlan)
	if err != nil {
		return nil, fmt.Errorf("getting plan %s: %w", id, err)
	}
	return &plan, nil
}

// GetByName returns a plan by its name. Returns ErrNotFound if no plan matches.
func (r *PlanRepository) GetByName(ctx context.Context, name string) (*models.Plan, error) {
	const q = `SELECT ` + planSelectCols + ` FROM plans WHERE name = $1`
	plan, err := ScanRow(ctx, r.db, q, []any{name}, scanPlan)
	if err != nil {
		return nil, fmt.Errorf("getting plan by name %s: %w", name, err)
	}
	return &plan, nil
}

// GetBySlug returns a plan by its slug. Returns ErrNotFound if no plan matches.
func (r *PlanRepository) GetBySlug(ctx context.Context, slug string) (*models.Plan, error) {
	const q = `SELECT ` + planSelectCols + ` FROM plans WHERE slug = $1`
	plan, err := ScanRow(ctx, r.db, q, []any{slug}, scanPlan)
	if err != nil {
		return nil, fmt.Errorf("getting plan by slug %s: %w", slug, err)
	}
	return &plan, nil
}

// List returns a paginated list of plans with optional filters and total count.
func (r *PlanRepository) List(ctx context.Context, filter PlanListFilter) ([]models.Plan, int, error) {
	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if filter.IsActive != nil {
		where = append(where, fmt.Sprintf("is_active = $%d", idx))
		args = append(args, *filter.IsActive)
		idx++
	}

	clause := strings.Join(where, " AND ")
	countQ := "SELECT COUNT(*) FROM plans WHERE " + clause
	total, err := CountRows(ctx, r.db, countQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting plans: %w", err)
	}

	limit := filter.Limit()
	offset := filter.Offset()
	listQ := fmt.Sprintf(
		"SELECT %s FROM plans WHERE %s ORDER BY sort_order ASC, name ASC LIMIT $%d OFFSET $%d",
		planSelectCols, clause, idx, idx+1,
	)
	args = append(args, limit, offset)

	plans, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.Plan, error) {
		return scanPlan(rows)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing plans: %w", err)
	}
	return plans, total, nil
}

// ListActive returns all active plans ordered by sort_order.
func (r *PlanRepository) ListActive(ctx context.Context) ([]models.Plan, error) {
	const q = `SELECT ` + planSelectCols + ` FROM plans WHERE is_active = true ORDER BY sort_order ASC, name ASC`
	plans, err := ScanRows(ctx, r.db, q, nil, func(rows pgx.Rows) (models.Plan, error) {
		return scanPlan(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing active plans: %w", err)
	}
	return plans, nil
}

// Update updates all editable fields of a plan.
// Returns ErrNoRowsAffected if no plan was found to update.
func (r *PlanRepository) Update(ctx context.Context, plan *models.Plan) error {
	const q = `
		UPDATE plans SET
			name = $1, slug = $2, vcpu = $3, memory_mb = $4, disk_gb = $5,
			bandwidth_limit_gb = $6, port_speed_mbps = $7,
			price_monthly = $8, price_hourly = $9, is_active = $10, sort_order = $11,
			snapshot_limit = $12, backup_limit = $13, iso_upload_limit = $14,
			updated_at = NOW()
		WHERE id = $15
		RETURNING ` + planSelectCols

	row := r.db.QueryRow(ctx, q,
		plan.Name, plan.Slug, plan.VCPU, plan.MemoryMB, plan.DiskGB,
		plan.BandwidthLimitGB, plan.PortSpeedMbps,
		plan.PriceMonthly, plan.PriceHourly, plan.IsActive, plan.SortOrder,
		plan.SnapshotLimit, plan.BackupLimit, plan.ISOUploadLimit,
		plan.ID,
	)
	updated, err := scanPlan(row)
	if err != nil {
		return fmt.Errorf("updating plan %s: %w", plan.ID, err)
	}
	*plan = updated
	return nil
}

// UpdateActive updates the is_active status of a plan.
// Returns ErrNoRowsAffected if no plan was found to update.
func (r *PlanRepository) UpdateActive(ctx context.Context, id string, isActive bool) error {
	const q = `UPDATE plans SET is_active = $1, updated_at = NOW() WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, isActive, id)
	if err != nil {
		return fmt.Errorf("updating plan %s active status: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating plan %s active status: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// UpdateSortOrder updates the sort_order of a plan.
// Returns ErrNoRowsAffected if no plan was found to update.
func (r *PlanRepository) UpdateSortOrder(ctx context.Context, id string, sortOrder int) error {
	const q = `UPDATE plans SET sort_order = $1, updated_at = NOW() WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, sortOrder, id)
	if err != nil {
		return fmt.Errorf("updating plan %s sort order: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating plan %s sort order: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// Delete permanently removes a plan from the database.
// Note: Plans with referencing VMs will fail due to FK RESTRICT constraint.
// Returns ErrNoRowsAffected if no plan was found to delete.
func (r *PlanRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM plans WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting plan %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting plan %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}
