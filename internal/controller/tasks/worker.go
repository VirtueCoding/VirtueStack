// Package tasks provides task types and worker for async job processing.
package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	taskmetrics "github.com/AbuGosok/VirtueStack/internal/controller/metrics"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

// JetStream constants.
const (
	StreamName     = "TASKS"
	StreamSubject  = "tasks.>"
	ConsumerName   = "task-worker"
	AckWait        = 5 * time.Minute
	MaxDeliver     = 3
	maxTaskRetries = 3
)

const stuckTaskRecoveredMessage = "stuck task recovered after timeout"

// nakDelays defines the exponential backoff delays applied when a task fails and
// must be redelivered. The index corresponds to (NumDelivered - 1): first failure
// waits 10 s, second waits 60 s, subsequent failures wait 300 s.
var nakDelays = []time.Duration{
	10 * time.Second,
	60 * time.Second,
	300 * time.Second,
}

// nakDelay returns the backoff delay for the given delivery attempt number
// (1-based). Index 0 means first delivery, so failure index = numDelivered-1.
func nakDelay(numDelivered uint64) time.Duration {
	idx := int(numDelivered) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(nakDelays) {
		idx = len(nakDelays) - 1
	}
	return nakDelays[idx]
}

// TaskHandler processes a task of a specific type.
type TaskHandler func(ctx context.Context, task *models.Task) error

type workerTaskRepository interface {
	FindStuckTasks(ctx context.Context, threshold time.Duration) ([]*models.Task, error)
	ResetTask(ctx context.Context, taskID string) error
	SetFailed(ctx context.Context, id string, errorMessage string) error
}

// Worker processes tasks from NATS JetStream.
type Worker struct {
	js       nats.JetStreamContext
	dbPool   *pgxpool.Pool
	taskRepo workerTaskRepository
	handlers map[string]TaskHandler
	logger   *slog.Logger
	mu       sync.RWMutex
	cancel   context.CancelFunc
	eg       *errgroup.Group
	egCtx    context.Context
}

// NewWorker creates a new task worker.
func NewWorker(js nats.JetStreamContext, dbPool *pgxpool.Pool, logger *slog.Logger) (*Worker, error) {
	streamConfig := &nats.StreamConfig{
		Name:      StreamName,
		Subjects:  []string{StreamSubject},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		MaxAge:    7 * 24 * time.Hour,
		Replicas:  1,
	}

	_, err := js.AddStream(streamConfig)
	if err != nil {
		if !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
			return nil, fmt.Errorf("creating JetStream stream: %w", err)
		}
	}

	return &Worker{
		js:       js,
		dbPool:   dbPool,
		taskRepo: repository.NewTaskRepository(dbPool),
		handlers: make(map[string]TaskHandler),
		logger:   logger.With("component", "task-worker"),
	}, nil
}

// RegisterHandler registers a handler for a task type.
func (w *Worker) RegisterHandler(taskType string, handler TaskHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers[taskType] = handler
	w.logger.Info("registered task handler", "task_type", taskType)
}

// Start begins consuming tasks from the NATS JetStream stream.
func (w *Worker) Start(ctx context.Context, numWorkers int) error {
	ctx, cancel := context.WithCancel(ctx)
	w.cancel = cancel

	consumerConfig := &nats.ConsumerConfig{
		Durable:        ConsumerName,
		AckPolicy:      nats.AckExplicitPolicy,
		AckWait:        AckWait,
		MaxDeliver:     MaxDeliver,
		DeliverSubject: fmt.Sprintf("%s.%s.deliver", StreamName, ConsumerName),
		DeliverGroup:   ConsumerName,
	}

	_, err := w.js.AddConsumer(StreamName, consumerConfig)
	if err != nil {
		if !errors.Is(err, nats.ErrConsumerNameAlreadyInUse) {
			return fmt.Errorf("creating JetStream consumer: %w", err)
		}
	}

	sub, err := w.js.QueueSubscribe(
		StreamSubject,
		ConsumerName,
		w.handleMessage(),
		nats.Durable(ConsumerName),
		nats.DeliverAll(),
		nats.ManualAck(),
	)
	if err != nil {
		return fmt.Errorf("subscribing to tasks: %w", err)
	}

	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(numWorkers)
	w.eg = eg
	w.egCtx = egCtx

	w.logger.Info("task worker started",
		"stream", StreamName,
		"consumer", ConsumerName,
		"workers", numWorkers,
	)

	eg.Go(func() error {
		<-egCtx.Done()
		w.logger.Info("stopping task worker")
		if err := sub.Unsubscribe(); err != nil {
			w.logger.Warn("failed to unsubscribe from task stream", "error", err)
		}
		return nil
	})

	return nil
}

