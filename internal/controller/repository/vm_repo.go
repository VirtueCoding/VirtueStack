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

// VMRepository provides database operations for virtual machines.
type VMRepository struct {
	db DB
}

// NewVMRepository creates a new VMRepository with the given database connection.
func NewVMRepository(db DB) *VMRepository {
	return &VMRepository{db: db}
}

// scanVM scans a single VM row into a models.VM struct.
func scanVM(row pgx.Row) (models.VM, error) {
	var vm models.VM
	err := row.Scan(
		&vm.ID, &vm.CustomerID, &vm.NodeID, &vm.PlanID,
		&vm.Hostname, &vm.Status, &vm.VCPU, &vm.MemoryMB,
		&vm.DiskGB, &vm.PortSpeedMbps, &vm.BandwidthLimitGB,
		&vm.BandwidthUsedBytes, &vm.BandwidthResetAt,
		&vm.MACAddress, &vm.TemplateID, &vm.LibvirtDomainName,
		&vm.RootPasswordEncrypted, &vm.WHMCSServiceID,
		&vm.AttachedISO,
		&vm.CreatedAt, &vm.UpdatedAt, &vm.DeletedAt,
		&vm.StorageBackend, &vm.DiskPath, &vm.CephPool, &vm.RBDImage,
	)
	return vm, err
}

const vmSelectCols = `
	id, customer_id, node_id, plan_id,
	hostname, status, vcpu, memory_mb,
	disk_gb, port_speed_mbps, bandwidth_limit_gb,
	bandwidth_used_bytes, bandwidth_reset_at,
	mac_address, template_id, libvirt_domain_name,
	root_password_encrypted, whmcs_service_id,
	attached_iso,
	created_at, updated_at, deleted_at,
	storage_backend, disk_path, ceph_pool, rbd_image`

// Create inserts a new VM record into the database.
// The VM's ID, CreatedAt, and UpdatedAt are populated by the database.
func (r *VMRepository) Create(ctx context.Context, vm *models.VM) error {
	const q = `
		INSERT INTO vms (
			customer_id, node_id, plan_id, hostname, status,
			vcpu, memory_mb, disk_gb, port_speed_mbps, bandwidth_limit_gb,
			mac_address, template_id, libvirt_domain_name,
			root_password_encrypted, whmcs_service_id,
			storage_backend, disk_path
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING ` + vmSelectCols

	row := r.db.QueryRow(ctx, q,
		vm.CustomerID, vm.NodeID, vm.PlanID, vm.Hostname, vm.Status,
		vm.VCPU, vm.MemoryMB, vm.DiskGB, vm.PortSpeedMbps, vm.BandwidthLimitGB,
		vm.MACAddress, vm.TemplateID, vm.LibvirtDomainName,
		vm.RootPasswordEncrypted, vm.WHMCSServiceID,
		vm.StorageBackend, vm.DiskPath,
	)
	created, err := scanVM(row)
	if err != nil {
		return fmt.Errorf("creating VM: %w", err)
	}
	*vm = created
	return nil
}

// GetByID returns a VM by its UUID. Returns ErrNotFound if no VM matches.
func (r *VMRepository) GetByID(ctx context.Context, id string) (*models.VM, error) {
	const q = `SELECT ` + vmSelectCols + ` FROM vms WHERE id = $1 AND deleted_at IS NULL`
	vm, err := ScanRow(ctx, r.db, q, []any{id}, scanVM)
	if err != nil {
		return nil, fmt.Errorf("getting VM %s: %w", id, err)
	}
	return &vm, nil
}

// GetByIDForCustomer returns a VM by ID within a transaction that has RLS applied.
// SetCustomerContext must be called on tx before this method.
func (r *VMRepository) GetByIDForCustomer(ctx context.Context, tx pgx.Tx, id, customerID string) (*models.VM, error) {
	const q = `SELECT ` + vmSelectCols + ` FROM vms WHERE id = $1 AND deleted_at IS NULL`
	vm, err := ScanRow(ctx, tx, q, []any{id}, scanVM)
	if err != nil {
		return nil, fmt.Errorf("getting VM %s for customer %s: %w", id, customerID, err)
	}
	return &vm, nil
}

