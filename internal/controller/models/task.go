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

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

const (
	TaskTypeVMCreate       = "vm.create"
	TaskTypeVMReinstall    = "vm.reinstall"
	TaskTypeVMMigrate      = "vm.migrate"
	TaskTypeVMResize       = "vm.resize"
	TaskTypeVMDelete       = "vm.delete"
	TaskTypeBackupCreate   = "backup.create"
	TaskTypeBackupRestore  = "backup.restore"
	TaskTypeSnapshotCreate = "snapshot.create"
	TaskTypeSnapshotRevert = "snapshot.revert"
	TaskTypeSnapshotDelete = "snapshot.delete"
)

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

func (t *Task) IsTerminal() bool {
	return t.Status == TaskStatusCompleted ||
		t.Status == TaskStatusFailed ||
		t.Status == TaskStatusCancelled
}

func (t *Task) SetRunning() {
	t.Status = TaskStatusRunning
	now := clock.Now()
	t.StartedAt = &now
}

func (t *Task) SetCompleted(result json.RawMessage) {
	t.Status = TaskStatusCompleted
	t.Progress = 100
	now := clock.Now()
	t.CompletedAt = &now
	if result != nil {
		t.Result = result
	}
}

func (t *Task) SetFailed(errMsg string) {
	t.Status = TaskStatusFailed
	t.ErrorMessage = errMsg
	now := clock.Now()
	t.CompletedAt = &now
}

func (t *Task) SetProgress(progress int) {
	if progress < 0 {
		progress = 0
	} else if progress > 100 {
		progress = 100
	}
	t.Progress = progress
}

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

func (p TaskPayload) ToJSON() (json.RawMessage, error) {
	return json.Marshal(p)
}
