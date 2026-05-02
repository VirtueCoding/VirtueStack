// Package models provides data model types for VirtueStack Controller.
package models

import (
	"fmt"
	"slices"
	"time"

	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// VM status constants define the lifecycle states of a virtual machine.
const (
	VMStatusProvisioning = "provisioning"
	VMStatusRunning      = "running"
	VMStatusStopped      = "stopped"
	VMStatusSuspended    = "suspended"
	VMStatusDeleting     = "deleting"
	VMStatusMigrating    = "migrating"
	VMStatusReinstalling = "reinstalling"
	VMStatusError        = "error"
	VMStatusDeleted      = "deleted"
)

// ValidVMTransitions defines all allowed VM lifecycle state transitions.
var ValidVMTransitions = map[string][]string{
	VMStatusProvisioning: {VMStatusRunning, VMStatusError},
	VMStatusRunning:      {VMStatusStopped, VMStatusSuspended, VMStatusDeleting, VMStatusMigrating, VMStatusReinstalling, VMStatusError},
	VMStatusStopped:      {VMStatusRunning, VMStatusDeleting, VMStatusReinstalling, VMStatusMigrating, VMStatusError},
	VMStatusSuspended:    {VMStatusRunning, VMStatusStopped, VMStatusDeleting, VMStatusMigrating},
	VMStatusDeleting:     {VMStatusDeleted, VMStatusError},
	VMStatusMigrating:    {VMStatusRunning, VMStatusStopped, VMStatusSuspended, VMStatusError},
	VMStatusReinstalling: {VMStatusRunning, VMStatusError},
	VMStatusError:        {VMStatusStopped, VMStatusDeleting},
}

// ValidateVMTransition validates whether a VM status transition is allowed.
func ValidateVMTransition(from, to string) error {
	allowedTargets, ok := ValidVMTransitions[from]
	if !ok {
		return fmt.Errorf("unknown VM source status %q for transition to %q: %w", from, to, sharederrors.ErrConflict)
	}
	if !slices.Contains(allowedTargets, to) {
		return fmt.Errorf("invalid VM transition from %q to %q: %w", from, to, sharederrors.ErrConflict)
	}
	return nil
}

// VM represents a virtual machine record as stored in the database.
type VM struct {
	ID                    string    `json:"id" db:"id"`
	CustomerID            string    `json:"customer_id" db:"customer_id"`
	NodeID                *string   `json:"node_id,omitempty" db:"node_id"`
	PlanID                string    `json:"plan_id" db:"plan_id"`
	Name                  string    `json:"name"`
	Hostname              string    `json:"hostname" db:"hostname"`
	Status                string    `json:"status" db:"status"`
	IPv4                  string    `json:"ipv4"`
	VCPU                  int       `json:"vcpu" db:"vcpu"`
	MemoryMB              int       `json:"memory_mb" db:"memory_mb"`
	DiskGB                int       `json:"disk_gb" db:"disk_gb"`
	PortSpeedMbps         int       `json:"port_speed_mbps" db:"port_speed_mbps"`
	BandwidthLimitGB      int       `json:"bandwidth_limit_gb" db:"bandwidth_limit_gb"`
	BandwidthUsedBytes    int64     `json:"bandwidth_used_bytes" db:"bandwidth_used_bytes"`
	BandwidthResetAt      time.Time `json:"bandwidth_reset_at" db:"bandwidth_reset_at"`
	MACAddress            string    `json:"mac_address" db:"mac_address"`
	TemplateID            *string   `json:"template_id,omitempty" db:"template_id"`
	LibvirtDomainName     *string   `json:"libvirt_domain_name,omitempty" db:"libvirt_domain_name"`
	RootPasswordEncrypted *string   `json:"-" db:"root_password_encrypted"` // Never expose in JSON
	ExternalServiceID     *int      `json:"external_service_id,omitempty" db:"external_service_id"`
	// StorageBackend: "ceph" or "qcow". Defaults to "ceph" for backward compatibility.
	StorageBackend string `json:"storage_backend" db:"storage_backend"`
	// DiskPath: path to disk file for qcow2 storage. Empty for ceph (uses CephPool/RBDImage).
	DiskPath *string `json:"disk_path,omitempty" db:"disk_path"`
	// CephPool: Ceph pool name for backward compatibility.
	CephPool *string `json:"ceph_pool,omitempty" db:"ceph_pool"`
	// RBDImage: RBD image name for ceph storage.
	RBDImage *string `json:"rbd_image,omitempty" db:"rbd_image"`
	// StorageBackendID: UUID of the storage backend this VM uses. Nil for legacy VMs using node defaults.
	StorageBackendID *string `json:"storage_backend_id,omitempty" db:"storage_backend_id"`
	// AttachedISO: UUID of the currently attached ISO, nil if none.
	AttachedISO *string `json:"attached_iso,omitempty" db:"attached_iso"`
	Timestamps
	SoftDelete
}

// VMCreateRequest holds the fields required to provision a new virtual machine.
type VMCreateRequest struct {
	CustomerID        string   `json:"customer_id" validate:"required,uuid"`
	PlanID            string   `json:"plan_id" validate:"required,uuid"`
	Hostname          string   `json:"hostname" validate:"required,hostname_rfc1123,max=63"`
	TemplateID        string   `json:"template_id" validate:"required,uuid"`
	Password          string   `json:"password" validate:"required,min=12,max=128"`
	SSHKeys           []string `json:"ssh_keys,omitempty" validate:"max=10,dive,max=4096"`
	LocationID        *string  `json:"location_id,omitempty" validate:"omitempty,uuid"`
	ExternalServiceID *int     `json:"external_service_id,omitempty"`
	IdempotencyKey    string   `json:"-"` // From header
}

// VMListFilter holds query parameters for filtering and paginating VM list results.
type VMListFilter struct {
	CustomerID *string  `form:"customer_id"`
	NodeID     *string  `form:"node_id"`
	Status     *string  `form:"status"`
	Search     *string  `form:"search"` // hostname search
	VMIDs      []string `form:"-"`      // Filter by multiple VM IDs (for API key vm_ids scope)
	PaginationParams
}

// VMDetail is an enriched VM representation with associated IPs and related metadata,
// suitable for detailed API responses.
type VMDetail struct {
	VM
	IPAddresses  []IPAddress    `json:"ip_addresses"`
	IPv6Subnets  []VMIPv6Subnet `json:"ipv6_subnets,omitempty"`
	NodeHostname *string        `json:"node_hostname,omitempty"`
	PlanName     string         `json:"plan_name"`
	TemplateName *string        `json:"template_name,omitempty"`
}

// VMMetrics represents real-time resource utilization metrics for a VM.
type VMMetrics struct {
	VMID             string    `json:"vm_id"`
	CPUUsagePercent  float64   `json:"cpu_usage_percent"`
	MemoryUsageBytes int64     `json:"memory_usage_bytes"`
	MemoryTotalBytes int64     `json:"memory_total_bytes"`
	DiskReadBytes    int64     `json:"disk_read_bytes"`
	DiskWriteBytes   int64     `json:"disk_write_bytes"`
	NetworkRxBytes   int64     `json:"network_rx_bytes"`
	NetworkTxBytes   int64     `json:"network_tx_bytes"`
	UptimeSeconds    int64     `json:"uptime_seconds"`
	Timestamp        time.Time `json:"timestamp"`
}
