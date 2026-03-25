// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// IPCooldownDuration is the period an IP address is held in cooldown after being
// released from a VM before it can be reassigned.  This prevents stale ARP/DNS
// entries from causing connectivity issues for the next customer.
const IPCooldownDuration = 5 * time.Minute

// IPRepository provides database operations for IP address management.
type IPRepository struct {
	db DB
}

// NewIPRepository creates a new IPRepository with the given database connection.
func NewIPRepository(db DB) *IPRepository {
	return &IPRepository{db: db}
}

// ============================================================================
// IPSet Operations
// ============================================================================

const ipSetSelectCols = `
	id, name, location_id, network, gateway, vlan_id, ip_version, node_ids, created_at`

func scanIPSet(row pgx.Row) (models.IPSet, error) {
	var ipSet models.IPSet
	err := row.Scan(
		&ipSet.ID, &ipSet.Name, &ipSet.LocationID, &ipSet.Network,
		&ipSet.Gateway, &ipSet.VLANID, &ipSet.IPVersion, &ipSet.NodeIDs,
		&ipSet.CreatedAt,
	)
	return ipSet, err
}

// CreateIPSet inserts a new IP set record into the database.
func (r *IPRepository) CreateIPSet(ctx context.Context, ipSet *models.IPSet) error {
	const q = `
		INSERT INTO ip_sets (name, location_id, network, gateway, vlan_id, ip_version, node_ids)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING ` + ipSetSelectCols

	row := r.db.QueryRow(ctx, q,
		ipSet.Name, ipSet.LocationID, ipSet.Network, ipSet.Gateway,
		ipSet.VLANID, ipSet.IPVersion, ipSet.NodeIDs,
	)
	created, err := scanIPSet(row)
	if err != nil {
		return fmt.Errorf("creating IP set: %w", err)
	}
	*ipSet = created
	return nil
}

// GetIPSetByID returns an IP set by its UUID. Returns ErrNotFound if no match.
func (r *IPRepository) GetIPSetByID(ctx context.Context, id string) (*models.IPSet, error) {
	const q = `SELECT ` + ipSetSelectCols + ` FROM ip_sets WHERE id = $1`
	ipSet, err := ScanRow(ctx, r.db, q, []any{id}, scanIPSet)
	if err != nil {
		return nil, fmt.Errorf("getting IP set %s: %w", id, err)
	}
	return &ipSet, nil
}

// GetIPSetByName returns an IP set by its name. Returns ErrNotFound if no match.
func (r *IPRepository) GetIPSetByName(ctx context.Context, name string) (*models.IPSet, error) {
	const q = `SELECT ` + ipSetSelectCols + ` FROM ip_sets WHERE name = $1`
	ipSet, err := ScanRow(ctx, r.db, q, []any{name}, scanIPSet)
	if err != nil {
		return nil, fmt.Errorf("getting IP set by name %s: %w", name, err)
	}
	return &ipSet, nil
}

// ListIPSets returns a paginated list of IP sets with optional filters.
func (r *IPRepository) ListIPSets(ctx context.Context, filter IPSetListFilter) ([]models.IPSet, int, error) {
	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if filter.LocationID != nil {
		where = append(where, fmt.Sprintf("location_id = $%d", idx))
		args = append(args, *filter.LocationID)
		idx++
	}
	if filter.IPVersion != nil {
		where = append(where, fmt.Sprintf("ip_version = $%d", idx))
		args = append(args, *filter.IPVersion)
		idx++
	}

	clause := strings.Join(where, " AND ")
	countQ := "SELECT COUNT(*) FROM ip_sets WHERE " + clause
	total, err := CountRows(ctx, r.db, countQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting IP sets: %w", err)
	}

	limit := filter.Limit()
	offset := filter.Offset()
	listQ := fmt.Sprintf(
		"SELECT %s FROM ip_sets WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		ipSetSelectCols, clause, idx, idx+1,
	)
	args = append(args, limit, offset)

	ipSets, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.IPSet, error) {
		return scanIPSet(rows)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing IP sets: %w", err)
	}
	return ipSets, total, nil
}

