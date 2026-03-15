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

	taskmetrics "github.com/AbuGosok/VirtueStack/internal/controller/metrics"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

// JetStream constants.
const (
	StreamName    = "TASKS"
	StreamSubject = "tasks.>"
	ConsumerName  = "task-worker"
	AckWait       = 5 * time.Minute
	MaxDeliver    = 3
)

// TaskHandler processes a task of a specific type.
type TaskHandler func(ctx context.Context, task *models.Task) error

// Worker processes tasks from NATS JetStream.
type Worker struct {
	js       nats.JetStreamContext
	dbPool   *pgxpool.Pool
	handlers map[string]TaskHandler
	logger   *slog.Logger
	mu       sync.RWMutex
	cancel   context.CancelFunc
	wg       sync.WaitGroup
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
		handlers: make(map[string]TaskHandler),
		logger:   logger.With("component", "task-worker"),
	}, nil
}

// RegisterHandler registers a handler for a task type.
func (w *Worker) RegisterHandler(taskType string, handler TaskHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers[taskType] = handler
	w.logger.Debug("registered task handler", "task_type", taskType)
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
		w.handleMessage(ctx),
		nats.Durable(ConsumerName),
		nats.DeliverAll(),
		nats.ManualAck(),
	)
	if err != nil {
		return fmt.Errorf("subscribing to tasks: %w", err)
	}

	w.logger.Info("task worker started",
		"stream", StreamName,
		"consumer", ConsumerName,
	)

	// NATS QueueSubscribe handles concurrency via its deliver policy and
	// consumer AckWait/MaxDeliver settings; no additional goroutine pool needed.
	// The numWorkers parameter is accepted for future use if a bounded pool is introduced.
	_ = numWorkers

	go func() {
		<-ctx.Done()
		w.logger.Info("stopping task worker")
		sub.Unsubscribe()
	}()

	return nil
}

// Stop gracefully stops the worker.
func (w *Worker) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
	w.logger.Info("task worker stopped")
}

// handleMessage returns a message handler for NATS messages.
func (w *Worker) handleMessage(ctx context.Context) func(msg *nats.Msg) {
	return func(msg *nats.Msg) {
		w.wg.Add(1)
		defer w.wg.Done()

		var task models.Task
		if err := json.Unmarshal(msg.Data, &task); err != nil {
			w.logger.Error("failed to unmarshal task",
				"error", err,
				"subject", msg.Subject,
			)
			msg.Ack()
			return
		}

		logger := w.logger.With(
			"task_id", task.ID,
			"task_type", task.Type,
		)

		logger.Info("processing task")

		taskStart := time.Now()

		if err := w.updateTaskStatus(ctx, &task, models.TaskStatusRunning, nil, ""); err != nil {
			logger.Error("failed to update task status to running", "error", err)
			msg.Nak()
			return
		}

		w.mu.RLock()
		handler, ok := w.handlers[task.Type]
		w.mu.RUnlock()

		if !ok {
			errMsg := fmt.Sprintf("no handler registered for task type: %s", task.Type)
			logger.Error(errMsg)
			w.updateTaskStatus(ctx, &task, models.TaskStatusFailed, nil, errMsg)
			taskmetrics.TasksTotal.WithLabelValues(task.Type, "failed").Inc()
			taskmetrics.TaskDuration.WithLabelValues(task.Type).Observe(time.Since(taskStart).Seconds())
			msg.Ack()
			return
		}

		err := handler(ctx, &task)
		if err != nil {
			logger.Error("task handler failed", "error", err)
			w.updateTaskStatus(ctx, &task, models.TaskStatusFailed, nil, err.Error())
			taskmetrics.TasksTotal.WithLabelValues(task.Type, "failed").Inc()
			taskmetrics.TaskDuration.WithLabelValues(task.Type).Observe(time.Since(taskStart).Seconds())
			msg.Nak()
			return
		}

		if err := w.updateTaskStatus(ctx, &task, models.TaskStatusCompleted, task.Result, ""); err != nil {
			logger.Error("failed to update task status to completed", "error", err)
		}

		taskmetrics.TasksTotal.WithLabelValues(task.Type, "completed").Inc()
		taskmetrics.TaskDuration.WithLabelValues(task.Type).Observe(time.Since(taskStart).Seconds())

		logger.Info("task completed successfully")
		msg.Ack()
	}
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
func (w *Worker) updateTaskStatus(ctx context.Context, task *models.Task, status models.TaskStatus, result json.RawMessage, errMsg string) error {
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
