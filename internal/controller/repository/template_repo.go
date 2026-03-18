// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// TemplateRepository provides database operations for OS templates.
type TemplateRepository struct {
	db DB
}

// NewTemplateRepository creates a new TemplateRepository with the given database connection.
func NewTemplateRepository(db DB) *TemplateRepository {
	return &TemplateRepository{db: db}
}

// TemplateListFilter holds filter options for listing templates.
type TemplateListFilter struct {
	models.PaginationParams
	IsActive         *bool
	OSFamily         *string
	SupportsCloudInit *bool
}

// scanTemplate scans a single template row into a models.Template struct.
func scanTemplate(row pgx.Row) (models.Template, error) {
	var t models.Template
	err := row.Scan(
		&t.ID, &t.Name, &t.OSFamily, &t.OSVersion,
		&t.RBDImage, &t.RBDSnapshot, &t.MinDiskGB,
		&t.SupportsCloudInit, &t.IsActive, &t.SortOrder,
		&t.Version, &t.Description, &t.StorageBackend, &t.FilePath,
		&t.CreatedAt, &t.UpdatedAt,
	)
	return t, err
}

const templateSelectCols = `
	id, name, os_family, os_version,
	rbd_image, rbd_snapshot, min_disk_gb,
	supports_cloudinit, is_active, sort_order,
	version, description, storage_backend, file_path,
	created_at, updated_at`

// Create inserts a new template record into the database.
// The template's ID, CreatedAt, and UpdatedAt are populated by the database.
func (r *TemplateRepository) Create(ctx context.Context, template *models.Template) error {
	const q = `
		INSERT INTO templates (
			name, os_family, os_version, rbd_image, rbd_snapshot,
			min_disk_gb, supports_cloudinit, is_active, sort_order, description,
			storage_backend, file_path
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING ` + templateSelectCols

	row := r.db.QueryRow(ctx, q,
		template.Name, template.OSFamily, template.OSVersion,
		template.RBDImage, template.RBDSnapshot,
		template.MinDiskGB, template.SupportsCloudInit,
		template.IsActive, template.SortOrder, template.Description,
		template.StorageBackend, template.FilePath,
	)
	created, err := scanTemplate(row)
	if err != nil {
		return fmt.Errorf("creating template: %w", err)
	}
	*template = created
	return nil
}

// GetByID returns a template by its UUID. Returns ErrNotFound if no template matches.
func (r *TemplateRepository) GetByID(ctx context.Context, id string) (*models.Template, error) {
	const q = `SELECT ` + templateSelectCols + ` FROM templates WHERE id = $1`
	template, err := ScanRow(ctx, r.db, q, []any{id}, scanTemplate)
	if err != nil {
		return nil, fmt.Errorf("getting template %s: %w", id, err)
	}
	return &template, nil
}

// GetByName returns a template by its name. Returns ErrNotFound if no template matches.
func (r *TemplateRepository) GetByName(ctx context.Context, name string) (*models.Template, error) {
	const q = `SELECT ` + templateSelectCols + ` FROM templates WHERE name = $1`
	template, err := ScanRow(ctx, r.db, q, []any{name}, scanTemplate)
	if err != nil {
		return nil, fmt.Errorf("getting template by name %s: %w", name, err)
	}
	return &template, nil
}

// List returns a paginated list of templates with optional filters and total count.
func (r *TemplateRepository) List(ctx context.Context, filter TemplateListFilter) ([]models.Template, int, error) {
	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if filter.IsActive != nil {
		where = append(where, fmt.Sprintf("is_active = $%d", idx))
		args = append(args, *filter.IsActive)
		idx++
	}
	if filter.OSFamily != nil {
		where = append(where, fmt.Sprintf("os_family = $%d", idx))
		args = append(args, *filter.OSFamily)
		idx++
	}
	if filter.SupportsCloudInit != nil {
		where = append(where, fmt.Sprintf("supports_cloudinit = $%d", idx))
		args = append(args, *filter.SupportsCloudInit)
		idx++
	}

	clause := strings.Join(where, " AND ")
	countQ := "SELECT COUNT(*) FROM templates WHERE " + clause
	total, err := CountRows(ctx, r.db, countQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting templates: %w", err)
	}

	limit := filter.Limit()
	offset := filter.Offset()
	listQ := fmt.Sprintf(
		"SELECT %s FROM templates WHERE %s ORDER BY sort_order ASC, name ASC LIMIT $%d OFFSET $%d",
		templateSelectCols, clause, idx, idx+1,
	)
	args = append(args, limit, offset)

	templates, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.Template, error) {
		return scanTemplate(rows)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing templates: %w", err)
	}
	return templates, total, nil
}

// ListActive returns all active templates ordered by sort_order.
func (r *TemplateRepository) ListActive(ctx context.Context) ([]models.Template, error) {
	const q = `SELECT ` + templateSelectCols + ` FROM templates WHERE is_active = true ORDER BY sort_order ASC, name ASC`
	templates, err := ScanRows(ctx, r.db, q, nil, func(rows pgx.Rows) (models.Template, error) {
		return scanTemplate(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing active templates: %w", err)
	}
	return templates, nil
}

// Update updates all editable fields of a template and increments the version.
// Returns ErrNoRowsAffected if no template was found to update.
func (r *TemplateRepository) Update(ctx context.Context, template *models.Template) error {
	const q = `
		UPDATE templates SET
			name = $1, os_family = $2, os_version = $3,
			rbd_image = $4, rbd_snapshot = $5, min_disk_gb = $6,
			supports_cloudinit = $7, is_active = $8, sort_order = $9,
			description = $10, storage_backend = $11, file_path = $12,
			version = version + 1, updated_at = NOW()
		WHERE id = $13
		RETURNING ` + templateSelectCols

	row := r.db.QueryRow(ctx, q,
		template.Name, template.OSFamily, template.OSVersion,
		template.RBDImage, template.RBDSnapshot, template.MinDiskGB,
		template.SupportsCloudInit, template.IsActive, template.SortOrder,
		template.Description, template.StorageBackend, template.FilePath,
		template.ID,
	)
	updated, err := scanTemplate(row)
	if err != nil {
		return fmt.Errorf("updating template %s: %w", template.ID, err)
	}
	*template = updated
	return nil
}

// UpdateActive updates the is_active status of a template and increments version.
// Returns ErrNoRowsAffected if no template was found to update.
func (r *TemplateRepository) UpdateActive(ctx context.Context, id string, isActive bool) error {
	const q = `UPDATE templates SET is_active = $1, version = version + 1, updated_at = NOW() WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, isActive, id)
	if err != nil {
		return fmt.Errorf("updating template %s active status: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating template %s active status: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// UpdateSortOrder updates the sort_order of a template and increments version.
// Returns ErrNoRowsAffected if no template was found to update.
func (r *TemplateRepository) UpdateSortOrder(ctx context.Context, id string, sortOrder int) error {
	const q = `UPDATE templates SET sort_order = $1, version = version + 1, updated_at = NOW() WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, sortOrder, id)
	if err != nil {
		return fmt.Errorf("updating template %s sort order: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating template %s sort order: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// Delete permanently removes a template from the database.
// Note: Templates with referencing VMs will have their template_id set to NULL due to FK ON DELETE SET NULL.
// Returns ErrNoRowsAffected if no template was found to delete.
func (r *TemplateRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM templates WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting template %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting template %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}