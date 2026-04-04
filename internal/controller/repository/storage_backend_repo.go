// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository/cursor"
)

// StorageBackendRepository provides database operations for storage backends.
type StorageBackendRepository struct {
	db DB
}

// NewStorageBackendRepository creates a new StorageBackendRepository with the given database connection.
func NewStorageBackendRepository(db DB) *StorageBackendRepository {
	return &StorageBackendRepository{db: db}
}

const storageBackendSelectCols = `
	id, name, type,
	ceph_pool, ceph_user, ceph_monitors, ceph_keyring_path,
	storage_path, lvm_volume_group, lvm_thin_pool,
	lvm_data_percent_threshold, lvm_metadata_percent_threshold,
	total_gb, used_gb, available_gb, health_status, health_message,
	lvm_data_percent, lvm_metadata_percent,
	created_at, updated_at`

// scanStorageBackend scans a single storage backend row into a models.StorageBackend struct.
func scanStorageBackend(row pgx.Row) (models.StorageBackend, error) {
	var sb models.StorageBackend
	err := row.Scan(
		&sb.ID, &sb.Name, &sb.Type,
		&sb.CephPool, &sb.CephUser, &sb.CephMonitors, &sb.CephKeyringPath,
		&sb.StoragePath, &sb.LVMVolumeGroup, &sb.LVMThinPool,
		&sb.LVMDataPercentThreshold, &sb.LVMMetadataPercentThreshold,
		&sb.TotalGB, &sb.UsedGB, &sb.AvailableGB, &sb.HealthStatus, &sb.HealthMessage,
		&sb.LVMDataPercent, &sb.LVMMetadataPercent,
		&sb.CreatedAt, &sb.UpdatedAt,
	)
	return sb, err
}

// Create inserts a new storage backend record into the database.
// The storage backend's ID, CreatedAt, and UpdatedAt are populated by the database.
func (r *StorageBackendRepository) Create(ctx context.Context, sb *models.StorageBackend) error {
	const q = `
		INSERT INTO storage_backends (
			name, type,
			ceph_pool, ceph_user, ceph_monitors, ceph_keyring_path,
			storage_path, lvm_volume_group, lvm_thin_pool,
			lvm_data_percent_threshold, lvm_metadata_percent_threshold,
			health_status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING ` + storageBackendSelectCols

	row := r.db.QueryRow(ctx, q,
		sb.Name, sb.Type,
		sb.CephPool, sb.CephUser, sb.CephMonitors, sb.CephKeyringPath,
		sb.StoragePath, sb.LVMVolumeGroup, sb.LVMThinPool,
		sb.LVMDataPercentThreshold, sb.LVMMetadataPercentThreshold,
		sb.HealthStatus,
	)
	created, err := scanStorageBackend(row)
	if err != nil {
		return fmt.Errorf("creating storage backend: %w", err)
	}
	*sb = created
	return nil
}

// GetByID returns a storage backend by its UUID. Returns ErrNotFound if no backend matches.
func (r *StorageBackendRepository) GetByID(ctx context.Context, id string) (*models.StorageBackend, error) {
	const q = `SELECT ` + storageBackendSelectCols + ` FROM storage_backends WHERE id = $1`
	sb, err := ScanRow(ctx, r.db, q, []any{id}, scanStorageBackend)
	if err != nil {
		return nil, fmt.Errorf("getting storage backend %s: %w", id, err)
	}
	return &sb, nil
}

// GetByIDWithNodes returns a storage backend by ID with its assigned nodes populated.
func (r *StorageBackendRepository) GetByIDWithNodes(ctx context.Context, id string) (*models.StorageBackend, error) {
	sb, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	nodes, err := r.GetNodesForBackend(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting nodes for storage backend %s: %w", id, err)
	}
	sb.Nodes = nodes
	return sb, nil
}

