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
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

// JetStream constants.
const (
	StreamName          = "TASKS"
	StreamSubject       = "tasks.>"
	EventStreamName     = "EVENTS"
	EventStreamSubject  = "virtuestack.events.>"
	ConsumerName        = "task-worker"
	AckWait             = 5 * time.Minute
	AckProgressInterval = time.Minute
	MaxDeliver          = 3
	maxTaskRetries      = 3
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

type taskMessage interface {
	payload() []byte
	subjectName() string
	ack() error
	nakWithDelay(delay time.Duration) error
	inProgress() error
	metadata() (*nats.MsgMetadata, error)
}

type taskQueue interface {
	AddStream(cfg *nats.StreamConfig, opts ...nats.JSOpt) (*nats.StreamInfo, error)
	AddConsumer(stream string, cfg *nats.ConsumerConfig, opts ...nats.JSOpt) (*nats.ConsumerInfo, error)
	QueueSubscribe(subj, queue string, cb nats.MsgHandler, opts ...nats.SubOpt) (*nats.Subscription, error)
	Publish(subj string, data []byte, opts ...nats.PubOpt) (*nats.PubAck, error)
}

type natsTaskMessage struct {
	msg *nats.Msg
}

func (m natsTaskMessage) payload() []byte {
	return m.msg.Data
}

func (m natsTaskMessage) subjectName() string {
	return m.msg.Subject
}

func (m natsTaskMessage) ack() error {
	return m.msg.Ack()
}

func (m natsTaskMessage) nakWithDelay(delay time.Duration) error {
	return m.msg.NakWithDelay(delay)
}

func (m natsTaskMessage) inProgress() error {
	return m.msg.InProgress()
}

func (m natsTaskMessage) metadata() (*nats.MsgMetadata, error) {
	return m.msg.Metadata()
}

type workerTaskRepository interface {
	FindStuckTasks(ctx context.Context, threshold time.Duration) ([]*models.Task, error)
	RetryTask(ctx context.Context, taskID string) error
	SetFailed(ctx context.Context, id string, errorMessage string) error
}

// Worker processes tasks from NATS JetStream.
type Worker struct {
	js                  taskQueue
	db                  repository.DB
	taskRepo            workerTaskRepository
	handlers            map[string]TaskHandler
	logger              *slog.Logger
	ackProgressInterval time.Duration
	handlerTimeout      time.Duration
	mu                  sync.RWMutex
	cancel              context.CancelFunc
	eg                  *errgroup.Group
	egCtx               context.Context
}

// NewWorker creates a new task worker.
func NewWorker(js nats.JetStreamContext, dbPool *pgxpool.Pool, logger *slog.Logger) (*Worker, error) {
	streamConfig := &nats.StreamConfig{
		Name:      StreamName,
		Subjects:  []string{StreamSubject},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		MaxAge:    7 * 24 * time.Hour,
		MaxMsgs:   1_000_000,
		Replicas:  1,
	}

	_, err := js.AddStream(streamConfig)
	if err != nil {
		if !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
			return nil, fmt.Errorf("creating JetStream stream: %w", err)
		}
	}

	_, err = js.AddStream(&nats.StreamConfig{
		Name:      EventStreamName,
		Subjects:  []string{EventStreamSubject},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		MaxAge:    7 * 24 * time.Hour,
		MaxMsgs:   1_000_000,
		Replicas:  1,
	})
	if err != nil {
		if !errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
			return nil, fmt.Errorf("creating event stream: %w", err)
		}
	}

	return &Worker{
		js:                  js,
		db:                  dbPool,
		taskRepo:            repository.NewTaskRepository(dbPool),
		handlers:            make(map[string]TaskHandler),
		logger:              logger.With("component", "task-worker"),
		ackProgressInterval: AckProgressInterval,
		handlerTimeout:      AckWait,
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
	workerCtx, cancel := context.WithCancel(ctx)
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

	eg, egCtx := errgroup.WithContext(workerCtx)
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
		if err := w.eg.Wait(); err != nil {
			w.logger.Warn("task worker wait returned error during shutdown", "error", err)
		}
	}
	w.logger.Info("task worker stopped")
}

// handleMessage returns a message handler for NATS messages.
func (w *Worker) handleMessage() func(msg *nats.Msg) {
	return func(msg *nats.Msg) {
		processCtx := w.egCtx
		if processCtx == nil {
			processCtx = context.Background()
		}

		run := func() error {
			if err := w.processMessage(processCtx, natsTaskMessage{msg: msg}); err != nil {
				w.logger.Warn("task processing returned error",
					"subject", msg.Subject,
					"error", err,
				)
			}
			return nil
		}

		if w.eg != nil {
			w.eg.Go(run)
			return
		}

		if err := run(); err != nil {
			w.logger.Warn("task processing returned error outside worker group",
				"subject", msg.Subject,
				"error", err,
			)
		}
	}
}

