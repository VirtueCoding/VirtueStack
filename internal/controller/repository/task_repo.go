// Package repository provides PostgreSQL database operations for VirtueStack Controller.
package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/tasks"
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
	Status    *tasks.TaskStatus
	Type      *string
	CreatedBy *string
	StartTime *time.Time
	EndTime   *time.Time
}

// scanTask scans a single task row into a tasks.Task struct.
func scanTask(row pgx.Row) (tasks.Task, error) {
	var t tasks.Task
	err := row.Scan(
		&t.ID, &t.Type, &t.Status, &t.Payload,
		&t.Result, &t.ErrorMessage, &t.Progress,
		&t.IdempotencyKey, &t.CreatedBy,
		&t.CreatedAt, &t.StartedAt, &t.CompletedAt,
	)
	return t, err
}

const taskSelectCols = `
	id, type, status, payload,
	result, error_message, progress,
	idempotency_key, created_by,
	created_at, started_at, completed_at`

// Create inserts a new task record into the database.
// The task's CreatedAt is populated by the database.
func (r *TaskRepository) Create(ctx context.Context, task *tasks.Task) error {
	const q = `
		INSERT INTO tasks (
			id, type, status, payload, progress,
			idempotency_key, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING ` + taskSelectCols

	row := r.db.QueryRow(ctx, q,
		task.ID, task.Type, task.Status, task.Payload, task.Progress,
		task.IdempotencyKey, task.CreatedBy,
	)
	created, err := scanTask(row)
	if err != nil {
		return fmt.Errorf("creating task: %w", err)
	}
	*task = created
	return nil
}

// GetByID returns a task by its UUID. Returns ErrNotFound if no task matches.
func (r *TaskRepository) GetByID(ctx context.Context, id string) (*tasks.Task, error) {
	const q = `SELECT ` + taskSelectCols + ` FROM tasks WHERE id = $1`
	task, err := ScanRow(ctx, r.db, q, []any{id}, scanTask)
	if err != nil {
		return nil, fmt.Errorf("getting task %s: %w", id, err)
	}
	return &task, nil
}

// GetByIDempotencyKey returns a task by its idempotency key.
// Returns ErrNotFound if no task matches. Used for deduplication.
func (r *TaskRepository) GetByIDempotencyKey(ctx context.Context, key string) (*tasks.Task, error) {
	const q = `SELECT ` + taskSelectCols + ` FROM tasks WHERE idempotency_key = $1`
	task, err := ScanRow(ctx, r.db, q, []any{key}, scanTask)
	if err != nil {
		return nil, fmt.Errorf("getting task by idempotency key %s: %w", key, err)
	}
	return &task, nil
}

// List returns a paginated list of tasks with optional filters and total count.
func (r *TaskRepository) List(ctx context.Context, filter TaskListFilter) ([]tasks.Task, int, error) {
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

	taskList, err := ScanRows(ctx, r.db, listQ, args, func(rows pgx.Rows) (tasks.Task, error) {
		return scanTask(rows)
	})
	if err != nil {
		return nil, 0, fmt.Errorf("listing tasks: %w", err)
	}
	return taskList, total, nil
}

// ListByStatus returns tasks matching the given status with a limit.
// Used for retry logic and task polling.
func (r *TaskRepository) ListByStatus(ctx context.Context, status tasks.TaskStatus, limit int) ([]tasks.Task, error) {
	const q = `SELECT ` + taskSelectCols + ` FROM tasks WHERE status = $1 ORDER BY created_at ASC LIMIT $2`
	taskList, err := ScanRows(ctx, r.db, q, []any{status, limit}, func(rows pgx.Rows) (tasks.Task, error) {
		return scanTask(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing tasks by status %s: %w", status, err)
	}
	return taskList, nil
}

// ListPending returns all tasks with status 'pending'.
// Convenience method for ListByStatus with pending status.
func (r *TaskRepository) ListPending(ctx context.Context) ([]tasks.Task, error) {
	const q = `SELECT ` + taskSelectCols + ` FROM tasks WHERE status = 'pending' ORDER BY created_at ASC`
	taskList, err := ScanRows(ctx, r.db, q, nil, func(rows pgx.Rows) (tasks.Task, error) {
		return scanTask(rows)
	})
	if err != nil {
		return nil, fmt.Errorf("listing pending tasks: %w", err)
	}
	return taskList, nil
}

// UpdateStatus updates the status field of a task.
func (r *TaskRepository) UpdateStatus(ctx context.Context, id string, status tasks.TaskStatus) error {
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
func (r *TaskRepository) UpdateProgress(ctx context.Context, id string, progress int, message string) error {
	const q = `UPDATE tasks SET progress = $1 WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, progress, id)
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
	const q = `UPDATE tasks SET status = 'failed', completed_at = NOW(), error_message = $1 WHERE id = $2`
	tag, err := r.db.Exec(ctx, q, errorMessage, id)
	if err != nil {
		return fmt.Errorf("setting task %s failed: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("setting task %s failed: %w", id, ErrNoRowsAffected)
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