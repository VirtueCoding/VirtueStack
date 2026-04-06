// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// NodeStorageRepository provides database operations for the node_storage junction table.
type NodeStorageRepository struct {
	db DB
}

// NewNodeStorageRepository creates a new NodeStorageRepository with the given database connection.
func NewNodeStorageRepository(db DB) *NodeStorageRepository {
	return &NodeStorageRepository{db: db}
}

// NodeStorageAssignment represents a single node-to-backend assignment.
type NodeStorageAssignment struct {
	NodeID           string `json:"node_id" db:"node_id"`
	StorageBackendID string `json:"storage_backend_id" db:"storage_backend_id"`
	Enabled          bool   `json:"enabled" db:"enabled"`
}

// CreateAssignment creates a new node-to-storage-backend assignment.
func (r *NodeStorageRepository) CreateAssignment(ctx context.Context, nodeID, storageBackendID string, enabled bool) error {
	const q = `
		INSERT INTO node_storage (node_id, storage_backend_id, enabled)
		VALUES ($1, $2, $3)
		ON CONFLICT (node_id, storage_backend_id) DO UPDATE SET enabled = $3`

	_, err := r.db.Exec(ctx, q, nodeID, storageBackendID, enabled)
	if err != nil {
		return fmt.Errorf("creating node storage assignment: %w", err)
	}
	return nil
}

// DeleteAssignment removes a node-to-storage-backend assignment.
func (r *NodeStorageRepository) DeleteAssignment(ctx context.Context, nodeID, storageBackendID string) error {
	const q = `DELETE FROM node_storage WHERE node_id = $1 AND storage_backend_id = $2`
	_, err := r.db.Exec(ctx, q, nodeID, storageBackendID)
	if err != nil {
		return fmt.Errorf("deleting node storage assignment: %w", err)
	}
	return nil
}