func (w *Worker) processMessage(ctx context.Context, msg taskMessage) error {
	var task models.Task
	if err := json.Unmarshal(msg.payload(), &task); err != nil {
		w.logger.Error("failed to unmarshal task",
			"error", err,
			"subject", msg.subjectName(),
		)
		if err := msg.ack(); err != nil {
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

	handlerTimeout := w.handlerTimeout
	if handlerTimeout <= 0 {
		handlerTimeout = AckWait
	}

	handlerCtx, handlerCancel := context.WithTimeout(ctx, handlerTimeout)
	defer handlerCancel()

	runningStatusCtx, runningStatusCancel := context.WithTimeout(ctx, handlerTimeout)
	defer runningStatusCancel()

	if err := w.updateTaskStatus(runningStatusCtx, &task, models.TaskStatusRunning, nil, nil); err != nil {
		if errors.Is(err, sharederrors.ErrNoRowsAffected) {
			logger.Warn("dropping orphaned task message with no backing task row")
			if ackErr := msg.ack(); ackErr != nil {
				logger.Warn("failed to ack orphaned task message", "error", ackErr)
			}
			return nil
		}
		logger.Error("failed to update task status to running", "error", err)
		// Determine backoff delay from delivery count.
		var backoffDelay time.Duration
		if meta, metaErr := msg.metadata(); metaErr == nil {
			backoffDelay = nakDelay(meta.NumDelivered)
		} else {
			backoffDelay = nakDelays[0]
		}
		if nakErr := msg.nakWithDelay(backoffDelay); nakErr != nil {
			logger.Warn("failed to nak task message", "error", nakErr)
		}
		return err
	}

	w.mu.RLock()
	handler, ok := w.handlers[task.Type]
	w.mu.RUnlock()

	if !ok {
		errMsg := fmt.Sprintf("no handler registered for task type: %s", task.Type)
		logger.Error(errMsg)
		if updateErr := w.updateTaskStatus(ctx, &task, models.TaskStatusFailed, nil, &errMsg); updateErr != nil {
			logger.Warn("failed to persist missing-handler task status", "error", updateErr)
			return updateErr
		}
		taskmetrics.TasksTotal.WithLabelValues(task.Type, "failed").Inc()
		taskmetrics.TaskDuration.WithLabelValues(task.Type).Observe(time.Since(taskStart).Seconds())
		if err := msg.ack(); err != nil {
			logger.Warn("failed to ack task message for missing handler", "error", err)
		}
		return nil
	}

	stopAckProgress := w.startAckProgress(handlerCtx, msg, task.ID, logger)
	defer stopAckProgress()

	err := handler(handlerCtx, &task)
	if err != nil {
		logger.Error("task handler failed", "error", err)
		taskmetrics.TasksTotal.WithLabelValues(task.Type, "failed").Inc()
		taskmetrics.TaskDuration.WithLabelValues(task.Type).Observe(time.Since(taskStart).Seconds())

		retryable, retryDelay, retryDecisionErr := taskRetryDecision(msg)
		if retryDecisionErr != nil {
			logger.Warn("failed to read task delivery metadata; failing closed", "error", retryDecisionErr)
		}
		if retryable {
			if updateErr := w.resetTaskForRetry(ctx, &task); updateErr != nil {
				logger.Error("failed to reset task state for retry", "error", updateErr)
				return err
			}
			if nakErr := msg.nakWithDelay(retryDelay); nakErr != nil {
				logger.Warn("failed to nak task message after handler failure", "error", nakErr)
			}
			return err
		}

		handlerErrMsg := err.Error()
		if updateErr := w.updateTaskStatus(ctx, &task, models.TaskStatusFailed, nil, &handlerErrMsg); updateErr != nil {
			logger.Error("failed to update task status to failed", "error", updateErr)
			return err
		}
		if ackErr := msg.ack(); ackErr != nil {
			logger.Warn("failed to ack task message after terminal handler failure", "error", ackErr)
		}
		return err
	}

	if err := w.updateTaskStatus(ctx, &task, models.TaskStatusCompleted, task.Result, nil); err != nil {
		logger.Error("failed to update task status to completed", "error", err)
		return err
	}

	taskmetrics.TasksTotal.WithLabelValues(task.Type, "completed").Inc()
	taskmetrics.TaskDuration.WithLabelValues(task.Type).Observe(time.Since(taskStart).Seconds())

	logger.Info("task completed successfully")
	if err := msg.ack(); err != nil {
		logger.Warn("failed to ack task message after completion", "error", err)
	}
	return nil
}

func taskRetryDecision(msg taskMessage) (bool, time.Duration, error) {
	meta, err := msg.metadata()
	if err != nil {
		return false, 0, fmt.Errorf("reading task delivery metadata: %w", err)
	}
	if meta.NumDelivered >= MaxDeliver {
		return false, 0, nil
	}
	return true, nakDelay(meta.NumDelivered), nil
}

func (w *Worker) startAckProgress(ctx context.Context, msg taskMessage, taskID string, logger *slog.Logger) func() {
	interval := w.ackProgressInterval
	if interval <= 0 {
		interval = AckProgressInterval
	}

	progressCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	go func() {
		defer close(done)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-progressCtx.Done():
				return
			case <-ticker.C:
				if err := msg.inProgress(); err != nil {
					logger.Warn("failed to extend task ack deadline", "error", err)
				}
				if err := w.touchRunningTask(progressCtx, taskID); err != nil {
					logger.Warn("failed to touch running task heartbeat", "error", err)
				}
			}
		}
	}()

	return func() {
		cancel()
		<-done
	}
}

