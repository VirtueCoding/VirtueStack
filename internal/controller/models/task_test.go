package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedClock is a Clock that always returns the same time, for deterministic tests.
type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }

func TestTaskIsTerminal(t *testing.T) {
	tests := []struct {
		name   string
		status TaskStatus
		want   bool
	}{
		{"pending is not terminal", TaskStatusPending, false},
		{"running is not terminal", TaskStatusRunning, false},
		{"completed is terminal", TaskStatusCompleted, true},
		{"failed is terminal", TaskStatusFailed, true},
		{"cancelled is terminal", TaskStatusCancelled, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{Status: tt.status}
			assert.Equal(t, tt.want, task.IsTerminal())
		})
	}
}

func TestTaskSetRunning(t *testing.T) {
	fixed := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	SetClock(fixedClock{now: fixed})
	defer ResetClock()

	task := &Task{Status: TaskStatusPending}
	task.SetRunning()

	assert.Equal(t, TaskStatusRunning, task.Status)
	require.NotNil(t, task.StartedAt)
	assert.Equal(t, fixed, *task.StartedAt)
}

func TestTaskSetCompleted(t *testing.T) {
	fixed := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	SetClock(fixedClock{now: fixed})
	defer ResetClock()

	t.Run("with result", func(t *testing.T) {
		task := &Task{Status: TaskStatusRunning}
		result := json.RawMessage(`{"vm_id":"abc"}`)
		task.SetCompleted(result)

		assert.Equal(t, TaskStatusCompleted, task.Status)
		assert.Equal(t, 100, task.Progress)
		require.NotNil(t, task.CompletedAt)
		assert.Equal(t, fixed, *task.CompletedAt)
		assert.JSONEq(t, `{"vm_id":"abc"}`, string(task.Result))
	})

	t.Run("with nil result", func(t *testing.T) {
		task := &Task{Status: TaskStatusRunning}
		task.SetCompleted(nil)

		assert.Equal(t, TaskStatusCompleted, task.Status)
		assert.Equal(t, 100, task.Progress)
		assert.Nil(t, task.Result)
	})
}

func TestTaskSetFailed(t *testing.T) {
	fixed := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	SetClock(fixedClock{now: fixed})
	defer ResetClock()

	task := &Task{Status: TaskStatusRunning}
	task.SetFailed("disk full")

	assert.Equal(t, TaskStatusFailed, task.Status)
	assert.Equal(t, "disk full", task.ErrorMessage)
	require.NotNil(t, task.CompletedAt)
	assert.Equal(t, fixed, *task.CompletedAt)
}

func TestTaskSetProgress(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{"normal value", 50, 50},
		{"zero", 0, 0},
		{"hundred", 100, 100},
		{"negative clamped to zero", -10, 0},
		{"over 100 clamped to 100", 150, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{}
			task.SetProgress(tt.input)
			assert.Equal(t, tt.expected, task.Progress)
		})
	}
}

func TestNewTask(t *testing.T) {
	fixed := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	SetClock(fixedClock{now: fixed})
	defer ResetClock()

	payload := json.RawMessage(`{"vm_id":"test-123"}`)
	task := NewTask("task-1", TaskTypeVMCreate, payload)

	assert.Equal(t, "task-1", task.ID)
	assert.Equal(t, TaskTypeVMCreate, task.Type)
	assert.Equal(t, TaskStatusPending, task.Status)
	assert.Equal(t, 0, task.Progress)
	assert.Equal(t, fixed, task.CreatedAt)
	assert.JSONEq(t, `{"vm_id":"test-123"}`, string(task.Payload))
	assert.Empty(t, task.CreatedBy)
}

func TestNewTaskWithCreator(t *testing.T) {
	fixed := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	SetClock(fixedClock{now: fixed})
	defer ResetClock()

	payload := json.RawMessage(`{}`)
	task := NewTaskWithCreator("task-2", TaskTypeBackupCreate, payload, "admin-1")

	assert.Equal(t, "task-2", task.ID)
	assert.Equal(t, TaskTypeBackupCreate, task.Type)
	assert.Equal(t, "admin-1", task.CreatedBy)
	assert.Equal(t, TaskStatusPending, task.Status)
}

func TestTaskPayload_ToJSON(t *testing.T) {
	t.Run("valid payload", func(t *testing.T) {
		p := TaskPayload{
			"vm_id":   "abc-123",
			"node_id": "node-1",
		}
		data, err := p.ToJSON()
		require.NoError(t, err)

		var result map[string]string
		err = json.Unmarshal(data, &result)
		require.NoError(t, err)
		assert.Equal(t, "abc-123", result["vm_id"])
		assert.Equal(t, "node-1", result["node_id"])
	})

	t.Run("empty payload", func(t *testing.T) {
		p := TaskPayload{}
		data, err := p.ToJSON()
		require.NoError(t, err)
		assert.JSONEq(t, `{}`, string(data))
	})
}

func TestTaskLifecycle(t *testing.T) {
	fixed := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	SetClock(fixedClock{now: fixed})
	defer ResetClock()

	// Create → Running → Progress → Completed
	task := NewTask("task-lc", TaskTypeVMReinstall, nil)
	assert.False(t, task.IsTerminal())

	task.SetRunning()
	assert.False(t, task.IsTerminal())

	task.SetProgress(50)
	assert.Equal(t, 50, task.Progress)

	task.SetCompleted(json.RawMessage(`{"ok":true}`))
	assert.True(t, task.IsTerminal())
	assert.Equal(t, 100, task.Progress)
}
