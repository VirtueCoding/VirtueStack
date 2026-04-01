// Package models provides data model types for VirtueStack Controller.
package models

import "time"

// Backup status constants define the lifecycle states of a backup.
const (
	BackupStatusCreating  = "creating"
	BackupStatusCompleted = "completed"
	BackupStatusFailed    = "failed"
	BackupStatusRestoring = "restoring"
	BackupStatusDeleted   = "deleted"
)

// Backup source constants define how a backup was initiated.
const (
	BackupSourceManual          = "manual"
	BackupSourceCustomerSchedule = "customer_schedule"
	BackupSourceAdminSchedule   = "admin_schedule"
)

// Backup method constants define the type of backup.
const (
	// BackupMethodFull is an exported full copy of the disk (traditional backup).
	BackupMethodFull = "full"
	// BackupMethodSnapshot is a fast point-in-time reference (in-place snapshot).
	BackupMethodSnapshot = "snapshot"
)

// Backup represents a point-in-time copy of a VM's disk.
// The Method field distinguishes between full backups and snapshots.
type Backup struct {
	ID   string `json:"id" db:"id"`
	VMID string `json:"vm_id" db:"vm_id"`
	// Method indicates backup type: "full" (exported copy) or "snapshot" (in-place reference)
	Method string `json:"method" db:"method"`
	// Name is an optional user-provided label (required for snapshots)
	Name *string `json:"name,omitempty" db:"name"`
	// Source indicates how the backup was initiated: "manual", "customer_schedule", or "admin_schedule"
	Source string `json:"source" db:"source"`
	// AdminScheduleID references the admin schedule that created this backup, if applicable
	AdminScheduleID *string `json:"admin_schedule_id,omitempty" db:"admin_schedule_id"`
	// StorageBackend indicates the storage type: "ceph", "qcow", or "lvm"
	StorageBackend string `json:"storage_backend" db:"storage_backend"`
	// RBDSnapshot is the RBD snapshot name for Ceph backups (backward compatibility)
	RBDSnapshot *string `json:"rbd_snapshot,omitempty" db:"rbd_snapshot"`
	// FilePath is the path to the backup file for QCOW backups
	FilePath *string `json:"file_path,omitempty" db:"file_path"`
	// SnapshotName is the internal QCOW snapshot name for QCOW backups
	SnapshotName *string    `json:"snapshot_name,omitempty" db:"snapshot_name"`
	StoragePath  *string    `json:"storage_path,omitempty" db:"storage_path"`
	SizeBytes    *int64     `json:"size_bytes,omitempty" db:"size_bytes"`
	Status       string     `json:"status" db:"status"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty" db:"expires_at"`
}