func (w *Worker) touchRunningTask(ctx context.Context, taskID string) error {
	const query = `UPDATE tasks SET updated_at = NOW() WHERE id = $1 AND status = 'running'`
	tag, err := w.db.Exec(ctx, query, taskID)
	if err != nil {
		return fmt.Errorf("touching running task heartbeat: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return sharederrors.ErrNoRowsAffected
	}
	return nil
}

func (w *Worker) resetTaskForRetry(ctx context.Context, task *models.Task) error {
	now := time.Now().UTC()

	const query = `
		UPDATE tasks
		SET status = $1,
		    result = NULL,
		    error_message = NULL,
		    progress = 0,
		    progress_message = NULL,
		    started_at = NULL,
		    completed_at = NULL,
		    updated_at = $2,
		    retry_count = retry_count + 1
		WHERE id = $3
	`

	tag, err := w.db.Exec(ctx, query, models.TaskStatusPending, now, task.ID)
	if err != nil {
		return fmt.Errorf("resetting task for retry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return sharederrors.ErrNoRowsAffected
	}

	task.Status = models.TaskStatusPending
	task.Result = nil
	task.ErrorMessage = ""
	task.Progress = 0
	task.ProgressMessage = ""
	task.StartedAt = nil
	task.CompletedAt = nil
	task.UpdatedAt = now
	task.RetryCount++

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

func (w *Worker) updateTaskStatus(ctx context.Context, task *models.Task, status models.TaskStatus, result json.RawMessage, errMsg *string) error {
	now := time.Now().UTC()

	query := `
		UPDATE tasks
		SET status = $1,
		    result = COALESCE($2, result),
		    error_message = $3,
		    started_at = COALESCE($4, started_at),
		    completed_at = COALESCE($5, completed_at),
		    updated_at = $6
		WHERE id = $7
	`
	if status == models.TaskStatusFailed {
		query = `
		UPDATE tasks
		SET status = $1,
		    result = COALESCE($2, result),
		    error_message = $3,
		    started_at = COALESCE($4, started_at),
		    completed_at = COALESCE($5, completed_at),
		    updated_at = $6,
		    retry_count = retry_count + 1
		WHERE id = $7
	`
	}

	var startedAt, completedAt *time.Time
	var errorMessage any = errMsg
	if status == models.TaskStatusRunning {
		startedAt = &now
		errorMessage = nil
	} else if status == models.TaskStatusCompleted || status == models.TaskStatusFailed {
		completedAt = &now
		if status == models.TaskStatusCompleted {
			errorMessage = nil
		}
	}

	tag, err := w.db.Exec(ctx, query,
		status,
		result,
		errorMessage,
		startedAt,
		completedAt,
		now,
		task.ID,
	)
	if err != nil {
		return fmt.Errorf("updating task status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return sharederrors.ErrNoRowsAffected
	}

	task.Status = status
	if status == models.TaskStatusFailed {
		task.RetryCount++
	}
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
		if retryErr := w.taskRepo.RetryTask(ctx, task.ID); retryErr != nil {
			logger.Error("failed to retry stuck task", "error", retryErr)
			continue
		}
		task.Status = models.TaskStatusPending
		if publishErr := w.PublishTask(ctx, task); publishErr != nil {
			logger.Error("failed to republish recovered stuck task", "error", publishErr)
			if setFailedErr := w.taskRepo.SetFailed(ctx, task.ID, publishErr.Error()); setFailedErr != nil {
				logger.Error("failed to mark recovered task failed after republish error", "error", setFailedErr)
			}
			continue
		}
		logger.Warn("stuck task reset to pending and retry_count incremented")
	}
}

// CreateTaskRecord creates a new task record in the database.
func (w *Worker) CreateTaskRecord(ctx context.Context, task *models.Task) error {
	query := `
		INSERT INTO tasks (id, type, status, payload, idempotency_key, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := w.db.Exec(ctx, query,
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
		       progress_message, idempotency_key, created_by, created_at, started_at, completed_at, updated_at
		FROM tasks
		WHERE id = $1
	`

	var task models.Task
	var progressMessage *string
	err := w.db.QueryRow(ctx, query, taskID).Scan(
		&task.ID,
		&task.Type,
		&task.Status,
		&task.Payload,
		&task.Result,
		&task.ErrorMessage,
		&task.Progress,
		&progressMessage,
		&task.IdempotencyKey,
		&task.CreatedBy,
		&task.CreatedAt,
		&task.StartedAt,
		&task.CompletedAt,
		&task.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("getting task: %w", err)
	}
	if progressMessage != nil {
		task.ProgressMessage = *progressMessage
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
	_, err := w.db.Exec(ctx, query, progress, time.Now().UTC(), taskID)
	if err != nil {
		return fmt.Errorf("updating task progress: %w", err)
	}

	return nil
}
