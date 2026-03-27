// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// TemplateCacheRepository provides database operations for template node cache entries.
type TemplateCacheRepository struct {
	db DB
}

// NewTemplateCacheRepository creates a new TemplateCacheRepository.
func NewTemplateCacheRepository(db DB) *TemplateCacheRepository {
	return &TemplateCacheRepository{db: db}
}

const templateCacheSelectCols = `
	template_id, node_id, status, local_path, size_bytes,
	synced_at, error_msg, created_at, updated_at`

func scanTemplateCacheEntry(row pgx.Row) (models.TemplateCacheEntry, error) {
	var e models.TemplateCacheEntry
	err := row.Scan(
		&e.TemplateID, &e.NodeID, &e.Status, &e.LocalPath,
		&e.SizeBytes, &e.SyncedAt, &e.ErrorMsg,
		&e.CreatedAt, &e.UpdatedAt,
	)
	return e, err
}

// Upsert inserts or updates a template cache entry using ON CONFLICT.
func (r *TemplateCacheRepository) Upsert(ctx context.Context, entry *models.TemplateCacheEntry) error {
	const q = `
		INSERT INTO template_node_cache (
			template_id, node_id, status, local_path, size_bytes,
			synced_at, error_msg
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (template_id, node_id) DO UPDATE SET
			status = EXCLUDED.status,
			local_path = EXCLUDED.local_path,
			size_bytes = EXCLUDED.size_bytes,
			synced_at = EXCLUDED.synced_at,
			error_msg = EXCLUDED.error_msg,
			updated_at = NOW()
		RETURNING ` + templateCacheSelectCols

	row := r.db.QueryRow(ctx, q,
		entry.TemplateID, entry.NodeID, entry.Status,
		entry.LocalPath, entry.SizeBytes, entry.SyncedAt,
		entry.ErrorMsg,
	)
	result, err := scanTemplateCacheEntry(row)
	if err != nil {
		return fmt.Errorf("upserting template cache entry: %w", err)
	}
	*entry = result
	return nil
}

// Get returns a cache entry by template and node IDs.
func (r *TemplateCacheRepository) Get(ctx context.Context, templateID, nodeID string) (*models.TemplateCacheEntry, error) {
	const q = `SELECT ` + templateCacheSelectCols + `
		FROM template_node_cache WHERE template_id = $1 AND node_id = $2`
	entry, err := ScanRow(ctx, r.db, q, []any{templateID, nodeID}, scanTemplateCacheEntry)
	if err != nil {
		return nil, fmt.Errorf("getting template cache entry: %w", err)
	}
	return &entry, nil
}

// ListByTemplate returns all cache entries for a given template.
func (r *TemplateCacheRepository) ListByTemplate(ctx context.Context, templateID string) ([]models.TemplateCacheEntry, error) {
	const q = `SELECT ` + templateCacheSelectCols + `
		FROM template_node_cache WHERE template_id = $1 ORDER BY node_id`
	entries, err := ScanRows(ctx, r.db, q, []any{templateID}, func(rows pgx.Rows) (models.TemplateCacheEntry, error) {
		return scanTemplateCacheEntry(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing template cache entries: %w", err)
	}
	return entries, nil
}

// ListByNode returns all cache entries for a given node.
func (r *TemplateCacheRepository) ListByNode(ctx context.Context, nodeID string) ([]models.TemplateCacheEntry, error) {
	const q = `SELECT ` + templateCacheSelectCols + `
		FROM template_node_cache WHERE node_id = $1 ORDER BY template_id`
	entries, err := ScanRows(ctx, r.db, q, []any{nodeID}, func(rows pgx.Rows) (models.TemplateCacheEntry, error) {
		return scanTemplateCacheEntry(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing template cache entries for node: %w", err)
	}
	return entries, nil
}

// UpdateStatus updates the status and optional fields of a cache entry.
func (r *TemplateCacheRepository) UpdateStatus(ctx context.Context, templateID, nodeID string, status models.TemplateCacheStatus, localPath *string, sizeBytes *int64, errorMsg *string) error {
	const q = `
		UPDATE template_node_cache SET
			status = $3,
			local_path = COALESCE($4, local_path),
			size_bytes = COALESCE($5, size_bytes),
			error_msg = $6,
			synced_at = CASE WHEN $3 = 'ready' THEN NOW() ELSE synced_at END,
			updated_at = NOW()
		WHERE template_id = $1 AND node_id = $2`
	tag, err := r.db.Exec(ctx, q, templateID, nodeID, status, localPath, sizeBytes, errorMsg)
	if err != nil {
		return fmt.Errorf("updating template cache status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating template cache status: %w", ErrNoRowsAffected)
	}
	return nil
}

// DeleteByTemplate removes all cache entries for a template.
func (r *TemplateCacheRepository) DeleteByTemplate(ctx context.Context, templateID string) error {
	const q = `DELETE FROM template_node_cache WHERE template_id = $1`
	_, err := r.db.Exec(ctx, q, templateID)
	if err != nil {
		return fmt.Errorf("deleting template cache entries: %w", err)
	}
	return nil
}

// DeleteByNode removes all cache entries for a node.
func (r *TemplateCacheRepository) DeleteByNode(ctx context.Context, nodeID string) error {
	const q = `DELETE FROM template_node_cache WHERE node_id = $1`
	_, err := r.db.Exec(ctx, q, nodeID)
	if err != nil {
		return fmt.Errorf("deleting template cache entries for node: %w", err)
	}
	return nil
}
