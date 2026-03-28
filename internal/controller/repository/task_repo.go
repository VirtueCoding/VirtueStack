// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// TaskRepository provides database operations for async tasks.
type TaskRepository struct {
	db DB
}

// NewTaskRepository creates a new TaskRepository with the given database connection.
func NewTaskRepository(db DB) *TaskRepository {
	return &TaskRepository{db: db}
}

// TaskListFilter holds query parameters for filtering and paginating task list results.
type TaskListFilter struct {
	models.PaginationParams
	Status    *models.TaskStatus
	Type      *string
	CreatedBy *string
	StartTime *time.Time
	EndTime   *time.Time
}

// scanTask scans a single task row into a models.Task struct.
func scanTask(row pgx.Row) (models.Task, error) {
	var t models.Task
	var result, errorMessage []byte
	var idempotencyKey, createdBy *string
	err := row.Scan(
		&t.ID, &t.Type, &t.Status, &t.Payload,
		&result, &errorMessage, &t.Progress, &t.RetryCount,
		&idempotencyKey, &createdBy,
		&t.CreatedAt, &t.StartedAt, &t.CompletedAt,
	)
	if result != nil {
		t.Result = result
	}
	if errorMessage != nil {
		t.ErrorMessage = string(errorMessage)
	}
	if idempotencyKey != nil {
		t.IdempotencyKey = *idempotencyKey
	}
	if createdBy != nil {
		t.CreatedBy = *createdBy
	}
	return t, err
}

const taskSelectCols = `
	id, type, status, payload,
	result, error_message, progress, retry_count,
	idempotency_key, created_by,
	created_at, started_at, completed_at`

// Create inserts a new task record into the database.
// The task's CreatedAt is populated by the database.
func (r *TaskRepository) Create(ctx context.Context, task *models.Task) error {
	const q = `
		INSERT INTO tasks (
			id, type, status, payload, progress, retry_count,
			idempotency_key, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING ` + taskSelectCols

	// Handle nullable UUID columns - empty string must be converted to NULL
	var idempotencyKey, createdBy any
	if task.IdempotencyKey != "" {
		idempotencyKey = task.IdempotencyKey
	}
	if task.CreatedBy != "" {
		createdBy = task.CreatedBy
	}

	row := r.db.QueryRow(ctx, q,
		task.ID, task.Type, task.Status, task.Payload, task.Progress, task.RetryCount,
		idempotencyKey, createdBy,
	)
	created, err := scanTask(row)
	if err != nil {
		return fmt.Errorf("creating task: %w", err)
	}
	*task = created
	return nil
}

// GetByID returns a task by its UUID. Returns ErrNotFound if no task matches.
func (r *TaskRepository) GetByID(ctx context.Context, id string) (*models.Task, error) {
	const q = `SELECT ` + taskSelectCols + ` FROM tasks WHERE id = $1`
	task, err := ScanRow(ctx, r.db, q, []any{id}, scanTask)
	if err != nil {
		return nil, fmt.Errorf("getting task %s: %w", id, err)
	}
	return &task, nil
}

// GetByIDempotencyKey returns a task by its idempotency key.
// Returns ErrNotFound if no task matches. Used for deduplication.
func (r *TaskRepository) GetByIDempotencyKey(ctx context.Context, key string) (*models.Task, error) {
	const q = `SELECT ` + taskSelectCols + ` FROM tasks WHERE idempotency_key = $1`
	task, err := ScanRow(ctx, r.db, q, []any{key}, scanTask)
	if err != nil {
		return nil, fmt.Errorf("getting task by idempotency key %s: %w", key, err)
	}
	return &task, nil
}