// SetEnabled updates the enabled status of a node-to-storage-backend assignment.
func (r *NodeStorageRepository) SetEnabled(ctx context.Context, nodeID, storageBackendID string, enabled bool) error {
	const q = `UPDATE node_storage SET enabled = $1 WHERE node_id = $2 AND storage_backend_id = $3`
	tag, err := r.db.Exec(ctx, q, enabled, nodeID, storageBackendID)
	if err != nil {
		return fmt.Errorf("setting node storage enabled: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("setting node storage enabled: %w", ErrNoRowsAffected)
	}
	return nil
}

// GetAssignment returns a specific node-to-storage-backend assignment.
func (r *NodeStorageRepository) GetAssignment(ctx context.Context, nodeID, storageBackendID string) (*NodeStorageAssignment, error) {
	const q = `
		SELECT node_id, storage_backend_id, enabled
		FROM node_storage
		WHERE node_id = $1 AND storage_backend_id = $2`

	assignment, err := ScanRow(ctx, r.db, q, []any{nodeID, storageBackendID}, func(row pgx.Row) (NodeStorageAssignment, error) {
		var a NodeStorageAssignment
		err := row.Scan(&a.NodeID, &a.StorageBackendID, &a.Enabled)
		return a, err
	})
	if err != nil {
		return nil, fmt.Errorf("getting node storage assignment: %w", err)
	}
	return &assignment, nil
}

// ListAssignmentsForNode returns all storage backend assignments for a node.
func (r *NodeStorageRepository) ListAssignmentsForNode(ctx context.Context, nodeID string) ([]NodeStorageAssignment, error) {
	const q = `
		SELECT node_id, storage_backend_id, enabled
		FROM node_storage
		WHERE node_id = $1
		ORDER BY storage_backend_id`

	assignments, err := ScanRows(ctx, r.db, q, []any{nodeID}, func(rows pgx.Rows) (NodeStorageAssignment, error) {
		var a NodeStorageAssignment
		err := rows.Scan(&a.NodeID, &a.StorageBackendID, &a.Enabled)
		return a, err
	})
	if err != nil {
		return nil, fmt.Errorf("listing node storage assignments: %w", err)
	}
	return assignments, nil
}

// ListAssignmentsForBackend returns all node assignments for a storage backend.
func (r *NodeStorageRepository) ListAssignmentsForBackend(ctx context.Context, storageBackendID string) ([]NodeStorageAssignment, error) {
	const q = `
		SELECT node_id, storage_backend_id, enabled
		FROM node_storage
		WHERE storage_backend_id = $1
		ORDER BY node_id`

	assignments, err := ScanRows(ctx, r.db, q, []any{storageBackendID}, func(rows pgx.Rows) (NodeStorageAssignment, error) {
		var a NodeStorageAssignment
		err := rows.Scan(&a.NodeID, &a.StorageBackendID, &a.Enabled)
		return a, err
	})
	if err != nil {
		return nil, fmt.Errorf("listing backend storage assignments: %w", err)
	}
	return assignments, nil
}

// CountNodesForBackend returns the number of nodes assigned to a storage backend.
func (r *NodeStorageRepository) CountNodesForBackend(ctx context.Context, storageBackendID string) (int, error) {
	const q = `SELECT COUNT(*) FROM node_storage WHERE storage_backend_id = $1`
	count, err := CountRows(ctx, r.db, q, storageBackendID)
	if err != nil {
		return 0, fmt.Errorf("counting nodes for backend: %w", err)
	}
	return count, nil
}

// CountBackendsForNode returns the number of storage backends assigned to a node.
func (r *NodeStorageRepository) CountBackendsForNode(ctx context.Context, nodeID string) (int, error) {
	const q = `SELECT COUNT(*) FROM node_storage WHERE node_id = $1`
	count, err := CountRows(ctx, r.db, q, nodeID)
	if err != nil {
		return 0, fmt.Errorf("counting backends for node: %w", err)
	}
	return count, nil
}

// DeleteAllAssignmentsForNode removes all storage backend assignments for a node.
// This is typically used when deleting a node.
func (r *NodeStorageRepository) DeleteAllAssignmentsForNode(ctx context.Context, nodeID string) error {
	const q = `DELETE FROM node_storage WHERE node_id = $1`
	_, err := r.db.Exec(ctx, q, nodeID)
	if err != nil {
		return fmt.Errorf("deleting all node storage assignments: %w", err)
	}
	return nil
}

// DeleteAllAssignmentsForBackend removes all node assignments for a storage backend.
// This is typically used when deleting a storage backend.
func (r *NodeStorageRepository) DeleteAllAssignmentsForBackend(ctx context.Context, storageBackendID string) error {
	const q = `DELETE FROM node_storage WHERE storage_backend_id = $1`
	_, err := r.db.Exec(ctx, q, storageBackendID)
	if err != nil {
		return fmt.Errorf("deleting all backend storage assignments: %w", err)
	}
	return nil
}

// GetEnabledBackendsForNode returns all enabled storage backends for a node with full details.
func (r *NodeStorageRepository) GetEnabledBackendsForNode(ctx context.Context, nodeID string) ([]models.StorageBackend, error) {
	const q = `
		SELECT sb.id, sb.name, sb.type,
			sb.ceph_pool, sb.ceph_user, sb.ceph_monitors, sb.ceph_keyring_path,
			sb.storage_path, sb.lvm_volume_group, sb.lvm_thin_pool,
			sb.total_gb, sb.used_gb, sb.available_gb, sb.health_status, sb.health_message,
			sb.lvm_data_percent, sb.lvm_metadata_percent,
			sb.created_at, sb.updated_at
		FROM storage_backends sb
		JOIN node_storage nsb ON nsb.storage_backend_id = sb.id
		WHERE nsb.node_id = $1 AND nsb.enabled = true
		ORDER BY sb.name`

	backends, err := ScanRows(ctx, r.db, q, []any{nodeID}, func(rows pgx.Rows) (models.StorageBackend, error) {
		var sb models.StorageBackend
		err := rows.Scan(
			&sb.ID, &sb.Name, &sb.Type,
			&sb.CephPool, &sb.CephUser, &sb.CephMonitors, &sb.CephKeyringPath,
			&sb.StoragePath, &sb.LVMVolumeGroup, &sb.LVMThinPool,
			&sb.TotalGB, &sb.UsedGB, &sb.AvailableGB, &sb.HealthStatus, &sb.HealthMessage,
			&sb.LVMDataPercent, &sb.LVMMetadataPercent,
			&sb.CreatedAt, &sb.UpdatedAt,
		)
		return sb, err
	})
	if err != nil {
		return nil, fmt.Errorf("getting enabled backends for node: %w", err)
	}
	return backends, nil
}

// BatchAssignToNodes assigns a storage backend to multiple nodes in a single transaction.
func (r *NodeStorageRepository) BatchAssignToNodes(ctx context.Context, storageBackendID string, nodeIDs []string, enabled bool) error {
	if len(nodeIDs) == 0 {
		return nil
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	const q = `
		INSERT INTO node_storage (node_id, storage_backend_id, enabled)
		VALUES ($1, $2, $3)
		ON CONFLICT (node_id, storage_backend_id) DO UPDATE SET enabled = $3`

	for _, nodeID := range nodeIDs {
		_, err := tx.Exec(ctx, q, nodeID, storageBackendID, enabled)
		if err != nil {
			return fmt.Errorf("assigning storage backend to node %s: %w", nodeID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing batch assignment: %w", err)
	}
	return nil
}
