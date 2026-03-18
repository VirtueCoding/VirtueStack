// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// NodeRepository provides database operations for hypervisor nodes.
type NodeRepository struct {
	db DB
}

// NewNodeRepository creates a new NodeRepository with the given database connection.
func NewNodeRepository(db DB) *NodeRepository {
	return &NodeRepository{db: db}
}

// scanNode scans a single node row into a models.Node struct.
func scanNode(row pgx.Row) (models.Node, error) {
	var n models.Node
	err := row.Scan(
		&n.ID, &n.Hostname, &n.GRPCAddress, &n.ManagementIP,
		&n.LocationID, &n.Status, &n.TotalVCPU, &n.TotalMemoryMB,
		&n.AllocatedVCPU, &n.AllocatedMemoryMB, &n.CephPool,
		&n.IPMIAddress, &n.IPMIUsernameEncrypted, &n.IPMIPasswordEncrypted,
		&n.LastHeartbeatAt, &n.ConsecutiveHeartbeatMisses, &n.CreatedAt,
		&n.StorageBackend, &n.StoragePath, &n.CephMonitors, &n.CephUser,
	)
	return n, err
}

const nodeSelectCols = `
	id, hostname, grpc_address, management_ip,
	location_id, status, total_vcpu, total_memory_mb,
	allocated_vcpu, allocated_memory_mb, ceph_pool,
	ipmi_address, ipmi_username_encrypted, ipmi_password_encrypted,
	last_heartbeat_at, consecutive_heartbeat_misses, created_at,
	storage_backend, storage_path, ceph_monitors, ceph_user`

// Create inserts a new node record into the database.
func (r *NodeRepository) Create(ctx context.Context, node *models.Node) error {
	const q = `
		INSERT INTO nodes (
			hostname, grpc_address, management_ip, location_id, status,
			total_vcpu, total_memory_mb, ceph_pool,
			ipmi_address, ipmi_username_encrypted, ipmi_password_encrypted,
			storage_backend, storage_path
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING ` + nodeSelectCols

	row := r.db.QueryRow(ctx, q,
		node.Hostname, node.GRPCAddress, node.ManagementIP, node.LocationID, node.Status,
		node.TotalVCPU, node.TotalMemoryMB, node.CephPool,
		node.IPMIAddress, node.IPMIUsernameEncrypted, node.IPMIPasswordEncrypted,
		node.StorageBackend, node.StoragePath,
	)
	created, err := scanNode(row)
	if err != nil {
		return fmt.Errorf("creating node: %w", err)
	}
	*node = created
	return nil
}

// GetByID returns a node by its UUID. Returns ErrNotFound if no node matches.
func (r *NodeRepository) GetByID(ctx context.Context, id string) (*models.Node, error) {
	const q = `SELECT ` + nodeSelectCols + ` FROM nodes WHERE id = $1`
	node, err := ScanRow(ctx, r.db, q, []any{id}, scanNode)
	if err != nil {
		return nil, fmt.Errorf("getting node %s: %w", id, err)
	}
	return &node, nil
}

// GetByHostname returns a node by its hostname. Returns ErrNotFound if no node matches.
func (r *NodeRepository) GetByHostname(ctx context.Context, hostname string) (*models.Node, error) {
	const q = `SELECT ` + nodeSelectCols + ` FROM nodes WHERE hostname = $1`
	node, err := ScanRow(ctx, r.db, q, []any{hostname}, scanNode)
	if err != nil {
		return nil, fmt.Errorf("getting node by hostname %s: %w", hostname, err)
	}
	return &node, nil
}

// List returns all nodes with pagination and optional status filter.
func (r *NodeRepository) List(ctx context.Context, filter models.NodeListFilter) ([]models.Node, int, error) {
	where := "1=1"
	args := []any{}
	idx := 1

	if filter.Status != nil {
		where += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, *filter.Status)
		idx++
	}
	if filter.LocationID != nil {
		where += fmt.Sprintf(" AND location_id = $%d", idx)
		args = append(args, *filter.LocationID)
		idx++
	}

	total, err := CountRows(ctx, r.db, "SELECT COUNT(*) FROM nodes WHERE "+where, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting nodes: %w", err)
	}

	listQ := fmt.Sprintf(
		"SELECT %s FROM nodes WHERE %s ORDER BY hostname ASC LIMIT $%d OFFSET $%d",
		nodeSelectCols, where, idx, idx+1,
	)
	args = append(args, filter.Limit(), filter.Offset())

	nodes, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.Node, error) {
		return scanNode(rows)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing nodes: %w", err)
	}
	return nodes, total, nil
}

