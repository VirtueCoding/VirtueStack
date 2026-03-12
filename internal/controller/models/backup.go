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

// Backup represents a full or incremental backup of a VM's disk.
type Backup struct {
	ID               string     `json:"id" db:"id"`
	VMID             string     `json:"vm_id" db:"vm_id"`
	Type             string     `json:"type" db:"type"` // "full" or "incremental"
	RBDSnapshot      *string    `json:"rbd_snapshot,omitempty" db:"rbd_snapshot"`
	DiffFromSnapshot *string    `json:"diff_from_snapshot,omitempty" db:"diff_from_snapshot"`
	StoragePath      *string    `json:"storage_path,omitempty" db:"storage_path"`
	SizeBytes        *int64     `json:"size_bytes,omitempty" db:"size_bytes"`
	Status           string     `json:"status" db:"status"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty" db:"expires_at"`
}

// Snapshot represents a point-in-time snapshot of a VM's disk stored in Ceph.
type Snapshot struct {
	ID          string    `json:"id" db:"id"`
	VMID        string    `json:"vm_id" db:"vm_id"`
	Name        string    `json:"name" db:"name"`
	RBDSnapshot string    `json:"rbd_snapshot" db:"rbd_snapshot"`
	SizeBytes   *int64    `json:"size_bytes,omitempty" db:"size_bytes"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// WebhookEvent constants define the event types that can trigger a webhook delivery.
const (
	WebhookEventVMCreated   = "vm.created"
	WebhookEventVMDeleted   = "vm.deleted"
	WebhookEventVMStarted   = "vm.started"
	WebhookEventVMStopped   = "vm.stopped"
	WebhookEventVMReinstall = "vm.reinstalled"
	WebhookEventBackupDone  = "backup.completed"
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
	ID         string    `json:"id" db:"id"`
	CustomerID string    `json:"customer_id" db:"customer_id"`
	URL        string    `json:"url" db:"url"`
	SecretHash string    `json:"-" db:"secret_hash"` // Never expose
	Events     []string  `json:"events" db:"events"`
	IsActive   bool      `json:"is_active" db:"is_active"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}

// WebhookDelivery represents a single delivery attempt for a webhook event.
type WebhookDelivery struct {
	ID             string     `json:"id" db:"id"`
	WebhookID      string     `json:"webhook_id" db:"webhook_id"`
	Event          string     `json:"event" db:"event"`
	Payload        string     `json:"payload" db:"payload"`
	AttemptCount   int        `json:"attempt_count" db:"attempt_count"`
	ResponseStatus *int       `json:"response_status,omitempty" db:"response_status"`
	ResponseBody   *string    `json:"response_body,omitempty" db:"response_body"`
	Success        bool       `json:"success" db:"success"`
	NextRetryAt    *time.Time `json:"next_retry_at,omitempty" db:"next_retry_at"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty" db:"delivered_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
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
