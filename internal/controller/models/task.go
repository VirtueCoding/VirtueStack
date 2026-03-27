// Package models provides data model types for VirtueStack Controller.
package models

import (
	"encoding/json"
	"time"
)

// Clock provides the current time. It is injected to enable deterministic testing.
// Production code uses DefaultClock which delegates to time.Now().
type Clock interface {
	Now() time.Time
}

// defaultClock is the production implementation of Clock.
type defaultClock struct{}

func (defaultClock) Now() time.Time { return time.Now().UTC() }

// DefaultClock is the Clock implementation used in production.
var DefaultClock Clock = defaultClock{}

// clock is the package-level clock instance. It can be replaced in tests.
var clock Clock = DefaultClock

// SetClock replaces the package clock for testing purposes.
// This must be called before any Task methods are invoked.
func SetClock(c Clock) { clock = c }

// ResetClock restores the default clock.
func ResetClock() { clock = DefaultClock }

// TaskStatus represents the lifecycle state of an async task.
type TaskStatus string

// Task status constants define the lifecycle states of an async task.
const (
	// TaskStatusPending indicates the task is waiting to be processed.
	TaskStatusPending TaskStatus = "pending"
	// TaskStatusRunning indicates the task is currently being processed.
	TaskStatusRunning TaskStatus = "running"
	// TaskStatusCompleted indicates the task finished successfully.
	TaskStatusCompleted TaskStatus = "completed"
	// TaskStatusFailed indicates the task encountered an error.
	TaskStatusFailed TaskStatus = "failed"
	// TaskStatusCancelled indicates the task was cancelled before completion.
	TaskStatusCancelled TaskStatus = "cancelled"
)

// Task type constants define the kinds of async operations.
const (
	// TaskTypeVMCreate creates a new virtual machine.
	TaskTypeVMCreate = "vm.create"
	// TaskTypeVMReinstall reinstalls a VM with a new template.
	TaskTypeVMReinstall = "vm.reinstall"
	// TaskTypeVMMigrate migrates a VM to a different node.
	TaskTypeVMMigrate = "vm.migrate"
	// TaskTypeVMResize resizes a VM's resources.
	TaskTypeVMResize = "vm.resize"
	// TaskTypeVMDelete deletes a virtual machine.
	TaskTypeVMDelete = "vm.delete"
	// TaskTypeBackupCreate creates a backup of a VM.
	TaskTypeBackupCreate = "backup.create"
	// TaskTypeBackupRestore restores a VM from a backup.
	TaskTypeBackupRestore = "backup.restore"
	// TaskTypeSnapshotCreate creates a snapshot of a VM.
	TaskTypeSnapshotCreate = "snapshot.create"
	// TaskTypeSnapshotRevert reverts a VM to a snapshot.
	TaskTypeSnapshotRevert = "snapshot.revert"
	// TaskTypeSnapshotDelete deletes a VM snapshot.
	TaskTypeSnapshotDelete = "snapshot.delete"
	// TaskTypeTemplateBuild builds a VM template from an ISO image.
	TaskTypeTemplateBuild = "template.build_from_iso"
	// TaskTypeTemplateDistribute distributes a template to specified nodes.
	TaskTypeTemplateDistribute = "template.distribute"
)

// Task represents an async operation tracked in the database and NATS JetStream.
type Task struct {
	ID             string          `json:"id"`
	Type           string          `json:"type"`
	Status         TaskStatus      `json:"status"`
	Payload        json.RawMessage `json:"payload"`
	Result         json.RawMessage `json:"result,omitempty"`
	ErrorMessage   string          `json:"error_message,omitempty"`
	Progress       int             `json:"progress"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	CreatedBy      string          `json:"created_by,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
}

// IsTerminal returns true if the task has reached a terminal state (completed, failed, or cancelled).
func (t *Task) IsTerminal() bool {
	return t.Status == TaskStatusCompleted ||
		t.Status == TaskStatusFailed ||
		t.Status == TaskStatusCancelled
}

// SetRunning marks the task as running and sets the StartedAt timestamp.
func (t *Task) SetRunning() {
	t.Status = TaskStatusRunning
	now := clock.Now()
	t.StartedAt = &now
}

// SetCompleted marks the task as completed with the given result.
func (t *Task) SetCompleted(result json.RawMessage) {
	t.Status = TaskStatusCompleted
	t.Progress = 100
	now := clock.Now()
	t.CompletedAt = &now
	if result != nil {
		t.Result = result
	}
}

// SetFailed marks the task as failed with the given error message.
func (t *Task) SetFailed(errMsg string) {
	t.Status = TaskStatusFailed
	t.ErrorMessage = errMsg
	now := clock.Now()
	t.CompletedAt = &now
}

// SetProgress updates the task progress percentage, clamping to 0-100.
func (t *Task) SetProgress(progress int) {
	if progress < 0 {
		progress = 0
	} else if progress > 100 {
		progress = 100
	}
	t.Progress = progress
}

// NewTask creates a new task with the given ID, type, and payload.
func NewTask(id, taskType string, payload json.RawMessage) *Task {
	return &Task{
		ID:        id,
		Type:      taskType,
		Status:    TaskStatusPending,
		Payload:   payload,
		Progress:  0,
		CreatedAt: clock.Now(),
	}
}

// NewTaskWithCreator creates a new task with the given ID, type, payload, and creator.
func NewTaskWithCreator(id, taskType string, payload json.RawMessage, createdBy string) *Task {
	task := NewTask(id, taskType, payload)
	task.CreatedBy = createdBy
	return task
}

// TaskPayload is a convenience map for constructing ad-hoc task payloads before
// they are marshalled into the json.RawMessage stored on Task.Payload.  It is
// intentionally untyped (map[string]any) because each task type carries a
// different set of fields; per-task typed structs (e.g. WebhookDeliveryPayload
// in the tasks package) are used when *reading* payloads back from the queue.
// This type is only used at the write/publish side to avoid boilerplate.
type TaskPayload map[string]any

// ToJSON marshals the TaskPayload to JSON for storage in Task.Payload.
func (p TaskPayload) ToJSON() (json.RawMessage, error) {
	return json.Marshal(p)
}
