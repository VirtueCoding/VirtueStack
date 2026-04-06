package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/nats-io/nats.go"
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

type mockStuckScannerJetStream struct {
	publishFunc func(subject string, data []byte, opts ...nats.PubOpt) (*nats.PubAck, error)
}

func (m *mockStuckScannerJetStream) AddStream(*nats.StreamConfig, ...nats.JSOpt) (*nats.StreamInfo, error) {
	return nil, errors.New("unexpected AddStream")
}

func (m *mockStuckScannerJetStream) AddConsumer(string, *nats.ConsumerConfig, ...nats.JSOpt) (*nats.ConsumerInfo, error) {
	return nil, errors.New("unexpected AddConsumer")
}

func (m *mockStuckScannerJetStream) QueueSubscribe(string, string, nats.MsgHandler, ...nats.SubOpt) (*nats.Subscription, error) {
	return nil, errors.New("unexpected QueueSubscribe")
}

func (m *mockStuckScannerJetStream) Publish(subject string, data []byte, opts ...nats.PubOpt) (*nats.PubAck, error) {
	if m.publishFunc != nil {
		return m.publishFunc(subject, data, opts...)
	}
	return &nats.PubAck{}, nil
}

func TestRecoverStuckTasks(t *testing.T) {
	tests := []struct {
		name             string
		stuckTasks       []*models.Task
		findErr          error
		wantRetryIDs     []string
		wantFailedIDs    []string
		wantPublishedIDs []string
		retryErrTaskID   string
		failedErrTaskID  string
		publishErrTaskID string
		wantFailedMsg    string
	}{
		{
			name: "stuck task older than threshold is reset to pending",
			stuckTasks: []*models.Task{
				{ID: "task-1", Type: models.TaskTypeVMCreate, RetryCount: 1, Payload: json.RawMessage(`{"vm_id":"vm-1"}`)},
			},
			wantRetryIDs:     []string{"task-1"},
			wantPublishedIDs: []string{"task-1"},
		},
		{
			name: "task at max retries is marked failed",
			stuckTasks: []*models.Task{
				{ID: "task-2", Type: models.TaskTypeVMCreate, RetryCount: maxTaskRetries},
			},
			wantFailedIDs: []string{"task-2"},
			wantFailedMsg: stuckTaskRecoveredMessage,
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
				{ID: "task-3", Type: models.TaskTypeVMCreate, RetryCount: 0, Payload: json.RawMessage(`{"vm_id":"vm-3"}`)},
				{ID: "task-4", Type: models.TaskTypeVMCreate, RetryCount: 0, Payload: json.RawMessage(`{"vm_id":"vm-4"}`)},
			},
			retryErrTaskID:   "task-3",
			wantRetryIDs:     []string{"task-4"},
			wantPublishedIDs: []string{"task-4"},
		},
		{
			name: "set failed error continues processing",
			stuckTasks: []*models.Task{
				{ID: "task-5", Type: models.TaskTypeVMCreate, RetryCount: maxTaskRetries},
				{ID: "task-6", Type: models.TaskTypeVMCreate, RetryCount: maxTaskRetries},
			},
			failedErrTaskID: "task-5",
			wantFailedIDs:   []string{"task-6"},
			wantFailedMsg:   stuckTaskRecoveredMessage,
		},
		{
			name: "publish failure marks recovered task failed",
			stuckTasks: []*models.Task{
				{ID: "task-7", Type: models.TaskTypeVMCreate, RetryCount: 1, Payload: json.RawMessage(`{"vm_id":"vm-7"}`)},
			},
			wantRetryIDs:     []string{"task-7"},
			wantFailedIDs:    []string{"task-7"},
			publishErrTaskID: "task-7",
			wantFailedMsg:    "publishing task: publish failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retryCalls := make([]string, 0)
			failedCalls := make([]string, 0)
			publishedTaskIDs := make([]string, 0)
			publishedStatuses := make([]models.TaskStatus, 0)

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
					require.Equal(t, tt.wantFailedMsg, errorMessage)
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
				js: &mockStuckScannerJetStream{
					publishFunc: func(_ string, data []byte, _ ...nats.PubOpt) (*nats.PubAck, error) {
						var task models.Task
						require.NoError(t, json.Unmarshal(data, &task))
						if task.ID == tt.publishErrTaskID {
							return nil, errors.New("publish failed")
						}
						publishedTaskIDs = append(publishedTaskIDs, task.ID)
						publishedStatuses = append(publishedStatuses, task.Status)
						return &nats.PubAck{}, nil
					},
				},
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
			wantPublished := tt.wantPublishedIDs
			if wantPublished == nil {
				wantPublished = []string{}
			}

			assert.Equal(t, wantRetry, retryCalls)
			assert.Equal(t, wantFailed, failedCalls)
			assert.Equal(t, wantPublished, publishedTaskIDs)
			for _, status := range publishedStatuses {
				assert.Equal(t, models.TaskStatusPending, status)
			}
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