// List returns a paginated list of VMs with optional filters and total count.
func (r *VMRepository) List(ctx context.Context, filter models.VMListFilter) ([]models.VM, int, error) {
	where := []string{"deleted_at IS NULL"}
	args := []any{}
	idx := 1

	if filter.CustomerID != nil {
		where = append(where, fmt.Sprintf("customer_id = $%d", idx))
		args = append(args, *filter.CustomerID)
		idx++
	}
	if filter.NodeID != nil {
		where = append(where, fmt.Sprintf("node_id = $%d", idx))
		args = append(args, *filter.NodeID)
		idx++
	}
	if filter.Status != nil {
		where = append(where, fmt.Sprintf("status = $%d", idx))
		args = append(args, *filter.Status)
		idx++
	}
	if filter.Search != nil {
		where = append(where, fmt.Sprintf("hostname ILIKE $%d", idx))
		args = append(args, "%"+*filter.Search+"%")
		idx++
	}
	if len(filter.VMIDs) > 0 {
		placeholders := make([]string, len(filter.VMIDs))
		for i, vmID := range filter.VMIDs {
			placeholders[i] = fmt.Sprintf("$%d", idx)
			args = append(args, vmID)
			idx++
		}
		where = append(where, fmt.Sprintf("id IN (%s)", strings.Join(placeholders, ",")))
	}

	clause := strings.Join(where, " AND ")
	countQ := "SELECT COUNT(*) FROM vms WHERE " + clause
	total, err := CountRows(ctx, r.db, countQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting VMs: %w", err)
	}

	limit := filter.Limit()
	offset := filter.Offset()
	listQ := fmt.Sprintf(
		"SELECT %s FROM vms WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		vmSelectCols, clause, idx, idx+1,
	)
	args = append(args, limit, offset)

	vms, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.VM, error) {
		return scanVM(rows)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing VMs: %w", err)
	}
	return vms, total, nil
}

// ListByCustomer returns all VMs owned by a customer with pagination.
func (r *VMRepository) ListByCustomer(ctx context.Context, customerID string, params models.PaginationParams) ([]models.VM, int, error) {
	filter := models.VMListFilter{
		CustomerID:       &customerID,
		PaginationParams: params,
	}
	return r.List(ctx, filter)
}

// UpdateStatus updates the status field of a VM.
func (r *VMRepository) UpdateStatus(ctx context.Context, id, status string) error {
	const q = `UPDATE vms SET status = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, q, status, id)
	if err != nil {
		return fmt.Errorf("updating VM %s status: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating VM %s status: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// UpdateNodeAssignment updates the node a VM is assigned to.
func (r *VMRepository) UpdateNodeAssignment(ctx context.Context, vmID, nodeID string) error {
	const q = `UPDATE vms SET node_id = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, q, nodeID, vmID)
	if err != nil {
		return fmt.Errorf("updating VM %s node assignment: %w", vmID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating VM %s node assignment: %w", vmID, ErrNoRowsAffected)
	}
	return nil
}