// StartStuckTaskScanner starts periodic scanning and recovery of stuck running tasks.
func (w *Worker) StartStuckTaskScanner(ctx context.Context, interval, stuckThreshold time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.recoverStuckTasks(ctx, stuckThreshold)
		}
	}
}

// Stop gracefully stops the worker.
func (w *Worker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	if w.eg != nil {
		_ = w.eg.Wait()
	}
	w.logger.Info("task worker stopped")
}

// handleMessage returns a message handler for NATS messages.
func (w *Worker) handleMessage() func(msg *nats.Msg) {
	return func(msg *nats.Msg) {
		w.eg.Go(func() error {
			return w.processMessage(w.egCtx, msg)
		})
	}
}

func (w *Worker) processMessage(ctx context.Context, msg *nats.Msg) error {
	var task models.Task
	if err := json.Unmarshal(msg.Data, &task); err != nil {
		w.logger.Error("failed to unmarshal task",
			"error", err,
			"subject", msg.Subject,
		)
		if err := msg.Ack(); err != nil {
			w.logger.Warn("failed to ack malformed task message", "error", err)
		}
		return nil
	}

	logger := w.logger.With(
		"task_id", task.ID,
		"task_type", task.Type,
	)

	logger.Info("processing task")

	taskStart := time.Now()

	handlerCtx, handlerCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer handlerCancel()

	if err := w.updateTaskStatus(handlerCtx, &task, models.TaskStatusRunning, nil, nil); err != nil {
		logger.Error("failed to update task status to running", "error", err)
		// Determine backoff delay from delivery count.
		var nakDelay_ time.Duration
		if meta, metaErr := msg.Metadata(); metaErr == nil {
			nakDelay_ = nakDelay(meta.NumDelivered)
		} else {
			nakDelay_ = nakDelays[0]
		}
		if err := msg.NakWithDelay(nakDelay_); err != nil {
			logger.Warn("failed to nak task message", "error", err)
		}
		return err
	}

	w.mu.RLock()
	handler, ok := w.handlers[task.Type]
	w.mu.RUnlock()

	if !ok {
		errMsg := fmt.Sprintf("no handler registered for task type: %s", task.Type)
		logger.Error(errMsg)
		// Error is intentionally discarded: the task is already being failed and the
		// handler context may be near expiry; the outer Ack() below ensures the message
		// is removed from the queue regardless of this status-update outcome.
		_ = w.updateTaskStatus(handlerCtx, &task, models.TaskStatusFailed, nil, &errMsg)
		taskmetrics.TasksTotal.WithLabelValues(task.Type, "failed").Inc()
		taskmetrics.TaskDuration.WithLabelValues(task.Type).Observe(time.Since(taskStart).Seconds())
		if err := msg.Ack(); err != nil {
			logger.Warn("failed to ack task message for missing handler", "error", err)
		}
		return nil
	}

	err := handler(handlerCtx, &task)
	if err != nil {
		logger.Error("task handler failed", "error", err)
		handlerErrMsg := err.Error()
		if updateErr := w.updateTaskStatus(handlerCtx, &task, models.TaskStatusFailed, nil, &handlerErrMsg); updateErr != nil {
			logger.Error("failed to update task status to failed", "error", updateErr)
		}
		taskmetrics.TasksTotal.WithLabelValues(task.Type, "failed").Inc()
		taskmetrics.TaskDuration.WithLabelValues(task.Type).Observe(time.Since(taskStart).Seconds())
		// Use exponential backoff when NAK-ing failed tasks.
		var nakDelay_ time.Duration
		if meta, metaErr := msg.Metadata(); metaErr == nil {
			nakDelay_ = nakDelay(meta.NumDelivered)
		} else {
			nakDelay_ = nakDelays[0]
		}
		if err := msg.NakWithDelay(nakDelay_); err != nil {
			logger.Warn("failed to nak task message after handler failure", "error", err)
		}
		return err
	}

	if err := w.updateTaskStatus(handlerCtx, &task, models.TaskStatusCompleted, task.Result, nil); err != nil {
		logger.Error("failed to update task status to completed", "error", err)
	}

	taskmetrics.TasksTotal.WithLabelValues(task.Type, "completed").Inc()
	taskmetrics.TaskDuration.WithLabelValues(task.Type).Observe(time.Since(taskStart).Seconds())

	logger.Info("task completed successfully")
	if err := msg.Ack(); err != nil {
		logger.Warn("failed to ack task message after completion", "error", err)
	}
	return nil
}

