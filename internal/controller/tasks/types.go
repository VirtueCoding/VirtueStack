// Package tasks provides task types and worker for async job processing.
package tasks

import (
	"encoding/json"
	"time"
)

// TaskStatus represents the status of a task.
type TaskStatus string

// Task status constants.
const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// Task type constants.
const (
	TaskTypeVMCreate      = "vm.create"
	TaskTypeVMReinstall   = "vm.reinstall"
	TaskTypeVMMigrate     = "vm.migrate"
	TaskTypeVMResize      = "vm.resize"
	TaskTypeBackupCreate  = "backup.create"
	TaskTypeBackupRestore = "backup.restore"
)

// Task represents an async task to be processed.
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

// IsTerminal returns true if the task is in a terminal state.
func (t *Task) IsTerminal() bool {
	return t.Status == TaskStatusCompleted ||
		t.Status == TaskStatusFailed ||
		t.Status == TaskStatusCancelled
}

// SetRunning updates the task to running status.
func (t *Task) SetRunning() {
	t.Status = TaskStatusRunning
	now := time.Now().UTC()
	t.StartedAt = &now
}

// SetCompleted updates the task to completed status with an optional result.
func (t *Task) SetCompleted(result json.RawMessage) {
	t.Status = TaskStatusCompleted
	t.Progress = 100
	now := time.Now().UTC()
	t.CompletedAt = &now
	if result != nil {
		t.Result = result
	}
}

// SetFailed updates the task to failed status with an error message.
func (t *Task) SetFailed(errMsg string) {
	t.Status = TaskStatusFailed
	t.ErrorMessage = errMsg
	now := time.Now().UTC()
	t.CompletedAt = &now
}

// SetProgress updates the task progress (0-100).
func (t *Task) SetProgress(progress int) {
	if progress < 0 {
		progress = 0
	} else if progress > 100 {
		progress = 100
	}
	t.Progress = progress
}

// NewTask creates a new task with the given type and payload.
func NewTask(id, taskType string, payload json.RawMessage) *Task {
	return &Task{
		ID:        id,
		Type:      taskType,
		Status:    TaskStatusPending,
		Payload:   payload,
		Progress:  0,
		CreatedAt: time.Now().UTC(),
	}
}

// NewTaskWithCreator creates a new task with creator information.
func NewTaskWithCreator(id, taskType string, payload json.RawMessage, createdBy string) *Task {
	task := NewTask(id, taskType, payload)
	task.CreatedBy = createdBy
	return task
}

// TaskPayload is a helper for constructing task payloads.
type TaskPayload map[string]any

// ToJSON converts the payload to JSON RawMessage.
func (p TaskPayload) ToJSON() (json.RawMessage, error) {
	return json.Marshal(p)
}