// List returns a paginated list of tasks with optional filters and total count.
func (r *TaskRepository) List(ctx context.Context, filter TaskListFilter) ([]models.Task, int, error) {
	where := []string{"1=1"}
	args := []any{}
	idx := 1

	if filter.Status != nil {
		where = append(where, fmt.Sprintf("status = $%d", idx))
		args = append(args, *filter.Status)
		idx++
	}
	if filter.Type != nil {
		where = append(where, fmt.Sprintf("type = $%d", idx))
		args = append(args, *filter.Type)
		idx++
	}
	if filter.CreatedBy != nil {
		where = append(where, fmt.Sprintf("created_by = $%d", idx))
		args = append(args, *filter.CreatedBy)
		idx++
	}
	if filter.StartTime != nil {
		where = append(where, fmt.Sprintf("created_at >= $%d", idx))
		args = append(args, *filter.StartTime)
		idx++
	}
	if filter.EndTime != nil {
		where = append(where, fmt.Sprintf("created_at <= $%d", idx))
		args = append(args, *filter.EndTime)
		idx++
	}

	clause := strings.Join(where, " AND ")
	countQ := "SELECT COUNT(*) FROM tasks WHERE " + clause
	total, err := CountRows(ctx, r.db, countQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("counting tasks: %w", err)
	}

	limit := filter.Limit()
	offset := filter.Offset()
	listQ := fmt.Sprintf(
		"SELECT %s FROM tasks WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		taskSelectCols, clause, idx, idx+1,
	)
	args = append(args, limit, offset)

	taskList, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (models.Task, error) {
		return scanTask(rows)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing tasks: %w", err)
	}
	return taskList, total, nil
}

