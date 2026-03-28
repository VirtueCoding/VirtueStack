package tasks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockWorkerTaskRepo struct {
	findStuckTasksFunc func(ctx context.Context, threshold time.Duration) ([]*models.Task, error)
	resetTaskFunc      func(ctx context.Context, taskID string) error
	setFailedFunc      func(ctx context.Context, id string, errorMessage string) error
}

func (m *mockWorkerTaskRepo) FindStuckTasks(ctx context.Context, threshold time.Duration) ([]*models.Task, error) {
	if m.findStuckTasksFunc != nil {
		return m.findStuckTasksFunc(ctx, threshold)
	}
	return nil, nil
}

func (m *mockWorkerTaskRepo) ResetTask(ctx context.Context, taskID string) error {
	if m.resetTaskFunc != nil {
		return m.resetTaskFunc(ctx, taskID)
	}
	return nil
}

func (m *mockWorkerTaskRepo) SetFailed(ctx context.Context, id string, errorMessage string) error {
	if m.setFailedFunc != nil {
		return m.setFailedFunc(ctx, id, errorMessage)
	}
	return nil
}

func TestRecoverStuckTasks(t *testing.T) {
	tests := []struct {
		name            string
		stuckTasks      []*models.Task
		findErr         error
		wantResetIDs    []string
		wantFailedIDs   []string
		resetErrTaskID  string
		failedErrTaskID string
	}{
		{
			name: "stuck task older than threshold is reset to pending",
			stuckTasks: []*models.Task{
				{ID: "task-1", Type: models.TaskTypeVMCreate, RetryCount: 1},
			},
			wantResetIDs: []string{"task-1"},
		},
		{
			name: "task at max retries is marked failed",
			stuckTasks: []*models.Task{
				{ID: "task-2", Type: models.TaskTypeVMCreate, RetryCount: maxTaskRetries},
			},
			wantFailedIDs: []string{"task-2"},
		},
		{
			name:       "empty result set does nothing",
			stuckTasks: []*models.Task{},
		},
		{
			name:    "find stuck tasks error handled gracefully",
			findErr: errors.New("db unavailable"),
		},
		{
			name: "reset failure continues processing",
			stuckTasks: []*models.Task{
				{ID: "task-3", Type: models.TaskTypeVMCreate, RetryCount: 0},
				{ID: "task-4", Type: models.TaskTypeVMCreate, RetryCount: 0},
			},
			resetErrTaskID: "task-3",
			wantResetIDs:   []string{"task-4"},
		},
		{
			name: "set failed error continues processing",
			stuckTasks: []*models.Task{
				{ID: "task-5", Type: models.TaskTypeVMCreate, RetryCount: maxTaskRetries},
				{ID: "task-6", Type: models.TaskTypeVMCreate, RetryCount: maxTaskRetries},
			},
			failedErrTaskID: "task-5",
			wantFailedIDs:   []string{"task-6"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetCalls := make([]string, 0)
			failedCalls := make([]string, 0)

			repo := &mockWorkerTaskRepo{
				findStuckTasksFunc: func(ctx context.Context, threshold time.Duration) ([]*models.Task, error) {
					return tt.stuckTasks, tt.findErr
				},
				resetTaskFunc: func(ctx context.Context, taskID string) error {
					if taskID == tt.resetErrTaskID {
						return errors.New("reset failed")
					}
					resetCalls = append(resetCalls, taskID)
					return nil
				},
				setFailedFunc: func(ctx context.Context, id string, errorMessage string) error {
					require.Equal(t, stuckTaskRecoveredMessage, errorMessage)
					if id == tt.failedErrTaskID {
						return errors.New("set failed failed")
					}
					failedCalls = append(failedCalls, id)
					return nil
				},
			}

			worker := &Worker{
				taskRepo: repo,
				logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
			}

			worker.recoverStuckTasks(context.Background(), 30*time.Minute)

			wantReset := tt.wantResetIDs
			if wantReset == nil {
				wantReset = []string{}
			}
			wantFailed := tt.wantFailedIDs
			if wantFailed == nil {
				wantFailed = []string{}
			}

			assert.Equal(t, wantReset, resetCalls)
			assert.Equal(t, wantFailed, failedCalls)
		})
	}
}

func TestStartStuckTaskScanner(t *testing.T) {
	callCount := 0
	repo := &mockWorkerTaskRepo{
		findStuckTasksFunc: func(ctx context.Context, threshold time.Duration) ([]*models.Task, error) {
			callCount++
			return []*models.Task{}, nil
		},
	}

	worker := &Worker{
		taskRepo: repo,
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.StartStuckTaskScanner(ctx, 10*time.Millisecond, 30*time.Minute)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scanner did not stop after context cancellation")
	}

	assert.GreaterOrEqual(t, callCount, 1)
}
