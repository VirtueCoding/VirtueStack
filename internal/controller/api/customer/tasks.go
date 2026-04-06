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
// Enforces customer isolation by verifying the task's vm_id belongs to the
// requesting customer. Tasks without a vm_id in the payload are not accessible.
// @Tags Customer
// @Summary Get task status
// @Description Returns status for an async task owned by the authenticated customer.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "Task ID"
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/tasks/{id} [get]
func (h *CustomerHandler) GetTaskStatus(c *gin.Context) {
	taskID := c.Param("id")
	customerID := middleware.GetUserID(c)

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

	// Verify the task belongs to the requesting customer by checking
	// whether the vm_id in the payload is owned by this customer.
	if !h.verifyTaskOwnership(c, task, customerID) {
		return
	}

	resp := buildCustomerTaskResponse(task)
	c.JSON(http.StatusOK, models.Response{Data: resp})
}

// verifyTaskOwnership checks that a task's associated VM belongs to the
// requesting customer. Returns false (and writes the error response) if
// ownership cannot be confirmed.
func (h *CustomerHandler) verifyTaskOwnership(c *gin.Context, task *models.Task, customerID string) bool {
	vmID := extractVMIDFromPayload(task.Payload)
	if vmID == "" {
		middleware.RespondWithError(c, http.StatusNotFound,
			"TASK_NOT_FOUND", "Task not found")
		return false
	}

	vm, err := h.vmRepo.GetByID(c.Request.Context(), vmID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound,
				"TASK_NOT_FOUND", "Task not found")
			return false
		}
		h.logger.Error("failed to verify task ownership",
			"task_id", task.ID, "vm_id", vmID, "error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError,
			"TASK_LOOKUP_FAILED", "Internal server error")
		return false
	}
	if vm == nil || vm.CustomerID != customerID {
		middleware.RespondWithError(c, http.StatusNotFound,
			"TASK_NOT_FOUND", "Task not found")
		return false
	}
	return middleware.CheckVMScope(c, vmID)
}

// extractVMIDFromPayload attempts to extract the vm_id field from a task payload.
func extractVMIDFromPayload(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		VMID string `json:"vm_id"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	return p.VMID
}

// buildCustomerTaskResponse builds a TaskStatusResponse from a Task model.
func buildCustomerTaskResponse(task *models.Task) TaskStatusResponse {
	resp := TaskStatusResponse{
		ID:              task.ID,
		Type:            task.Type,
		Status:          task.Status,
		Progress:        task.Progress,
		ProgressMessage: task.ProgressMessage,
		CreatedAt:       task.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
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
