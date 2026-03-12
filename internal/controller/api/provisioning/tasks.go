package provisioning

import (
	"encoding/json"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetTask handles GET /tasks/:id - retrieves the status of an async task.
// This endpoint is used by WHMCS to poll the status of long-running operations
// like VM creation, deletion, or reinstallation.
func (h *ProvisioningHandler) GetTask(c *gin.Context) {
	taskID := c.Param("id")

	// Validate UUID format
	if _, err := uuid.Parse(taskID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_TASK_ID", "Task ID must be a valid UUID")
		return
	}

	// Get the task from repository
	task, err := h.taskRepo.GetByID(c.Request.Context(), taskID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "TASK_NOT_FOUND", "Task not found")
			return
		}
		h.logger.Error("failed to get task",
			"task_id", taskID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "TASK_LOOKUP_FAILED", err.Error())
		return
	}

	// Build the response
	resp := TaskStatusResponse{
		ID:        task.ID,
		Type:      task.Type,
		Status:    task.Status,
		Progress:  task.Progress,
		Message:   getTaskMessage(task),
		CreatedAt: task.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	// Include result if task is completed
	if task.Status == models.TaskStatusCompleted && task.Result != nil {
		var result any
		if err := json.Unmarshal(task.Result, &result); err == nil {
			resp.Result = result
		}
	}

	// Include error message if task failed
	if task.Status == models.TaskStatusFailed && task.ErrorMessage != "" {
		resp.Message = task.ErrorMessage
	}

	c.JSON(http.StatusOK, models.Response{
		Data: resp,
	})
}

// getTaskMessage returns a human-readable message for a task based on its type and status.
func getTaskMessage(task *models.Task) string {
	if task.ErrorMessage != "" {
		return task.ErrorMessage
	}

	switch task.Type {
	case models.TaskTypeVMCreate:
		return getVMCreateMessage(task)
	case models.TaskTypeVMDelete:
		return getVMDeleteMessage(task)
	case models.TaskTypeVMReinstall:
		return getVMReinstallMessage(task)
	case models.TaskTypeVMResize:
		return getVMResizeMessage(task)
	case models.TaskTypeBackupCreate:
		return getBackupCreateMessage(task)
	case models.TaskTypeBackupRestore:
		return getBackupRestoreMessage(task)
	case models.TaskTypeSnapshotCreate:
		return getSnapshotCreateMessage(task)
	case models.TaskTypeSnapshotRevert:
		return getSnapshotRevertMessage(task)
	case models.TaskTypeSnapshotDelete:
		return getSnapshotDeleteMessage(task)
	default:
		return getGenericTaskMessage(task)
	}
}

func getVMCreateMessage(task *models.Task) string {
	switch task.Status {
	case models.TaskStatusPending:
		return "VM creation queued"
	case models.TaskStatusRunning:
		return getProgressMessage(task.Progress, []string{
			"Validating parameters...",
			"Allocating IP addresses...",
			"Cloning disk image...",
			"Resizing disk...",
			"Generating cloud-init...",
			"Uploading cloud-init ISO...",
			"Creating VM definition...",
			"Starting VM...",
			"Verifying VM is running...",
		})
	case models.TaskStatusCompleted:
		return "VM created successfully"
	case models.TaskStatusFailed:
		return task.ErrorMessage
	default:
		return "Unknown status"
	}
}

func getVMDeleteMessage(task *models.Task) string {
	switch task.Status {
	case models.TaskStatusPending:
		return "VM deletion queued"
	case models.TaskStatusRunning:
		return getProgressMessage(task.Progress, []string{
			"Stopping VM...",
			"Deleting disk image...",
			"Releasing IP addresses...",
			"Removing VM definition...",
		})
	case models.TaskStatusCompleted:
		return "VM deleted successfully"
	case models.TaskStatusFailed:
		return task.ErrorMessage
	default:
		return "Unknown status"
	}
}