// ListByStatus returns all nodes matching the given status.
// Pagination is intentionally omitted: this is an internal method used by the
// scheduler and provisioning logic that must consider every eligible node.
// The total number of nodes is bounded by infrastructure capacity.
func (r *NodeRepository) ListByStatus(ctx context.Context, status string) ([]models.Node, error) {
	const q = `SELECT ` + nodeSelectCols + ` FROM nodes WHERE status = $1 ORDER BY hostname ASC`
	nodes, err := ScanRows(ctx, r.db, q, []any{status}, func(rows pgx.Rows) (models.Node, error) {
		return scanNode(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing nodes by status %s: %w", status, err)
	}
	return nodes, nil
}

// UpdateStatus updates a node's operational status.
func (r *NodeRepository) UpdateStatus(ctx context.Context, id, status string) error {
	const q = `UPDATE nodes SET status = $1 WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, status, id)
	if err != nil {
		return fmt.Errorf("updating node %s status: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating node %s status: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// RecordHeartbeat inserts a heartbeat record for a node and updates last_heartbeat_at.
func (r *NodeRepository) RecordHeartbeat(ctx context.Context, hb *models.NodeHeartbeat) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("starting heartbeat transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	// Rollback error is ignored intentionally: if Commit succeeds, Rollback is a no-op.
	// If Commit fails, the original error is already being returned and is more important.
	// This is standard Go idiom for transaction defer - rollback is a safety net.

	const insertQ = `
		INSERT INTO node_heartbeats (node_id, timestamp, cpu_percent, memory_percent, disk_percent, vm_count, load_average)
		VALUES ($1, NOW(), $2, $3, $4, $5, $6)
		RETURNING id, timestamp`
	row := tx.QueryRow(ctx, insertQ,
		hb.NodeID, hb.CPUPercent, hb.MemoryPercent, hb.DiskPercent, hb.VMCount, hb.LoadAverage,
	)
	if err := row.Scan(&hb.ID, &hb.Timestamp); err != nil {
		return fmt.Errorf("inserting node heartbeat: %w", err)
	}

	const updateQ = `
		UPDATE nodes SET last_heartbeat_at = NOW(), consecutive_heartbeat_misses = 0,
		status = CASE WHEN status = 'offline' THEN 'online' ELSE status END
		WHERE id = $1`
	if _, err := tx.Exec(ctx, updateQ, hb.NodeID); err != nil {
		return fmt.Errorf("updating node %s last_heartbeat_at: %w", hb.NodeID, err)
	}

	return tx.Commit(ctx)
}

// UpdateHeartbeatMisses increments the consecutive heartbeat miss counter for a node.
func (r *NodeRepository) UpdateHeartbeatMisses(ctx context.Context, nodeID string, misses int) error {
	const q = `UPDATE nodes SET consecutive_heartbeat_misses = $1 WHERE id = $2`
	if _, err := r.db.Exec(ctx, q, misses, nodeID); err != nil {
		return fmt.Errorf("updating heartbeat misses for node %s: %w", nodeID, err)
	}
	return nil
}

// UpdateAllocatedResources updates the allocated vCPU and memory counts for a node.
func (r *NodeRepository) UpdateAllocatedResources(ctx context.Context, nodeID string, vcpu, memoryMB int) error {
	const q = `UPDATE nodes SET allocated_vcpu = $1, allocated_memory_mb = $2 WHERE id = $3`
	tag, err := r.db.Exec(ctx, q, vcpu, memoryMB, nodeID)
	if err != nil {
		return fmt.Errorf("updating node %s allocated resources: %w", nodeID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating node %s allocated resources: %w", nodeID, ErrNoRowsAffected)
	}
	return nil
}

// GetLeastLoadedNode returns the online node in the given location with the most available capacity.
// Nodes can host VMs with any storage backend (ceph or qcow), as long as the
// necessary configuration is present on the node.
// Returns ErrNotFound if no eligible node exists.
func (r *NodeRepository) GetLeastLoadedNode(ctx context.Context, locationID string) (*models.Node, error) {
	const q = `
		SELECT ` + nodeSelectCols + `
		FROM nodes
		WHERE location_id = $1
		  AND status = 'online'
		ORDER BY (total_vcpu - allocated_vcpu) DESC,
		         (total_memory_mb - allocated_memory_mb) DESC
		LIMIT 1`
	node, err := ScanRow(ctx, r.db, q, []any{locationID}, scanNode)
	if err != nil {
		return nil, fmt.Errorf("getting least loaded node in location %s: %w", locationID, err)
	}
	return &node, nil
}

// Delete permanently removes a node record from the database.
func (r *NodeRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM nodes WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting node %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting node %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// Update updates all mutable fields of a node and returns the updated record.
func (r *NodeRepository) Update(ctx context.Context, node *models.Node) error {
	const q = `
		UPDATE nodes SET
			grpc_address = $1,
			location_id = $2,
			total_vcpu = $3,
			total_memory_mb = $4,
			ipmi_address = $5,
			storage_backend = $6,
			storage_path = $7
		WHERE id = $8
		RETURNING ` + nodeSelectCols

	row := r.db.QueryRow(ctx, q,
		node.GRPCAddress,
		node.LocationID,
		node.TotalVCPU,
		node.TotalMemoryMB,
		node.IPMIAddress,
		node.StorageBackend,
		node.StoragePath,
		node.ID,
	)

	updated, err := scanNode(row)
	if err != nil {
		return fmt.Errorf("updating node %s: %w", node.ID, err)
	}

	*node = updated
	return nil
}