// UpdateIPSet updates all mutable fields of an IP set.
func (r *IPRepository) UpdateIPSet(ctx context.Context, ipSet *models.IPSet) error {
	const q = `
		UPDATE ip_sets SET
			name = $1, location_id = $2, network = $3, gateway = $4,
			vlan_id = $5, ip_version = $6, node_ids = $7
		WHERE id = $8
		RETURNING ` + ipSetSelectCols

	row := r.db.QueryRow(ctx, q,
		ipSet.Name, ipSet.LocationID, ipSet.Network, ipSet.Gateway,
		ipSet.VLANID, ipSet.IPVersion, ipSet.NodeIDs, ipSet.ID,
	)
	updated, err := scanIPSet(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("updating IP set %s: %w", ipSet.ID, ErrNoRowsAffected)
		}
		return fmt.Errorf("updating IP set %s: %w", ipSet.ID, err)
	}
	*ipSet = updated
	return nil
}

// DeleteIPSet permanently removes an IP set from the database.
func (r *IPRepository) DeleteIPSet(ctx context.Context, id string) error {
	const q = `DELETE FROM ip_sets WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting IP set %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting IP set %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// ============================================================================
// IPAddress Operations
// ============================================================================

const ipAddressSelectCols = `
	id, ip_set_id, address, ip_version, vm_id, customer_id, is_primary,
	rdns_hostname, status, assigned_at, released_at, cooldown_until, created_at`

func scanIPAddress(row pgx.Row) (models.IPAddress, error) {
	var ip models.IPAddress
	err := row.Scan(
		&ip.ID, &ip.IPSetID, &ip.Address, &ip.IPVersion,
		&ip.VMID, &ip.CustomerID, &ip.IsPrimary, &ip.RDNSHostname,
		&ip.Status, &ip.AssignedAt, &ip.ReleasedAt, &ip.CooldownUntil,
		&ip.CreatedAt,
	)
	return ip, err
}

// CreateIPAddress inserts a new IP address record into the database.
func (r *IPRepository) CreateIPAddress(ctx context.Context, ip *models.IPAddress) error {
	const q = `
		INSERT INTO ip_addresses (ip_set_id, address, ip_version, vm_id, customer_id, is_primary, rdns_hostname, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING ` + ipAddressSelectCols

	row := r.db.QueryRow(ctx, q,
		ip.IPSetID, ip.Address, ip.IPVersion, ip.VMID, ip.CustomerID,
		ip.IsPrimary, ip.RDNSHostname, ip.Status,
	)
	created, err := scanIPAddress(row)
	if err != nil {
		return fmt.Errorf("creating IP address: %w", err)
	}
	*ip = created
	return nil
}

// GetIPAddressByID returns an IP address by its UUID. Returns ErrNotFound if no match.
func (r *IPRepository) GetIPAddressByID(ctx context.Context, id string) (*models.IPAddress, error) {
	const q = `SELECT ` + ipAddressSelectCols + ` FROM ip_addresses WHERE id = $1`
	ip, err := ScanRow(ctx, r.db, q, []any{id}, scanIPAddress)
	if err != nil {
		return nil, fmt.Errorf("getting IP address %s: %w", id, err)
	}
	return &ip, nil
}

// GetIPAddressByAddress returns an IP address by its address string. Returns ErrNotFound if no match.
func (r *IPRepository) GetIPAddressByAddress(ctx context.Context, address string) (*models.IPAddress, error) {
	const q = `SELECT ` + ipAddressSelectCols + ` FROM ip_addresses WHERE address = $1`
	ip, err := ScanRow(ctx, r.db, q, []any{address}, scanIPAddress)
	if err != nil {
		return nil, fmt.Errorf("getting IP address by address %s: %w", address, err)
	}
	return &ip, nil
}

// ListIPAddresses returns a paginated list of IP addresses with optional filters.
func (r *IPRepository) ListIPAddresses(ctx context.Context, filter IPAddressListFilter) ([]models.IPAddress, int, error) {
	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if filter.IPSetID != nil {
		where = append(where, fmt.Sprintf("ip_set_id = $%d", idx))
		args = append(args, *filter.IPSetID)
		idx++
	}
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
	if filter.Status != nil {
		where = append(where, fmt.Sprintf("status = $%d", idx))
		args = append(args, *filter.Status)
		idx++
	}

	clause := strings.Join(where, " AND ")
	countQ := "SELECT COUNT(*) FROM ip_addresses WHERE " + clause
	total, err := CountRows(ctx, r.db, countQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting IP addresses: %w", err)
	}

	limit := filter.Limit()
	offset := filter.Offset()
	listQ := fmt.Sprintf(
		"SELECT %s FROM ip_addresses WHERE %s ORDER BY address ASC LIMIT $%d OFFSET $%d",
		ipAddressSelectCols, clause, idx, idx+1,
	)
	args = append(args, limit, offset)

	ips, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.IPAddress, error) {
		return scanIPAddress(rows)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing IP addresses: %w", err)
	}
	return ips, total, nil
}

// CountIPsByStatus returns a map of IP status to count for a given IP set.
// This is more efficient than listing all IPs for large sets like /16 (65K IPs).
func (r *IPRepository) CountIPsByStatus(ctx context.Context, ipSetID string) (map[string]int, error) {
	const q = `
		SELECT status, COUNT(*) AS cnt
		FROM ip_addresses
		WHERE ip_set_id = $1
		GROUP BY status`

	rows, err := r.db.Query(ctx, q, ipSetID)
	if err != nil {
		return nil, fmt.Errorf("counting IPs by status for set %s: %w", ipSetID, err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var cnt int
		if scanErr := rows.Scan(&status, &cnt); scanErr != nil {
			return nil, fmt.Errorf("scanning IP status count: %w", scanErr)
		}
		counts[status] = cnt
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating IP status counts: %w", err)
	}
	return counts, nil
}

// AllocateIPv4 atomically allocates an available IPv4 address from an IP set.
// A single UPDATE ... WHERE id = (SELECT id ... FOR UPDATE SKIP LOCKED) RETURNING
// statement is used to reduce from two round-trips to one, eliminating the window
// between the SELECT and the UPDATE that existed in the previous implementation.
// Returns the allocated IP address or an error if no addresses are available.
func (r *IPRepository) AllocateIPv4(ctx context.Context, ipSetID, vmID, customerID string) (*models.IPAddress, error) {
	now := time.Now().UTC()
	const q = `
		UPDATE ip_addresses SET
			status = 'assigned',
			vm_id = $2,
			customer_id = $3,
			assigned_at = $4
		WHERE id = (
			SELECT id FROM ip_addresses
			WHERE ip_set_id = $1 AND status = 'available'
			ORDER BY address ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING ` + ipAddressSelectCols

	row := r.db.QueryRow(ctx, q, ipSetID, vmID, customerID, now)
	updated, err := scanIPAddress(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("no available IPv4 addresses in IP set %s", ipSetID)
		}
		return nil, fmt.Errorf("allocating IPv4 address from IP set %s: %w", ipSetID, err)
	}

	return &updated, nil
}

// ReleaseIPv4 sets an IP address to cooldown status and schedules it for reuse.
// The cooldown_until is set to the current time plus the cooldown period.
func (r *IPRepository) ReleaseIPv4(ctx context.Context, ipID string) error {
	// Get current time and calculate cooldown end.
	now := time.Now().UTC()
	cooldownEnd := now.Add(IPCooldownDuration)

	const q = `
		UPDATE ip_addresses SET
			status = 'cooldown',
			vm_id = NULL,
			customer_id = NULL,
			released_at = $1,
			cooldown_until = $2,
			is_primary = FALSE
		WHERE id = $3 AND status = 'assigned'`

	tag, err := r.db.Exec(ctx, q, now, cooldownEnd, ipID)
	if err != nil {
		return fmt.Errorf("releasing IP address %s: %w", ipID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("releasing IP address %s: %w", ipID, ErrNoRowsAffected)
	}

	return nil
}

// SetRDNS sets the reverse DNS hostname for an IP address.
func (r *IPRepository) SetRDNS(ctx context.Context, ipID, hostname string) error {
	const q = `UPDATE ip_addresses SET rdns_hostname = $1 WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, hostname, ipID)
	if err != nil {
		return fmt.Errorf("setting RDNS for IP %s: %w", ipID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("setting RDNS for IP %s: %w", ipID, ErrNoRowsAffected)
	}
	return nil
}

// GetRDNS returns the reverse DNS hostname for an IP address.
func (r *IPRepository) GetRDNS(ctx context.Context, ipID string) (string, error) {
	const q = `SELECT rdns_hostname FROM ip_addresses WHERE id = $1`
	var hostname *string
	row := r.db.QueryRow(ctx, q, ipID)
	if err := row.Scan(&hostname); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("getting RDNS for IP %s: %w", ipID, ErrNoRowsAffected)
		}
		return "", fmt.Errorf("getting RDNS for IP %s: %w", ipID, err)
	}
	if hostname == nil {
		return "", nil
	}
	return *hostname, nil
}

