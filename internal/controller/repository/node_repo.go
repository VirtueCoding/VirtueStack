// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"database/sql"
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
	var cephPool sql.NullString
	err := row.Scan(
		&n.ID, &n.Hostname, &n.GRPCAddress, &n.ManagementIP,
		&n.LocationID, &n.Status, &n.TotalVCPU, &n.TotalMemoryMB,
		&n.AllocatedVCPU, &n.AllocatedMemoryMB, &cephPool,
		&n.IPMIAddress, &n.IPMIUsernameEncrypted, &n.IPMIPasswordEncrypted,
		&n.LastHeartbeatAt, &n.ConsecutiveHeartbeatMisses, &n.CreatedAt,
		&n.StorageBackend, &n.StoragePath, &n.CephMonitors, &n.CephUser,
	)
	if err != nil {
		return n, err
	}
	if cephPool.Valid {
		n.CephPool = cephPool.String
	}
	return n, nil
}

const nodeSelectCols = `
	id, hostname, grpc_address, management_ip::text,
	location_id, status, total_vcpu, total_memory_mb,
	allocated_vcpu, allocated_memory_mb, ceph_pool,
	ipmi_address::text, ipmi_username_encrypted, ipmi_password_encrypted,
	last_heartbeat_at, consecutive_heartbeat_misses, created_at,
	storage_backend, COALESCE(storage_path, ''), ceph_monitors, ceph_user`

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

// GetAllNodeIDs returns all nodes matching the given IDs.
// Returns a slice of IDs that were found. If any IDs are not found, the returned
// slice will have fewer elements than the input slice.
// Use len(result) != len(ids) to detect missing nodes.
func (r *NodeRepository) GetAllNodeIDs(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	const q = `SELECT id FROM nodes WHERE id = ANY($1)`
	rows, err := r.db.Query(ctx, q, ids)
	if err != nil {
		return nil, fmt.Errorf("getting nodes by IDs: %w", err)
	}
	defer rows.Close()

	var foundIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning node row: %w", err)
		}
		foundIDs = append(foundIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating node rows: %w", err)
	}
	return foundIDs, nil
}

// GetByIDs returns full node objects for all given IDs in a single query.
// Missing IDs are silently skipped; callers should compare len(result) vs len(ids).
func (r *NodeRepository) GetByIDs(ctx context.Context, ids []string) ([]models.Node, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	const q = `SELECT ` + nodeSelectCols + ` FROM nodes WHERE id = ANY($1)`
	nodes, err := ScanRows(ctx, r.db, q, []any{ids}, func(rows pgx.Rows) (models.Node, error) {
		return scanNode(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("getting nodes by IDs: %w", err)
	}
	return nodes, nil
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
func (r *NodeRepository) List(ctx context.Context, filter models.NodeListFilter) ([]models.Node, bool, string, error) {
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

	cp := filter.DecodeCursor()
	if cp.LastID != "" {
		where += fmt.Sprintf(" AND id < $%d", idx)
		args = append(args, cp.LastID)
		idx++
	}
	listQ := fmt.Sprintf(
		"SELECT %s FROM nodes WHERE %s ORDER BY id DESC LIMIT $%d",
		nodeSelectCols, where, idx,
	)
	args = append(args, filter.PerPage+1)

	nodes, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.Node, error) {
		return scanNode(rows)
	})
	if err != nil {
		return nil, false, "", fmt.Errorf("listing nodes: %w", err)
	}

	hasMore := len(nodes) > filter.PerPage
	if hasMore {
		nodes = nodes[:filter.PerPage]
	}
	var lastID string
	if len(nodes) > 0 {
		lastID = nodes[len(nodes)-1].ID
	}
	return nodes, hasMore, lastID, nil
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
	tag, err := r.db.Exec(ctx, q, misses, nodeID)
	if err != nil {
		return fmt.Errorf("updating heartbeat misses for node %s: %w", nodeID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating heartbeat misses for node %s: %w", nodeID, ErrNoRowsAffected)
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

// IncrementAllocatedResources atomically adds the specified vCPU and memory to a node's allocated counts.
// This is safe for concurrent updates as it uses SQL atomic operations.
func (r *NodeRepository) IncrementAllocatedResources(ctx context.Context, nodeID string, vcpu, memoryMB int) error {
	const q = `UPDATE nodes SET allocated_vcpu = allocated_vcpu + $1, allocated_memory_mb = allocated_memory_mb + $2 WHERE id = $3`
	tag, err := r.db.Exec(ctx, q, vcpu, memoryMB, nodeID)
	if err != nil {
		return fmt.Errorf("incrementing node %s allocated resources: %w", nodeID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("incrementing node %s allocated resources: %w", nodeID, ErrNoRowsAffected)
	}
	return nil
}

// DecrementAllocatedResources atomically subtracts the specified vCPU and memory from a node's allocated counts.
// This is safe for concurrent updates as it uses SQL atomic operations.
// Values are clamped to zero to prevent negative allocations.
func (r *NodeRepository) DecrementAllocatedResources(ctx context.Context, nodeID string, vcpu, memoryMB int) error {
	const q = `UPDATE nodes SET allocated_vcpu = GREATEST(0, allocated_vcpu - $1), allocated_memory_mb = GREATEST(0, allocated_memory_mb - $2) WHERE id = $3`
	tag, err := r.db.Exec(ctx, q, vcpu, memoryMB, nodeID)
	if err != nil {
		return fmt.Errorf("decrementing node %s allocated resources: %w", nodeID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("decrementing node %s allocated resources: %w", nodeID, ErrNoRowsAffected)
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