// ListByStatus returns tasks matching the given status with a limit.
// Used for retry logic and task polling.
func (r *TaskRepository) ListByStatus(ctx context.Context, status models.TaskStatus, limit int) ([]models.Task, error) {
	const q = `SELECT ` + taskSelectCols + ` FROM tasks WHERE status = $1 ORDER BY created_at ASC LIMIT $2`
	taskList, err := ScanRows(ctx, r.db, q, []any{status, limit}, func(rows pgx.Rows) (models.Task, error) {
		return scanTask(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing tasks by status %s: %w", status, err)
	}
	return taskList, nil
}

// maxPendingBatch is the maximum number of pending tasks returned in a single ListPending call.
// This prevents unbounded result sets when the task queue grows large.
const maxPendingBatch = 500

// ListPending returns up to maxPendingBatch tasks with status 'pending', oldest first.
// Convenience method for ListByStatus with pending status.
func (r *TaskRepository) ListPending(ctx context.Context) ([]models.Task, error) {
	const q = `SELECT ` + taskSelectCols + ` FROM tasks WHERE status = 'pending' ORDER BY created_at ASC LIMIT $1`
	taskList, err := ScanRows(ctx, r.db, q, []any{maxPendingBatch}, func(rows pgx.Rows) (models.Task, error) {
		return scanTask(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing pending tasks: %w", err)
	}
	return taskList, nil
}

// UpdateStatus updates the status field of a task.
func (r *TaskRepository) UpdateStatus(ctx context.Context, id string, status models.TaskStatus) error {
	const q = `UPDATE tasks SET status = $1 WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, status, id)
	if err != nil {
		return fmt.Errorf("updating task %s status: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating task %s status: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// UpdateProgress updates the progress (0-100) and optional message for a task.
// The progress_message column (added by migration 000060) stores a human-readable
// description of the current progress stage (e.g., "Copying disk 3 of 5").
func (r *TaskRepository) UpdateProgress(ctx context.Context, id string, progress int, message string) error {
	const q = `UPDATE tasks SET progress = $1, progress_message = NULLIF($2, '') WHERE id = $3`
	tag, err := r.db.Exec(ctx, q, progress, message, id)
	if err != nil {
		return fmt.Errorf("updating task %s progress: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating task %s progress: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// SetResult stores the result JSON for a completed task.
func (r *TaskRepository) SetResult(ctx context.Context, id string, result []byte) error {
	const q = `UPDATE tasks SET result = $1 WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, result, id)
	if err != nil {
		return fmt.Errorf("setting task %s result: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("setting task %s result: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// SetError stores the error message for a failed task.
func (r *TaskRepository) SetError(ctx context.Context, id string, errorMessage string) error {
	const q = `UPDATE tasks SET error_message = $1 WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, errorMessage, id)
	if err != nil {
		return fmt.Errorf("setting task %s error: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("setting task %s error: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// SetStarted marks a task as running and sets started_at to NOW().
func (r *TaskRepository) SetStarted(ctx context.Context, id string) error {
	const q = `UPDATE tasks SET status = 'running', started_at = NOW() WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("setting task %s started: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("setting task %s started: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// SetCompleted marks a task as completed, sets completed_at to NOW(), and stores the result.
func (r *TaskRepository) SetCompleted(ctx context.Context, id string, result []byte) error {
	const q = `UPDATE tasks SET status = 'completed', completed_at = NOW(), result = $1, progress = 100 WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, result, id)
	if err != nil {
		return fmt.Errorf("setting task %s completed: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("setting task %s completed: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// SetFailed marks a task as failed, sets completed_at to NOW(), and stores the error message.
func (r *TaskRepository) SetFailed(ctx context.Context, id string, errorMessage string) error {
	const q = `UPDATE tasks SET status = 'failed', completed_at = NOW(), error_message = $1, retry_count = retry_count + 1 WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, errorMessage, id)
	if err != nil {
		return fmt.Errorf("setting task %s failed: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("setting task %s failed: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// FindStuckTasks finds running tasks older than the provided threshold.
func (r *TaskRepository) FindStuckTasks(ctx context.Context, threshold time.Duration) ([]*models.Task, error) {
	const q = `SELECT ` + taskSelectCols + ` FROM tasks WHERE status = 'running' AND started_at IS NOT NULL AND started_at < NOW() - ($1 * INTERVAL '1 second') ORDER BY started_at ASC`
	seconds := int64(threshold.Seconds())
	tasks, err := ScanRows(ctx, r.db, q, []any{seconds}, func(rows pgx.Rows) (models.Task, error) {
		return scanTask(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("finding stuck tasks: %w", err)
	}
	result := make([]*models.Task, 0, len(tasks))
	for i := range tasks {
		taskCopy := tasks[i]
		result = append(result, &taskCopy)
	}
	return result, nil
}

// ResetTask resets a task to pending state for retry.
func (r *TaskRepository) ResetTask(ctx context.Context, taskID string) error {
	const q = `UPDATE tasks SET status = 'pending', error_message = NULL, started_at = NULL, completed_at = NULL, updated_at = NOW() WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, taskID)
	if err != nil {
		return fmt.Errorf("resetting task %s: %w", taskID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("resetting task %s: %w", taskID, ErrNoRowsAffected)
	}
	return nil
}

// Delete permanently removes a task record from the database.
// Used for cleanup of old completed/failed tasks.
func (r *TaskRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM tasks WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting task %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("deleting task %s: %w", id, ErrNoRowsAffected)
	}
	return nil
}

// CountByStorageBackend returns the count of in-flight tasks (pending or running) that
// reference the given storage backend. Tasks may reference the storage backend through:
// - VM migration tasks (payload contains source_node_id or target_node_id from nodes using this backend)
// - Backup tasks for VMs using this backend
// - Snapshot tasks for VMs using this backend
func (r *TaskRepository) CountByStorageBackend(ctx context.Context, storageBackendID string) (int, error) {
	const q = `
		SELECT COUNT(*)
		FROM tasks t
		WHERE t.status IN ('pending', 'running')
		  AND (
			-- Tasks referencing VMs that use this storage backend
			t.payload::jsonb ? 'vm_id'
			AND EXISTS (
				SELECT 1 FROM vms v WHERE v.id = (t.payload::jsonb->>'vm_id')::uuid AND v.storage_backend_id = $1
			)
			OR
			-- Tasks referencing source node that has this storage backend
			t.payload::jsonb ? 'source_node_id'
			AND EXISTS (
				SELECT 1 FROM node_storage ns WHERE ns.node_id = (t.payload::jsonb->>'source_node_id')::uuid AND ns.storage_backend_id = $1
			)
			OR
			-- Tasks referencing target node that has this storage backend
			t.payload::jsonb ? 'target_node_id'
			AND EXISTS (
				SELECT 1 FROM node_storage ns WHERE ns.node_id = (t.payload::jsonb->>'target_node_id')::uuid AND ns.storage_backend_id = $1
			)
		  )
	`
	var count int
	err := r.db.QueryRow(ctx, q, storageBackendID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting tasks by storage backend %s: %w", storageBackendID, err)
	}
	return count, nil
}