// ============================================================================
// IPv6Prefix Operations
// ============================================================================

const ipv6PrefixSelectCols = `id, node_id, prefix, created_at`

func scanIPv6Prefix(row pgx.Row) (models.IPv6Prefix, error) {
	var prefix models.IPv6Prefix
	err := row.Scan(&prefix.ID, &prefix.NodeID, &prefix.Prefix, &prefix.CreatedAt)
	return prefix, err
}

// CreateIPv6Prefix inserts a new IPv6 prefix record into the database.
func (r *IPRepository) CreateIPv6Prefix(ctx context.Context, prefix *models.IPv6Prefix) error {
	const q = `
		INSERT INTO ipv6_prefixes (node_id, prefix)
		VALUES ($1, $2)
		RETURNING ` + ipv6PrefixSelectCols

	row := r.db.QueryRow(ctx, q, prefix.NodeID, prefix.Prefix)
	created, err := scanIPv6Prefix(row)
	if err != nil {
		return fmt.Errorf("creating IPv6 prefix: %w", err)
	}
	*prefix = created
	return nil
}

// GetIPv6PrefixByNode returns the IPv6 prefix allocated to a node. Returns ErrNotFound if none exists.
func (r *IPRepository) GetIPv6PrefixByNode(ctx context.Context, nodeID string) (*models.IPv6Prefix, error) {
	const q = `SELECT ` + ipv6PrefixSelectCols + ` FROM ipv6_prefixes WHERE node_id = $1`
	prefix, err := ScanRow(ctx, r.db, q, []any{nodeID}, scanIPv6Prefix)
	if err != nil {
		return nil, fmt.Errorf("getting IPv6 prefix for node %s: %w", nodeID, err)
	}
	return &prefix, nil
}