// Snapshot represents a point-in-time snapshot of a VM's disk.
type Snapshot struct {
	ID   string `json:"id" db:"id"`
	VMID string `json:"vm_id" db:"vm_id"`
	Name string `json:"name" db:"name"`
	// StorageBackend indicates the storage type: "ceph", "qcow", or "lvm"
	StorageBackend string `json:"storage_backend" db:"storage_backend"`
	// RBDSnapshot is the RBD snapshot name for Ceph storage
	RBDSnapshot string `json:"rbd_snapshot" db:"rbd_snapshot"`
	// QCOWSnapshot is the internal qemu-img snapshot name for QCOW storage
	QCOWSnapshot *string   `json:"qcow_snapshot,omitempty" db:"qcow_snapshot"`
	SizeBytes    *int64    `json:"size_bytes,omitempty" db:"size_bytes"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// WebhookEvent constants define the event types that can trigger a webhook delivery.
const (
	WebhookEventVMCreated     = "vm.created"
	WebhookEventVMDeleted     = "vm.deleted"
	WebhookEventVMStarted     = "vm.started"
	WebhookEventVMStopped     = "vm.stopped"
	WebhookEventVMReinstall   = "vm.reinstalled"
	WebhookEventVMMigrated    = "vm.migrated"
	WebhookEventBackupDone    = "backup.completed"
	WebhookEventBackupFail    = "backup.failed"
	WebhookEventSnapshotDone  = "snapshot.created"
	WebhookEventBandwidthThresh = "bandwidth.threshold"
)

// BackupSchedule represents a scheduled backup configuration.
type BackupSchedule struct {
	ID             string    `json:"id" db:"id"`
	VMID           string    `json:"vm_id" db:"vm_id"`
	CustomerID     string    `json:"customer_id" db:"customer_id"`
	Frequency      string    `json:"frequency" db:"frequency"`
	RetentionCount int       `json:"retention_count" db:"retention_count"`
	Active         bool      `json:"active" db:"active"`
	NextRunAt      time.Time `json:"next_run_at" db:"next_run_at"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// CustomerWebhook represents a webhook endpoint registered by a customer.
type CustomerWebhook struct {
	ID            string     `json:"id" db:"id"`
	CustomerID    string     `json:"customer_id" db:"customer_id"`
	URL           string     `json:"url" db:"url"`
	SecretHash    string     `json:"-" db:"secret_hash"` // Never expose in JSON; stores the encrypted secret
	Events        []string   `json:"events" db:"events"`
	IsActive      bool       `json:"active" db:"active"`
	FailCount     int        `json:"fail_count" db:"fail_count"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty" db:"last_success_at"`
	LastFailureAt *time.Time `json:"last_failure_at,omitempty" db:"last_failure_at"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
}

// WebhookDelivery represents a single delivery attempt for a webhook event.
type WebhookDelivery struct {
	ID             string     `json:"id" db:"id"`
	WebhookID      string     `json:"webhook_id" db:"webhook_id"`
	Event          string     `json:"event" db:"event"`
	IdempotencyKey string     `json:"idempotency_key" db:"idempotency_key"`
	Payload        []byte     `json:"payload" db:"payload"`
	Status         string     `json:"status" db:"status"`
	AttemptCount   int        `json:"attempt_count" db:"attempt_count"`
	MaxAttempts    int        `json:"max_attempts" db:"max_attempts"`
	NextRetryAt    *time.Time `json:"next_retry_at,omitempty" db:"next_retry_at"`
	ResponseStatus *int       `json:"response_status,omitempty" db:"response_status"`
	ResponseBody   *string    `json:"response_body,omitempty" db:"response_body"`
	ErrorMessage   *string    `json:"error_message,omitempty" db:"error_message"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty" db:"delivered_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

// SnapshotCreateRequest holds the fields required to create a new VM snapshot.
type SnapshotCreateRequest struct {
	Name string `json:"name" validate:"required,max=100"`
}

// AdminBackupSchedule represents a mass backup campaign targeting multiple VMs.
type AdminBackupSchedule struct {
	ID              string     `json:"id" db:"id"`
	Name            string     `json:"name" db:"name"`
	Description     *string    `json:"description,omitempty" db:"description"`
	Frequency       string     `json:"frequency" db:"frequency"` // "daily", "weekly", "monthly"
	RetentionCount  int        `json:"retention_count" db:"retention_count"`
	TargetAll       bool       `json:"target_all" db:"target_all"`
	TargetPlanIDs   []string   `json:"target_plan_ids,omitempty" db:"target_plan_ids"`
	TargetNodeIDs   []string   `json:"target_node_ids,omitempty" db:"target_node_ids"`
	TargetCustomerIDs []string `json:"target_customer_ids,omitempty" db:"target_customer_ids"`
	Active          bool       `json:"active" db:"active"`
	NextRunAt       time.Time  `json:"next_run_at" db:"next_run_at"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty" db:"last_run_at"`
	CreatedBy       *string    `json:"created_by,omitempty" db:"created_by"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

// BackupScheduleFrequency constants for customer backup schedules.
const (
	BackupScheduleFrequencyDaily   = "daily"
	BackupScheduleFrequencyWeekly  = "weekly"
	BackupScheduleFrequencyMonthly = "monthly"
)

// AdminBackupScheduleFrequency constants define the valid frequency values.
const (
	AdminBackupScheduleFrequencyDaily   = "daily"
	AdminBackupScheduleFrequencyWeekly  = "weekly"
	AdminBackupScheduleFrequencyMonthly = "monthly"
)

// CalculateNextRunTime calculates the next run time based on frequency.
func CalculateNextRunTime(frequency string, from time.Time) time.Time {
	switch frequency {
	case AdminBackupScheduleFrequencyDaily:
		return from.Add(24 * time.Hour)
	case AdminBackupScheduleFrequencyWeekly:
		return from.Add(7 * 24 * time.Hour)
	case AdminBackupScheduleFrequencyMonthly:
		return from.AddDate(0, 1, 0)
	default:
		return from.Add(24 * time.Hour)
	}
}

// CustomerAPIKeyCreateRequest holds the fields required to create a new customer API key.
type CustomerAPIKeyCreateRequest struct {
	Name        string   `json:"name" validate:"required,max=100"`
	Permissions []string `json:"permissions" validate:"required,min=1,dive,max=100"`
	ExpiresAt   *string  `json:"expires_at,omitempty"` // RFC3339 timestamp
}

// CustomerWebhookCreateRequest holds the fields required to register a new webhook endpoint.
type CustomerWebhookCreateRequest struct {
	URL    string   `json:"url" validate:"required,url,max=2048"`
	Secret string   `json:"secret" validate:"required,min=16,max=128"`
	Events []string `json:"events" validate:"required,min=1,dive,max=100"`
}