// List returns a paginated list of storage backends with optional filters.
func (r *StorageBackendRepository) List(ctx context.Context, filter models.StorageBackendListFilter) ([]models.StorageBackend, bool, string, error) {
	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if filter.Type != nil {
		where = append(where, fmt.Sprintf("type = $%d", idx))
		args = append(args, *filter.Type)
		idx++
	}
	if filter.Status != nil {
		where = append(where, fmt.Sprintf("health_status = $%d", idx))
		args = append(args, *filter.Status)
		idx++
	}

	clause := strings.Join(where, " AND ")

	cp := cursor.ParseParams(filter.PaginationParams)
	var extraArg any
	clause, idx, extraArg = cursor.BuildWhereClause(clause, cp, true, idx)
	if extraArg != nil {
		args = append(args, extraArg)
	}

	listQ := fmt.Sprintf(
		"SELECT %s FROM storage_backends WHERE %s ORDER BY id DESC LIMIT $%d",
		storageBackendSelectCols, clause, idx,
	)
	args = append(args, filter.PerPage+1)

	backends, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.StorageBackend, error) {
		return scanStorageBackend(rows)
	})
	if err != nil {
		return nil, false, "", fmt.Errorf("listing storage backends: %w", err)
	}

	backends, hasMore, lastID := cursor.TrimResults(backends, filter.PerPage, func(sb models.StorageBackend) string { return sb.ID })
	return backends, hasMore, lastID, nil
}