// UpdateResources updates the vCPU, memory, and disk allocations of a VM.
func (r *VMRepository) UpdateResources(ctx context.Context, id string, vcpu, memoryMB, diskGB int) error {
	const q = `
		UPDATE vms SET vcpu = $1, memory_mb = $2, disk_gb = $3, updated_at = NOW()
		WHERE id = $4 AND deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, q, vcpu, memoryMB, diskGB, id)
	if err != nil {
		return fmt.Errorf("updating VM %s resources: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating VM %s resources: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// UpdateBandwidthUsed updates the bandwidth_used_bytes counter for a VM.
func (r *VMRepository) UpdateBandwidthUsed(ctx context.Context, id string, usedBytes int64) error {
	const q = `UPDATE vms SET bandwidth_used_bytes = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, q, usedBytes, id)
	if err != nil {
		return fmt.Errorf("updating VM %s bandwidth: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating VM %s bandwidth: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// ResetBandwidth resets the bandwidth counter to zero and records the reset timestamp.
func (r *VMRepository) ResetBandwidth(ctx context.Context, id string) error {
	const q = `
		UPDATE vms SET bandwidth_used_bytes = 0, bandwidth_reset_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("resetting VM %s bandwidth: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("resetting VM %s bandwidth: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// SoftDelete marks a VM as deleted by setting deleted_at and status to "deleted".
func (r *VMRepository) SoftDelete(ctx context.Context, id string) error {
	now := time.Now().UTC()
	const q = `UPDATE vms SET deleted_at = $1, status = 'deleted', updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, q, now, id)
	if err != nil {
		return fmt.Errorf("soft-deleting VM %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("soft-deleting VM %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// GetByWHMCSServiceID finds a VM by its WHMCS service ID.
// Returns ErrNotFound if no matching VM exists.
func (r *VMRepository) GetByWHMCSServiceID(ctx context.Context, serviceID int) (*models.VM, error) {
	const q = `SELECT ` + vmSelectCols + ` FROM vms WHERE whmcs_service_id = $1 AND deleted_at IS NULL`
	vm, err := ScanRow(ctx, r.db, q, []any{serviceID}, scanVM)
	if err != nil {
		return nil, fmt.Errorf("getting VM by WHMCS service %d: %w", serviceID, err)
	}
	return &vm, nil
}

// CountByNode returns the number of active (non-deleted) VMs on a given node.
func (r *VMRepository) CountByNode(ctx context.Context, nodeID string) (int, error) {
	const q = `SELECT COUNT(*) FROM vms WHERE node_id = $1 AND deleted_at IS NULL`
	count, err := CountRows(ctx, r.db, q, nodeID)
	if err != nil {
		return 0, fmt.Errorf("counting VMs on node %s: %w", nodeID, err)
	}
	return count, nil
}

// UpdateTemplateID updates the template_id of a VM.
// This is used after a reinstall to track which template the VM was rebuilt from.
func (r *VMRepository) UpdateTemplateID(ctx context.Context, vmID, templateID string) error {
	const q = `UPDATE vms SET template_id = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, q, templateID, vmID)
	if err != nil {
		return fmt.Errorf("updating VM %s template_id: %w", vmID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating VM %s template_id: %w", vmID, ErrNoRowsAffected)
	}
	return nil
}

// UpdateMACAddress updates the mac_address of a VM.
// This is used after VM creation to persist the generated MAC address.
func (r *VMRepository) UpdateMACAddress(ctx context.Context, vmID, macAddress string) error {
	const q = `UPDATE vms SET mac_address = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, q, macAddress, vmID)
	if err != nil {
		return fmt.Errorf("updating VM %s mac_address: %w", vmID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating VM %s mac_address: %w", vmID, ErrNoRowsAffected)
	}
	return nil
}

// UpdatePassword updates the root_password_encrypted field of a VM.
// The encryptedPassword should already be hashed/encrypted before calling this method.
// Returns ErrNoRowsAffected if the VM does not exist or is already deleted.
func (r *VMRepository) UpdatePassword(ctx context.Context, vmID string, encryptedPassword string) error {
	const q = `UPDATE vms SET root_password_encrypted = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, q, encryptedPassword, vmID)
	if err != nil {
		return fmt.Errorf("updating VM %s password: %w", vmID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating VM %s password: %w", vmID, ErrNoRowsAffected)
	}
	return nil
}

// UpdateHostname updates the hostname of a VM.
func (r *VMRepository) UpdateHostname(ctx context.Context, vmID, hostname string) error {
	const q = `UPDATE vms SET hostname = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, q, hostname, vmID)
	if err != nil {
		return fmt.Errorf("updating hostname for VM %s: %w", vmID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating hostname for VM %s: %w", vmID, ErrNoRowsAffected)
	}
	return nil
}

// UpdateNetworkLimits updates the port_speed_mbps and bandwidth_limit_gb fields of a VM.
func (r *VMRepository) UpdateNetworkLimits(ctx context.Context, vmID string, portSpeedMbps, bandwidthLimitGB int) error {
	const q = `UPDATE vms SET port_speed_mbps = $1, bandwidth_limit_gb = $2, updated_at = NOW()
		WHERE id = $3 AND deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, q, portSpeedMbps, bandwidthLimitGB, vmID)
	if err != nil {
		return fmt.Errorf("updating network limits for VM %s: %w", vmID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating network limits for VM %s: %w", vmID, ErrNoRowsAffected)
	}
	return nil
}

// ListAllActive returns all active (non-deleted) VMs that have a node assigned.
// Pagination is intentionally omitted: this method is used exclusively by
// internal scheduled tasks (e.g. automated bandwidth checks and backup scheduling)
// that must process every active VM. Callers are responsible for chunking work
// if the result set is large.
//
// NOTE: This method has no LIMIT and will load every active VM into memory.
// For large fleets prefer ListAllActiveCursor which implements keyset pagination
// with id > $1 so that memory usage is bounded per page.
func (r *VMRepository) ListAllActive(ctx context.Context) ([]models.VM, error) {
	const q = `SELECT ` + vmSelectCols + ` FROM vms WHERE deleted_at IS NULL AND node_id IS NOT NULL`
	vms, err := ScanRows(ctx, r.db, q, nil, func(rows pgx.Rows) (models.VM, error) {
		return scanVM(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing active VMs: %w", err)
	}
	return vms, nil
}

// ListAllActiveBatch returns a batch of active VMs using keyset (cursor-based) pagination.
// Use this for large-scale deployments where loading all VMs at once would be memory-intensive.
// Pass afterID="" for the first batch, then pass the last VM's ID from the previous batch.
// The offset parameter is unused and retained only for call-site compatibility; callers should
// migrate to ListAllActiveCursor directly.
// Returns an empty slice when no more results are available.
//
// Note: The previous OFFSET-based implementation had O(n) cost at the database for large offsets.
// This implementation delegates to ListAllActiveCursor which uses id > $1 keyset pagination.
func (r *VMRepository) ListAllActiveBatch(ctx context.Context, afterID string, limit int) ([]models.VM, error) {
	return r.ListAllActiveCursor(ctx, afterID, limit)
}

// ListAllActiveCursor returns a batch of active VMs using cursor-based pagination.
// This is more efficient than offset-based pagination for large datasets.
// Pass afterID="" for the first batch, then pass the last VM's ID from the previous batch.
// Returns an empty slice when no more results are available.
func (r *VMRepository) ListAllActiveCursor(ctx context.Context, afterID string, limit int) ([]models.VM, error) {
	var q string
	var args []any

	if afterID == "" {
		q = `SELECT ` + vmSelectCols + ` FROM vms WHERE deleted_at IS NULL AND node_id IS NOT NULL ORDER BY id LIMIT $1`
		args = []any{limit}
	} else {
		q = `SELECT ` + vmSelectCols + ` FROM vms WHERE deleted_at IS NULL AND node_id IS NOT NULL AND id > $1 ORDER BY id LIMIT $2`
		args = []any{afterID, limit}
	}

	vms, err := ScanRows(ctx, r.db, q, args, func(rows pgx.Rows) (models.VM, error) {
		return scanVM(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing active VMs cursor: %w", err)
	}
	return vms, nil
}

// UpdateAttachedISO sets or clears the attached ISO for a VM.
func (r *VMRepository) UpdateAttachedISO(ctx context.Context, vmID string, isoID *string) error {
	const q = `UPDATE vms SET attached_iso = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, q, isoID, vmID)
	if err != nil {
		return fmt.Errorf("updating attached ISO for VM %s: %w", vmID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("VM %s not found: %w", vmID, ErrNoRowsAffected)
	}
	return nil
}

// ListByNodeIDs returns all active VMs on any of the specified nodes.
// This is more efficient than making multiple individual node queries.
func (r *VMRepository) ListByNodeIDs(ctx context.Context, nodeIDs []string) ([]models.VM, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	q := `SELECT ` + vmSelectCols + ` FROM vms WHERE deleted_at IS NULL AND node_id = ANY($1)`
	vms, err := ScanRows(ctx, r.db, q, []any{nodeIDs}, func(rows pgx.Rows) (models.VM, error) {
		return scanVM(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing VMs by node IDs: %w", err)
	}
	return vms, nil
}

// ListByCustomerIDs returns all active VMs owned by any of the specified customers.
// This is more efficient than making multiple individual customer queries.
func (r *VMRepository) ListByCustomerIDs(ctx context.Context, customerIDs []string) ([]models.VM, error) {
	if len(customerIDs) == 0 {
		return nil, nil
	}

	q := `SELECT ` + vmSelectCols + ` FROM vms WHERE deleted_at IS NULL AND customer_id = ANY($1)`
	vms, err := ScanRows(ctx, r.db, q, []any{customerIDs}, func(rows pgx.Rows) (models.VM, error) {
		return scanVM(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing VMs by customer IDs: %w", err)
	}
	return vms, nil
}

// ListByPlanIDs returns all active VMs using any of the specified plans.
// This is more efficient than fetching all VMs and filtering in-memory.
func (r *VMRepository) ListByPlanIDs(ctx context.Context, planIDs []string) ([]models.VM, error) {
	if len(planIDs) == 0 {
		return nil, nil
	}

	q := `SELECT ` + vmSelectCols + ` FROM vms WHERE deleted_at IS NULL AND plan_id = ANY($1)`
	vms, err := ScanRows(ctx, r.db, q, []any{planIDs}, func(rows pgx.Rows) (models.VM, error) {
		return scanVM(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing VMs by plan IDs: %w", err)
	}
	return vms, nil
}

// AdminBackupTargetFilter combines multiple filter criteria for admin backup schedules.
// All specified filters are applied as an AND condition (intersection).
type AdminBackupTargetFilter struct {
	NodeIDs     []string
	CustomerIDs []string
	PlanIDs     []string
}

// ListForAdminBackupTarget returns VMs matching the combined filter criteria.
// If multiple filters are specified, the result is their intersection.
// If no filters are specified, all active VMs are returned.
func (r *VMRepository) ListForAdminBackupTarget(ctx context.Context, filter AdminBackupTargetFilter) ([]models.VM, error) {
	// If no filters, return all active VMs
	if len(filter.NodeIDs) == 0 && len(filter.CustomerIDs) == 0 && len(filter.PlanIDs) == 0 {
		return r.ListAllActive(ctx)
	}

	// Build query with optional filters
	where := []string{"deleted_at IS NULL", "node_id IS NOT NULL"}
	args := []any{}
	idx := 1

	if len(filter.NodeIDs) > 0 {
		where = append(where, fmt.Sprintf("node_id = ANY($%d)", idx))
		args = append(args, filter.NodeIDs)
		idx++
	}
	if len(filter.CustomerIDs) > 0 {
		where = append(where, fmt.Sprintf("customer_id = ANY($%d)", idx))
		args = append(args, filter.CustomerIDs)
		idx++
	}
	if len(filter.PlanIDs) > 0 {
		where = append(where, fmt.Sprintf("plan_id = ANY($%d)", idx))
		args = append(args, filter.PlanIDs)
	}

	q := fmt.Sprintf("SELECT %s FROM vms WHERE %s", vmSelectCols, strings.Join(where, " AND "))
	vms, err := ScanRows(ctx, r.db, q, args, func(rows pgx.Rows) (models.VM, error) {
		return scanVM(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing VMs for admin backup target: %w", err)
	}
	return vms, nil
}

// UpdatePlanID updates the plan_id for a VM.
// This is used when a VM is upgraded to a new plan.
func (r *VMRepository) UpdatePlanID(ctx context.Context, vmID, planID string) error {
	const q = `UPDATE vms SET plan_id = $1, updated_at = NOW() WHERE id = $2 AND deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, q, planID, vmID)
	if err != nil {
		return fmt.Errorf("updating VM %s plan: %w", vmID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating VM %s plan: %w", vmID, ErrNoRowsAffected)
	}
	return nil
}

// CountByStorageBackend returns the count of active (non-deleted) VMs using a storage backend.
// This is used to prevent deletion of storage backends that are in use.
func (r *VMRepository) CountByStorageBackend(ctx context.Context, storageBackendID string) (int, error) {
	const q = `SELECT COUNT(*) FROM vms WHERE storage_backend_id = $1 AND deleted_at IS NULL`
	count, err := CountRows(ctx, r.db, q, storageBackendID)
	if err != nil {
		return 0, fmt.Errorf("counting VMs by storage backend %s: %w", storageBackendID, err)
	}
	return count, nil
}
