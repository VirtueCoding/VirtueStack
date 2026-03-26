package customer

import (
	"encoding/json"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// TaskStatusResponse represents the status of an async task.
type TaskStatusResponse struct {
	ID              string            `json:"id"`
	Type            string            `json:"type"`
	Status          models.TaskStatus `json:"status"`
	Progress        int               `json:"progress"`
	ProgressMessage string            `json:"progress_message,omitempty"`
	ErrorMessage    string            `json:"error_message,omitempty"`
	Result          any               `json:"result,omitempty"`
	CreatedAt       string            `json:"created_at"`
	StartedAt       string            `json:"started_at,omitempty"`
	CompletedAt     string            `json:"completed_at,omitempty"`
}

// GetTaskStatus handles GET /tasks/:id - retrieves the status of an async task.
func (h *CustomerHandler) GetTaskStatus(c *gin.Context) {
	taskID := c.Param("id")

	if _, err := uuid.Parse(taskID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest,
			"INVALID_TASK_ID", "Task ID must be a valid UUID")
		return
	}

	task, err := h.taskRepo.GetByID(c.Request.Context(), taskID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound,
				"TASK_NOT_FOUND", "Task not found")
			return
		}
		h.logger.Error("failed to get task",
			"task_id", taskID, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"TASK_LOOKUP_FAILED", "Internal server error")
		return
	}

	resp := buildCustomerTaskResponse(task)
	c.JSON(http.StatusOK, models.Response{Data: resp})
}

// buildCustomerTaskResponse builds a TaskStatusResponse from a Task model.
func buildCustomerTaskResponse(task *models.Task) TaskStatusResponse {
	resp := TaskStatusResponse{
		ID:       task.ID,
		Type:     task.Type,
		Status:   task.Status,
		Progress: task.Progress,
		CreatedAt: task.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if task.StartedAt != nil {
		resp.StartedAt = task.StartedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if task.CompletedAt != nil {
		resp.CompletedAt = task.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	if task.Status == models.TaskStatusCompleted && task.Result != nil {
		var result any
		if err := json.Unmarshal(task.Result, &result); err == nil {
			resp.Result = result
		}
	}

	if task.Status == models.TaskStatusFailed && task.ErrorMessage != "" {
		resp.ErrorMessage = task.ErrorMessage
	}

	return resp
}