// PublishTask publishes a new task to the stream.
func (w *Worker) PublishTask(ctx context.Context, task *models.Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshaling task: %w", err)
	}

	subject := fmt.Sprintf("tasks.%s", task.Type)
	_, err = w.js.Publish(subject, data)
	if err != nil {
		return fmt.Errorf("publishing task: %w", err)
	}

	w.logger.Info("task published",
		"task_id", task.ID,
		"task_type", task.Type,
	)
	return nil
}

// updateTaskStatus updates the task status in the database.
// errMsg must be nil when there is no error message so that COALESCE($3, error_message)
// correctly preserves the existing column value instead of overwriting it with an
// empty string. Pass a non-nil pointer only when there is an actual error to record.
func (w *Worker) updateTaskStatus(ctx context.Context, task *models.Task, status models.TaskStatus, result json.RawMessage, errMsg *string) error {
	now := time.Now().UTC()

	query := `
		UPDATE tasks
		SET status = $1,
		    result = COALESCE($2, result),
		    error_message = COALESCE($3, error_message),
		    started_at = COALESCE($4, started_at),
		    completed_at = COALESCE($5, completed_at),
		    updated_at = $6
		WHERE id = $7
	`

	var startedAt, completedAt *time.Time
	if status == models.TaskStatusRunning {
		startedAt = &now
	} else if status == models.TaskStatusCompleted || status == models.TaskStatusFailed {
		completedAt = &now
	}

	_, err := w.dbPool.Exec(ctx, query,
		status,
		result,
		errMsg,
		startedAt,
		completedAt,
		now,
		task.ID,
	)
	if err != nil {
		return fmt.Errorf("updating task status: %w", err)
	}

	task.Status = status
	return nil
}

func (w *Worker) recoverStuckTasks(ctx context.Context, stuckThreshold time.Duration) {
	stuckTasks, err := w.taskRepo.FindStuckTasks(ctx, stuckThreshold)
	if err != nil {
		w.logger.Error("failed to find stuck tasks", "error", err)
		return
	}
	if len(stuckTasks) == 0 {
		return
	}

	for _, task := range stuckTasks {
		logger := w.logger.With("task_id", task.ID, "task_type", task.Type, "retry_count", task.RetryCount)
		if task.RetryCount >= maxTaskRetries {
			if setFailedErr := w.taskRepo.SetFailed(ctx, task.ID, stuckTaskRecoveredMessage); setFailedErr != nil {
				logger.Error("failed to mark stuck task as failed", "error", setFailedErr)
				continue
			}
			logger.Warn("stuck task marked failed after max retries")
			continue
		}
		if resetErr := w.taskRepo.ResetTask(ctx, task.ID); resetErr != nil {
			logger.Error("failed to reset stuck task", "error", resetErr)
			continue
		}
		logger.Warn("stuck task reset to pending for retry")
	}
}

// CreateTaskRecord creates a new task record in the database.
func (w *Worker) CreateTaskRecord(ctx context.Context, task *models.Task) error {
	query := `
		INSERT INTO tasks (id, type, status, payload, idempotency_key, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := w.dbPool.Exec(ctx, query,
		task.ID,
		task.Type,
		task.Status,
		task.Payload,
		task.IdempotencyKey,
		task.CreatedBy,
		task.CreatedAt,
		task.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("creating task record: %w", err)
	}

	return nil
}

// GetTask retrieves a task by ID from the database.
func (w *Worker) GetTask(ctx context.Context, taskID string) (*models.Task, error) {
	query := `
		SELECT id, type, status, payload, result, error_message, progress, 
		       idempotency_key, created_by, created_at, started_at, completed_at
		FROM tasks
		WHERE id = $1
	`

	var task models.Task
	err := w.dbPool.QueryRow(ctx, query, taskID).Scan(
		&task.ID,
		&task.Type,
		&task.Status,
		&task.Payload,
		&task.Result,
		&task.ErrorMessage,
		&task.Progress,
		&task.IdempotencyKey,
		&task.CreatedBy,
		&task.CreatedAt,
		&task.StartedAt,
		&task.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("getting task: %w", err)
	}

	return &task, nil
}

// UpdateTaskProgress updates the task progress in the database.
func (w *Worker) UpdateTaskProgress(ctx context.Context, taskID string, progress int) error {
	if progress < 0 {
		progress = 0
	} else if progress > 100 {
		progress = 100
	}

	query := `UPDATE tasks SET progress = $1, updated_at = $2 WHERE id = $3`
	_, err := w.dbPool.Exec(ctx, query, progress, time.Now().UTC(), taskID)
	if err != nil {
		return fmt.Errorf("updating task progress: %w", err)
	}

	return nil
}