func getVMReinstallMessage(task *models.Task) string {
	switch task.Status {
	case models.TaskStatusPending:
		return "VM reinstallation queued"
	case models.TaskStatusRunning:
		return getProgressMessage(task.Progress, []string{
			"Stopping VM...",
			"Deleting disk image...",
			"Cloning fresh disk image...",
			"Regenerating cloud-init...",
			"Starting VM...",
		})
	case models.TaskStatusCompleted:
		return "VM reinstalled successfully"
	case models.TaskStatusFailed:
		return task.ErrorMessage
	default:
		return "Unknown status"
	}
}

func getVMResizeMessage(task *models.Task) string {
	switch task.Status {
	case models.TaskStatusPending:
		return "VM resize queued"
	case models.TaskStatusRunning:
		return "Resizing VM resources..."
	case models.TaskStatusCompleted:
		return "VM resized successfully"
	case models.TaskStatusFailed:
		return task.ErrorMessage
	default:
		return "Unknown status"
	}
}

func getBackupCreateMessage(task *models.Task) string {
	switch task.Status {
	case models.TaskStatusPending:
		return "Backup creation queued"
	case models.TaskStatusRunning:
		return "Creating backup..."
	case models.TaskStatusCompleted:
		return "Backup created successfully"
	case models.TaskStatusFailed:
		return task.ErrorMessage
	default:
		return "Unknown status"
	}
}

func getBackupRestoreMessage(task *models.Task) string {
	switch task.Status {
	case models.TaskStatusPending:
		return "Backup restore queued"
	case models.TaskStatusRunning:
		return "Restoring from backup..."
	case models.TaskStatusCompleted:
		return "Backup restored successfully"
	case models.TaskStatusFailed:
		return task.ErrorMessage
	default:
		return "Unknown status"
	}
}

func getSnapshotCreateMessage(task *models.Task) string {
	switch task.Status {
	case models.TaskStatusPending:
		return "Snapshot creation queued"
	case models.TaskStatusRunning:
		return getProgressMessage(task.Progress, []string{
			"Validating parameters...",
			"Creating disk snapshot...",
			"Updating snapshot record...",
		})
	case models.TaskStatusCompleted:
		return "Snapshot created successfully"
	case models.TaskStatusFailed:
		return task.ErrorMessage
	default:
		return "Unknown status"
	}
}

func getSnapshotRevertMessage(task *models.Task) string {
	switch task.Status {
	case models.TaskStatusPending:
		return "Snapshot revert queued"
	case models.TaskStatusRunning:
		return getProgressMessage(task.Progress, []string{
			"Stopping VM...",
			"Restoring from snapshot...",
			"Starting VM...",
		})
	case models.TaskStatusCompleted:
		return "Snapshot reverted successfully"
	case models.TaskStatusFailed:
		return task.ErrorMessage
	default:
		return "Unknown status"
	}
}

func getSnapshotDeleteMessage(task *models.Task) string {
	switch task.Status {
	case models.TaskStatusPending:
		return "Snapshot deletion queued"
	case models.TaskStatusRunning:
		return getProgressMessage(task.Progress, []string{
			"Deleting from storage...",
			"Removing snapshot record...",
		})
	case models.TaskStatusCompleted:
		return "Snapshot deleted successfully"
	case models.TaskStatusFailed:
		return task.ErrorMessage
	default:
		return "Unknown status"
	}
}

func getGenericTaskMessage(task *models.Task) string {
	switch task.Status {
	case models.TaskStatusPending:
		return "Task queued"
	case models.TaskStatusRunning:
		if task.Progress > 0 {
			return "Processing..."
		}
		return "Task running"
	case models.TaskStatusCompleted:
		return "Task completed successfully"
	case models.TaskStatusFailed:
		return task.ErrorMessage
	default:
		return "Unknown status"
	}
}

// getProgressMessage returns a message based on progress percentage and a list of steps.
func getProgressMessage(progress int, steps []string) string {
	if len(steps) == 0 {
		return "Processing..."
	}

	// Map progress to step index
	stepIndex := (progress * len(steps)) / 100
	if stepIndex >= len(steps) {
		stepIndex = len(steps) - 1
	}

	return steps[stepIndex]
}