// DeleteIPv6Prefix permanently removes an IPv6 prefix from the database.
func (r *IPRepository) DeleteIPv6Prefix(ctx context.Context, id string) error {
	const q = `DELETE FROM ipv6_prefixes WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting IPv6 prefix %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting IPv6 prefix %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// ============================================================================
// VMIPv6Subnet Operations
// ============================================================================

const vmIPv6SubnetSelectCols = `id, vm_id, ipv6_prefix_id, subnet, subnet_index, gateway, created_at`

func scanVMIPv6Subnet(row pgx.Row) (models.VMIPv6Subnet, error) {
	var subnet models.VMIPv6Subnet
	err := row.Scan(
		&subnet.ID, &subnet.VMID, &subnet.IPv6PrefixID,
		&subnet.Subnet, &subnet.SubnetIndex, &subnet.Gateway,
		&subnet.CreatedAt,
	)
	return subnet, err
}

// CreateVMIPv6Subnet inserts a new VM IPv6 subnet record into the database.
func (r *IPRepository) CreateVMIPv6Subnet(ctx context.Context, subnet *models.VMIPv6Subnet) error {
	const q = `
		INSERT INTO vm_ipv6_subnets (vm_id, ipv6_prefix_id, subnet, subnet_index, gateway)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING ` + vmIPv6SubnetSelectCols

	row := r.db.QueryRow(ctx, q,
		subnet.VMID, subnet.IPv6PrefixID, subnet.Subnet, subnet.SubnetIndex, subnet.Gateway,
	)
	created, err := scanVMIPv6Subnet(row)
	if err != nil {
		return fmt.Errorf("creating VM IPv6 subnet: %w", err)
	}
	*subnet = created
	return nil
}

// GetVMIPv6SubnetsByVM returns all IPv6 subnets assigned to a VM.
func (r *IPRepository) GetVMIPv6SubnetsByVM(ctx context.Context, vmID string) ([]models.VMIPv6Subnet, error) {
	const q = `SELECT ` + vmIPv6SubnetSelectCols + ` FROM vm_ipv6_subnets WHERE vm_id = $1 ORDER BY subnet_index ASC`
	subnets, err := ScanRows(ctx, r.db, q, []any{vmID}, func(rows pgx.Rows) (models.VMIPv6Subnet, error) {
		return scanVMIPv6Subnet(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("getting IPv6 subnets for VM %s: %w", vmID, err)
	}
	return subnets, nil
}

// DeleteVMIPv6SubnetsByVM removes all IPv6 subnets for a VM.
func (r *IPRepository) DeleteVMIPv6SubnetsByVM(ctx context.Context, vmID string) error {
	const q = `DELETE FROM vm_ipv6_subnets WHERE vm_id = $1`
	_, err := r.db.Exec(ctx, q, vmID)
	if err != nil {
		return fmt.Errorf("deleting IPv6 subnets for VM %s: %w", vmID, err)
	}
	return nil
}

// GetVMIPv6SubnetsByPrefix returns all IPv6 subnets for a prefix.
func (r *IPRepository) GetVMIPv6SubnetsByPrefix(ctx context.Context, prefixID string) ([]models.VMIPv6Subnet, error) {
	const q = `SELECT ` + vmIPv6SubnetSelectCols + ` FROM vm_ipv6_subnets WHERE ipv6_prefix_id = $1 ORDER BY subnet_index ASC`
	subnets, err := ScanRows(ctx, r.db, q, []any{prefixID}, func(rows pgx.Rows) (models.VMIPv6Subnet, error) {
		return scanVMIPv6Subnet(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("getting IPv6 subnets for prefix %s: %w", prefixID, err)
	}
	return subnets, nil
}

// GetVMIPv6SubnetByID returns an IPv6 subnet by its ID.
func (r *IPRepository) GetVMIPv6SubnetByID(ctx context.Context, id string) (*models.VMIPv6Subnet, error) {
	const q = `SELECT ` + vmIPv6SubnetSelectCols + ` FROM vm_ipv6_subnets WHERE id = $1`
	subnet, err := ScanRow(ctx, r.db, q, []any{id}, scanVMIPv6Subnet)
	if err != nil {
		return nil, fmt.Errorf("getting IPv6 subnet %s: %w", id, err)
	}
	return &subnet, nil
}

// DeleteVMIPv6SubnetByID removes a single IPv6 subnet by its ID.
func (r *IPRepository) DeleteVMIPv6SubnetByID(ctx context.Context, id string) error {
	const q = `DELETE FROM vm_ipv6_subnets WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting IPv6 subnet %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting IPv6 subnet %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// CreateVMIPv6SubnetWithIndexCheck atomically finds the next available subnet index
// and creates a new IPv6 subnet. This prevents TOCTOU race conditions when multiple
// concurrent requests try to allocate subnets from the same prefix.
// The prefix string is queried within the transaction to compute the actual subnet.
// Returns an error if the prefix is exhausted (65536 subnets max for a /48).
//
// NOTE: Even with the FOR UPDATE lock on the prefix row, two concurrent transactions
// that both read the same max(subnet_index) before either inserts can assign duplicate
// subnet_index values if the lock on the prefix row is not held consistently.
// A UNIQUE constraint on (ipv6_prefix_id, subnet_index) in the database provides
// a safety-net that causes one of the concurrent inserts to fail with a
// unique-violation error rather than silently storing a duplicate.
// Migration 000058 adds this constraint: uq_ipv6_subnets_prefix_index.
func (r *IPRepository) CreateVMIPv6SubnetWithIndexCheck(ctx context.Context, vmID, prefixID string) (*models.VMIPv6Subnet, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	// Lock the prefix row and get the prefix string
	const lockQ = `SELECT prefix FROM ipv6_prefixes WHERE id = $1 FOR UPDATE`
	var prefixStr string
	err = tx.QueryRow(ctx, lockQ, prefixID).Scan(&prefixStr)
	if err != nil {
		return nil, fmt.Errorf("locking IPv6 prefix row: %w", err)
	}

	// Find the max subnet_index within the transaction
	const maxQ = `SELECT COALESCE(MAX(subnet_index), -1) FROM vm_ipv6_subnets WHERE ipv6_prefix_id = $1`
	var maxIndex int
	err = tx.QueryRow(ctx, maxQ, prefixID).Scan(&maxIndex)
	if err != nil {
		return nil, fmt.Errorf("finding max subnet index: %w", err)
	}

	nextIndex := maxIndex + 1

	// Check we haven't exhausted the /48 prefix (65536 possible /64 subnets)
	if nextIndex >= 65536 {
		return nil, fmt.Errorf("IPv6 prefix is exhausted (no available /64 subnets)")
	}

	// Generate the /64 subnet from the /48 prefix
	subnet, gateway, err := generateIPv6Subnet(prefixStr, nextIndex)
	if err != nil {
		return nil, fmt.Errorf("generating IPv6 subnet: %w", err)
	}

	// Create the subnet with the calculated index
	const q = `
		INSERT INTO vm_ipv6_subnets (vm_id, ipv6_prefix_id, subnet, subnet_index, gateway)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING ` + vmIPv6SubnetSelectCols

	row := tx.QueryRow(ctx, q, vmID, prefixID, subnet, nextIndex, gateway)
	created, err := scanVMIPv6Subnet(row)
	if err != nil {
		return nil, fmt.Errorf("creating VM IPv6 subnet: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("committing transaction: %w", err)
	}

	return &created, nil
}

// generateIPv6Subnet generates a /64 subnet and gateway from a /48 prefix and subnet index.
// The subnet index determines which /64 within the /48 is allocated.
func generateIPv6Subnet(prefix48 string, subnetIndex int) (subnet, gateway string, err error) {
	// Parse the /48 prefix
	prefix, err := netip.ParsePrefix(prefix48)
	if err != nil {
		return "", "", fmt.Errorf("parsing prefix %s: %w", prefix48, err)
	}

	// Get the prefix address
	addr := prefix.Addr()

	// Convert to 16-byte array
	bytes := addr.As16()

	// The subnet index goes in bits 48-63 of the address (the next 16 bits after the /48 prefix)
	// We need to modify bytes 6 and 7 (indices 6 and 7 in the 16-byte array)
	// Bytes 6-7 contain bits 48-63
	byte6 := (subnetIndex >> 8) & 0xFF
	byte7 := subnetIndex & 0xFF

	bytes[6] = byte(byte6)
	bytes[7] = byte(byte7)

	// Create the /64 subnet
	subnetAddr := netip.AddrFrom16(bytes)
	subnet = fmt.Sprintf("%s/64", subnetAddr.String())

	// Gateway is the first address in the subnet (::1)
	gatewayBytes := bytes
	gatewayBytes[15] = 1
	gatewayAddr := netip.AddrFrom16(gatewayBytes)
	gateway = gatewayAddr.String()

	return subnet, gateway, nil
}

// ============================================================================
// Filter Types
// ============================================================================

// IPSetListFilter provides filtering options for listing IP sets.
type IPSetListFilter struct {
	models.PaginationParams
	LocationID *string
	IPVersion  *int16
}

// IPAddressListFilter provides filtering options for listing IP addresses.
type IPAddressListFilter struct {
	models.PaginationParams
	IPSetID    *string
	VMID       *string
	CustomerID *string
	Status     *string
}