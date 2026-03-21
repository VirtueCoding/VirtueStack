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

// Backup represents a full backup of a VM's disk.
type Backup struct {
	ID   string `json:"id" db:"id"`
	VMID string `json:"vm_id" db:"vm_id"`
	Type string `json:"type" db:"type"` // "full"
	// StorageBackend indicates the storage type: "ceph" or "qcow"
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
	// StorageBackend indicates the storage type: "ceph" or "qcow"
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
