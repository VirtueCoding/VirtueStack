package tasks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockWorkerTaskRepo struct {
	findStuckTasksFunc func(ctx context.Context, threshold time.Duration) ([]*models.Task, error)
	retryTaskFunc      func(ctx context.Context, taskID string) error
	setFailedFunc      func(ctx context.Context, id string, errorMessage string) error
}

func (m *mockWorkerTaskRepo) FindStuckTasks(ctx context.Context, threshold time.Duration) ([]*models.Task, error) {
	if m.findStuckTasksFunc != nil {
		return m.findStuckTasksFunc(ctx, threshold)
	}
	return nil, nil
}

func (m *mockWorkerTaskRepo) RetryTask(ctx context.Context, taskID string) error {
	if m.retryTaskFunc != nil {
		return m.retryTaskFunc(ctx, taskID)
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
		wantRetryIDs    []string
		wantFailedIDs   []string
		retryErrTaskID  string
		failedErrTaskID string
	}{
		{
			name: "stuck task older than threshold is reset to pending",
			stuckTasks: []*models.Task{
				{ID: "task-1", Type: models.TaskTypeVMCreate, RetryCount: 1},
			},
			wantRetryIDs: []string{"task-1"},
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
			retryErrTaskID: "task-3",
			wantRetryIDs:   []string{"task-4"},
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
			retryCalls := make([]string, 0)
			failedCalls := make([]string, 0)

			repo := &mockWorkerTaskRepo{
				findStuckTasksFunc: func(ctx context.Context, threshold time.Duration) ([]*models.Task, error) {
					return tt.stuckTasks, tt.findErr
				},
				retryTaskFunc: func(ctx context.Context, taskID string) error {
					if taskID == tt.retryErrTaskID {
						return errors.New("retry failed")
					}
					retryCalls = append(retryCalls, taskID)
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

			wantRetry := tt.wantRetryIDs
			if wantRetry == nil {
				wantRetry = []string{}
			}
			wantFailed := tt.wantFailedIDs
			if wantFailed == nil {
				wantFailed = []string{}
			}

			assert.Equal(t, wantRetry, retryCalls)
			assert.Equal(t, wantFailed, failedCalls)
		})
	}
}

func TestStartStuckTaskScanner(t *testing.T) {
	var callCount atomic.Int32
	repo := &mockWorkerTaskRepo{
		findStuckTasksFunc: func(ctx context.Context, threshold time.Duration) ([]*models.Task, error) {
			callCount.Add(1)
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

	// Poll until the scanner has run at least once instead of using time.Sleep
	require.Eventually(t, func() bool {
		return callCount.Load() >= 1
	}, time.Second, 5*time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scanner did not stop after context cancellation")
	}

	assert.GreaterOrEqual(t, int(callCount.Load()), 1)
}
