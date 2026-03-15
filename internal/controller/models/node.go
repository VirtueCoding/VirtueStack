// Package models provides data model types for VirtueStack Controller.
package models

import "time"

// Node status constants define the operational states of a hypervisor node.
const (
	NodeStatusOnline   = "online"
	NodeStatusDegraded = "degraded"
	NodeStatusOffline  = "offline"
	NodeStatusDraining = "draining"
	NodeStatusFailed   = "failed"
)

// Node represents a hypervisor node as stored in the database.
type Node struct {
	ID                string  `json:"id" db:"id"`
	Hostname          string  `json:"hostname" db:"hostname"`
	GRPCAddress       string  `json:"grpc_address" db:"grpc_address"`
	ManagementIP      string  `json:"management_ip" db:"management_ip"`
	LocationID        *string `json:"location_id,omitempty" db:"location_id"`
	Status            string  `json:"status" db:"status"`
	TotalVCPU         int     `json:"total_vcpu" db:"total_vcpu"`
	TotalMemoryMB     int     `json:"total_memory_mb" db:"total_memory_mb"`
	AllocatedVCPU     int     `json:"allocated_vcpu" db:"allocated_vcpu"`
	AllocatedMemoryMB int     `json:"allocated_memory_mb" db:"allocated_memory_mb"`
	// StorageBackend: "ceph" or "qcow". Defaults to "ceph" for backward compatibility.
	StorageBackend string `json:"storage_backend" db:"storage_backend"`
	// StoragePath: base directory for qcow2 file storage. Empty for ceph.
	StoragePath string `json:"storage_path,omitempty" db:"storage_path"`
	// CephPool: Ceph pool name for backward compatibility.
	CephPool string `json:"ceph_pool" db:"ceph_pool"`
	// CephMonitors: comma-separated Ceph monitor addresses.
	CephMonitors string `json:"ceph_monitors" db:"ceph_monitors"`
	// CephUser: Ceph authentication user.
	CephUser                   string     `json:"ceph_user" db:"ceph_user"`
	IPMIAddress                *string    `json:"-" db:"ipmi_address"`            // Sensitive
	IPMIUsernameEncrypted      *string    `json:"-" db:"ipmi_username_encrypted"` // Sensitive
	IPMIPasswordEncrypted      *string    `json:"-" db:"ipmi_password_encrypted"` // Sensitive
	LastHeartbeatAt            *time.Time `json:"last_heartbeat_at,omitempty" db:"last_heartbeat_at"`
	ConsecutiveHeartbeatMisses int        `json:"consecutive_heartbeat_misses" db:"consecutive_heartbeat_misses"`
	CreatedAt                  time.Time  `json:"created_at" db:"created_at"`
}

// NodeCreateRequest holds the fields required to register a new hypervisor node.
type NodeCreateRequest struct {
	Hostname       string  `json:"hostname" validate:"required,max=255"`
	GRPCAddress    string  `json:"grpc_address" validate:"required,max=255"`
	ManagementIP   string  `json:"management_ip" validate:"required,ip"`
	LocationID     *string `json:"location_id,omitempty" validate:"omitempty,uuid"`
	TotalVCPU      int     `json:"total_vcpu" validate:"required,min=1"`
	TotalMemoryMB  int     `json:"total_memory_mb" validate:"required,min=1024"`
	StorageBackend string  `json:"storage_backend" validate:"required,oneof=ceph qcow"`
	StoragePath    string  `json:"storage_path,omitempty" validate:"omitempty,max=500"`
	CephPool       string  `json:"ceph_pool" validate:"omitempty,max=100"`
	IPMIAddress    *string `json:"ipmi_address,omitempty" validate:"omitempty,ip"`
	IPMIUsername   *string `json:"ipmi_username,omitempty"`
	IPMIPassword   *string `json:"ipmi_password,omitempty"`
}

// NodeHeartbeat represents a periodic health report submitted by a hypervisor node agent.
type NodeHeartbeat struct {
	ID            int64     `json:"id" db:"id"`
	NodeID        string    `json:"node_id" db:"node_id"`
	Timestamp     time.Time `json:"timestamp" db:"timestamp"`
	CPUPercent    float32   `json:"cpu_percent" db:"cpu_percent"`
	MemoryPercent float32   `json:"memory_percent" db:"memory_percent"`
	DiskPercent   float32   `json:"disk_percent" db:"disk_percent"`
	TotalDiskGB   int64     `json:"total_disk_gb" db:"total_disk_gb"`
	UsedDiskGB    int64     `json:"used_disk_gb" db:"used_disk_gb"`
	CephConnected bool      `json:"ceph_connected" db:"ceph_connected"`
	CephTotalGB   int64     `json:"ceph_total_gb" db:"ceph_total_gb"`
	CephUsedGB    int64     `json:"ceph_used_gb" db:"ceph_used_gb"`
	VMCount       int       `json:"vm_count" db:"vm_count"`
	LoadAverage   []float32 `json:"load_average" db:"load_average"`
}

// NodeListFilter holds query parameters for filtering and paginating node list results.
type NodeListFilter struct {
	Status     *string `form:"status"`
	LocationID *string `form:"location_id"`
	PaginationParams
}

// NodeStatus represents the health and operational status of a node.
// It aggregates information from the node record and recent heartbeats.
type NodeStatus struct {
	NodeID                     string     `json:"node_id"`
	Hostname                   string     `json:"hostname"`
	Status                     string     `json:"status"`
	LastHeartbeatAt            *time.Time `json:"last_heartbeat_at,omitempty"`
	ConsecutiveHeartbeatMisses int        `json:"consecutive_heartbeat_misses"`
	TotalVCPU                  int        `json:"total_vcpu"`
	AllocatedVCPU              int        `json:"allocated_vcpu"`
	AvailableVCPU              int        `json:"available_vcpu"`
	TotalMemoryMB              int        `json:"total_memory_mb"`
	AllocatedMemoryMB          int        `json:"allocated_memory_mb"`
	AvailableMemoryMB          int        `json:"available_memory_mb"`
	VMCount                    int        `json:"vm_count"`
	CPUPercent                 float32    `json:"cpu_percent,omitempty"`
	MemoryPercent              float32    `json:"memory_percent,omitempty"`
	DiskPercent                float32    `json:"disk_percent,omitempty"`
	TotalDiskGB                int64      `json:"total_disk_gb,omitempty"`
	UsedDiskGB                 int64      `json:"used_disk_gb,omitempty"`
	CephStatus                 string     `json:"ceph_status,omitempty"`
	CephTotalGB                int64      `json:"ceph_total_gb,omitempty"`
	CephUsedGB                 int64      `json:"ceph_used_gb,omitempty"`
	LoadAverage                []float32  `json:"load_average,omitempty"`
	IsHealthy                  bool       `json:"is_healthy"`
}
