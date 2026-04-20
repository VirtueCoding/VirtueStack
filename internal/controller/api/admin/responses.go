package admin

import "time"

// NodeStatusResponse represents the response for node status operations.
type NodeStatusResponse struct {
	Status string `json:"status"`
}

// VMCreateResponse represents the response for VM creation operations.
type VMCreateResponse struct {
	VMID  string `json:"vm_id"`
	TaskID string `json:"task_id"`
}

// VMResizeResponse represents the response for VM resize operations.
type VMResizeResponse struct {
	TaskID   string `json:"task_id"`
	VMID     string `json:"vm_id"`
	VCPU     int    `json:"vcpu"`
	MemoryMB int    `json:"memory_mb"`
	DiskGB   int    `json:"disk_gb"`
}

// VMMigrateResponse represents the response for VM migration operations.
type VMMigrateResponse struct {
	VMID         string `json:"vm_id"`
	TargetNodeID string `json:"target_node_id"`
	TaskID       string `json:"task_id"`
	Status       string `json:"status"`
}

// BackupScheduleCreateResponse represents the response for backup schedule creation.
type BackupScheduleCreateResponse struct {
	ID        string    `json:"id"`
	NextRunAt time.Time `json:"next_run_at,omitempty"`
}

// MessageResponse represents a simple message response.
type MessageResponse struct {
	Message string `json:"message"`
}

// BackupRestoreResponse represents the response for backup restore operations.
type BackupRestoreResponse struct {
	BackupID string `json:"backup_id"`
	Status   string `json:"status"`
}