// ListAll returns all storage backends without pagination.
// Use sparingly; prefer List for paginated results.
func (r *StorageBackendRepository) ListAll(ctx context.Context) ([]models.StorageBackend, error) {
	const q = `SELECT ` + storageBackendSelectCols + ` FROM storage_backends ORDER BY name`
	backends, err := ScanRows(ctx, r.db, q, nil, func(rows pgx.Rows) (models.StorageBackend, error) {
		return scanStorageBackend(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing all storage backends: %w", err)
	}
	return backends, nil
}

// Update updates an existing storage backend record.
func (r *StorageBackendRepository) Update(ctx context.Context, sb *models.StorageBackend) error {
	const q = `
		UPDATE storage_backends SET
			name = $1,
			ceph_pool = $2, ceph_user = $3, ceph_monitors = $4, ceph_keyring_path = $5,
			storage_path = $6, lvm_volume_group = $7, lvm_thin_pool = $8,
			lvm_data_percent_threshold = $9, lvm_metadata_percent_threshold = $10,
			updated_at = NOW()
		WHERE id = $11
		RETURNING ` + storageBackendSelectCols

	updated, err := ScanRow(ctx, r.db, q, []any{
		sb.Name,
		sb.CephPool, sb.CephUser, sb.CephMonitors, sb.CephKeyringPath,
		sb.StoragePath, sb.LVMVolumeGroup, sb.LVMThinPool,
		sb.LVMDataPercentThreshold, sb.LVMMetadataPercentThreshold,
		sb.ID,
	}, scanStorageBackend)
	if err != nil {
		return fmt.Errorf("updating storage backend %s: %w", sb.ID, err)
	}
	*sb = updated
	return nil
}

// Delete removes a storage backend by ID.
// Returns ErrNoRowsAffected if the backend does not exist.
func (r *StorageBackendRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM storage_backends WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting storage backend %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting storage backend %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// AssignToNodes assigns a storage backend to multiple nodes.
// This creates records in the node_storage_backends junction table.
func (r *StorageBackendRepository) AssignToNodes(ctx context.Context, storageBackendID string, nodeIDs []string) error {
	if len(nodeIDs) == 0 {
		return nil
	}

	// Build batch insert
	const insertPrefix = `INSERT INTO node_storage_backends (node_id, storage_backend_id, enabled) VALUES `
	placeholders := make([]string, len(nodeIDs))
	args := make([]any, 0, len(nodeIDs)*3)

	for i, nodeID := range nodeIDs {
		baseIdx := i * 3
		placeholders[i] = fmt.Sprintf("($%d, $%d, $%d)", baseIdx+1, baseIdx+2, baseIdx+3)
		args = append(args, nodeID, storageBackendID, true)
	}

	q := insertPrefix + strings.Join(placeholders, ", ") +
		` ON CONFLICT (node_id, storage_backend_id) DO UPDATE SET enabled = EXCLUDED.enabled`

	_, err := r.db.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("assigning storage backend %s to nodes: %w", storageBackendID, err)
	}
	return nil
}

// RemoveFromNode removes a storage backend assignment from a specific node.
func (r *StorageBackendRepository) RemoveFromNode(ctx context.Context, storageBackendID, nodeID string) error {
	const q = `DELETE FROM node_storage_backends WHERE node_id = $1 AND storage_backend_id = $2`
	_, err := r.db.Exec(ctx, q, nodeID, storageBackendID)
	if err != nil {
		return fmt.Errorf("removing storage backend %s from node %s: %w", storageBackendID, nodeID, err)
	}
	return nil
}

// GetNodesForBackend returns all nodes assigned to a storage backend.
func (r *StorageBackendRepository) GetNodesForBackend(ctx context.Context, storageBackendID string) ([]models.StorageBackendNode, error) {
	const q = `
		SELECT nsb.node_id, n.hostname, nsb.enabled
		FROM node_storage_backends nsb
		JOIN nodes n ON n.id = nsb.node_id
		WHERE nsb.storage_backend_id = $1
		ORDER BY n.hostname`

	nodes, err := ScanRows(ctx, r.db, q, []any{storageBackendID}, func(rows pgx.Rows) (models.StorageBackendNode, error) {
		var node models.StorageBackendNode
		err := rows.Scan(&node.NodeID, &node.Hostname, &node.Enabled)
		return node, err
	})
	if err != nil {
		return nil, fmt.Errorf("getting nodes for storage backend %s: %w", storageBackendID, err)
	}
	return nodes, nil
}

// GetBackendsForNode returns all storage backends assigned to a specific node.
func (r *StorageBackendRepository) GetBackendsForNode(ctx context.Context, nodeID string) ([]models.StorageBackend, error) {
	const q = `
		SELECT ` + storageBackendSelectCols + `
		FROM storage_backends sb
		JOIN node_storage_backends nsb ON nsb.storage_backend_id = sb.id
		WHERE nsb.node_id = $1 AND nsb.enabled = true
		ORDER BY sb.name`

	backends, err := ScanRows(ctx, r.db, q, []any{nodeID}, func(rows pgx.Rows) (models.StorageBackend, error) {
		return scanStorageBackend(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("getting backends for node %s: %w", nodeID, err)
	}
	return backends, nil
}

// UpdateHealth updates the health metrics for a storage backend.
func (r *StorageBackendRepository) UpdateHealth(ctx context.Context, id string, health models.StorageBackendHealth) error {
	const q = `
		UPDATE storage_backends SET
			total_gb = $1,
			used_gb = $2,
			available_gb = $3,
			health_status = $4,
			health_message = $5,
			lvm_data_percent = $6,
			lvm_metadata_percent = $7,
			updated_at = NOW()
		WHERE id = $8`

	tag, err := r.db.Exec(ctx, q,
		health.TotalGB,
		health.UsedGB,
		health.AvailableGB,
		health.HealthStatus,
		health.HealthMessage,
		health.LVMDataPercent,
		health.LVMMetadataPercent,
		id,
	)
	if err != nil {
		return fmt.Errorf("updating storage backend %s health: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating storage backend %s health: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// GetByName returns a storage backend by its name. Returns ErrNotFound if no backend matches.
func (r *StorageBackendRepository) GetByName(ctx context.Context, name string) (*models.StorageBackend, error) {
	const q = `SELECT ` + storageBackendSelectCols + ` FROM storage_backends WHERE name = $1`
	sb, err := ScanRow(ctx, r.db, q, []any{name}, scanStorageBackend)
	if err != nil {
		return nil, fmt.Errorf("getting storage backend by name %s: %w", name, err)
	}
	return &sb, nil
}
