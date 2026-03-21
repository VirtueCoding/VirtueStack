package customer

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	defaultSnapshotLimit = 2
	defaultBackupLimit   = 2
	defaultISOLimit      = 2
)

// CreateSnapshotRequest represents the request body for creating a snapshot.
type CreateSnapshotRequest struct {
	VMID string `json:"vm_id" validate:"required,uuid"`
	Name string `json:"name" validate:"required,max=100"`
}

// SnapshotResponse represents the response for snapshot operations.
type SnapshotResponse struct {
	Snapshot *models.Snapshot `json:"snapshot"`
	TaskID   string           `json:"task_id,omitempty"`
}

// ListSnapshots handles GET /snapshots - lists all snapshots for the customer's VMs.
// Supports pagination and filtering by VM ID.
func (h *CustomerHandler) ListSnapshots(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	// Parse pagination
	pagination := models.ParsePagination(c)

	// Build filter
	filter := repository.SnapshotListFilter{
		PaginationParams: pagination,
	}

	// Optional VM ID filter
	if vmID := c.Query("vm_id"); vmID != "" {
		// Verify VM belongs to customer
		if _, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false); err != nil {
			if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
				middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
				return
			}
			middleware.RespondWithError(c, http.StatusInternalServerError, "SNAPSHOT_LIST_FAILED", "Failed to verify VM")
			return
		}
		filter.VMID = &vmID
	}

	snapshots, total, err := h.backupRepo.ListSnapshotsByCustomer(c.Request.Context(), customerID, filter)
	if err != nil {
		h.logger.Error("failed to list snapshots",
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SNAPSHOT_LIST_FAILED", "Failed to retrieve snapshots")
		return
	}

	c.JSON(http.StatusOK, models.ListResponse{
		Data: snapshots,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}

// CreateSnapshot handles POST /snapshots - creates a snapshot for a VM.
// Snapshots are quick point-in-time copies stored in Ceph RBD.
func (h *CustomerHandler) CreateSnapshot(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	var req CreateSnapshotRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Validate UUID
	if _, err := uuid.Parse(req.VMID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Verify VM belongs to customer
	vm, err := h.vmService.GetVM(c.Request.Context(), req.VMID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "SNAPSHOT_CREATE_FAILED", "Failed to verify VM")
		return
	}

	planLimit := defaultSnapshotLimit
	plan, planErr := h.planRepo.GetByID(c.Request.Context(), vm.PlanID)
	if planErr == nil && plan.SnapshotLimit > 0 {
		planLimit = plan.SnapshotLimit
	}

	snapshotCount, countErr := h.backupRepo.CountSnapshotsByVM(c.Request.Context(), vm.ID)
	if countErr == nil && snapshotCount >= planLimit {
		middleware.RespondWithError(c, http.StatusConflict, "SNAPSHOT_LIMIT_EXCEEDED",
			fmt.Sprintf("Snapshot limit reached for this VM (%d/%d). Delete existing snapshots first.", snapshotCount, planLimit))
		return
	}

	// Create snapshot asynchronously
	snapshot, taskID, err := h.backupService.CreateSnapshotAsync(c.Request.Context(), vm.ID, req.Name, customerID)
	if err != nil {
		if errors.Is(err, services.ErrSnapshotQuotaExceeded) {
			middleware.RespondWithError(c, http.StatusConflict, "SNAPSHOT_QUOTA_EXCEEDED", err.Error())
			return
		}
		h.logger.Error("failed to create snapshot",
			"vm_id", req.VMID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SNAPSHOT_CREATE_FAILED", "Internal server error")
		return
	}

	h.logger.Info("snapshot creation initiated via customer API",
		"snapshot_id", snapshot.ID,
		"vm_id", req.VMID,
		"customer_id", customerID,
		"task_id", taskID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusAccepted, SnapshotResponse{
		Snapshot: snapshot,
		TaskID:   taskID,
	})
}

// DeleteSnapshot handles DELETE /snapshots/:id - deletes a snapshot.
// Returns 200 OK on success.
func (h *CustomerHandler) DeleteSnapshot(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	snapshotID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(snapshotID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_SNAPSHOT_ID", "Snapshot ID must be a valid UUID")
		return
	}

	// Get snapshot to verify ownership
	snapshot, err := h.backupRepo.GetSnapshotByID(c.Request.Context(), snapshotID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "SNAPSHOT_NOT_FOUND", "Snapshot not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "SNAPSHOT_DELETE_FAILED", "Failed to retrieve snapshot")
		return
	}

	// Verify snapshot belongs to a VM owned by the customer
	if !h.verifySnapshotOwnership(c.Request.Context(), snapshot.VMID, customerID) {
		middleware.RespondWithError(c, http.StatusNotFound, "SNAPSHOT_NOT_FOUND", "Snapshot not found")
		return
	}

	// Delete snapshot asynchronously
	taskID, err := h.backupService.DeleteSnapshotAsync(c.Request.Context(), snapshotID, customerID)
	if err != nil {
		h.logger.Error("failed to delete snapshot",
			"snapshot_id", snapshotID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SNAPSHOT_DELETE_FAILED", "Internal server error")
		return
	}

	h.logger.Info("snapshot deletion initiated via customer API",
		"snapshot_id", snapshotID,
		"customer_id", customerID,
		"task_id", taskID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusAccepted, gin.H{
		"message":     "Snapshot deletion initiated",
		"snapshot_id": snapshotID,
		"task_id":     taskID,
	})
}

// RestoreSnapshot handles POST /snapshots/:id/restore - restores a VM from a snapshot.
// This is an async operation. The VM may need to be stopped during restore.
func (h *CustomerHandler) RestoreSnapshot(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	snapshotID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(snapshotID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_SNAPSHOT_ID", "Snapshot ID must be a valid UUID")
		return
	}

	// Get snapshot to verify ownership
	snapshot, err := h.backupRepo.GetSnapshotByID(c.Request.Context(), snapshotID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "SNAPSHOT_NOT_FOUND", "Snapshot not found")
			return
		}
		middleware.RespondWithError(c, http.StatusInternalServerError, "SNAPSHOT_RESTORE_FAILED", "Failed to retrieve snapshot")
		return
	}

	// Verify snapshot belongs to a VM owned by the customer
	if !h.verifySnapshotOwnership(c.Request.Context(), snapshot.VMID, customerID) {
		middleware.RespondWithError(c, http.StatusNotFound, "SNAPSHOT_NOT_FOUND", "Snapshot not found")
		return
	}

	// Restore snapshot asynchronously
	taskID, err := h.backupService.RevertSnapshotAsync(c.Request.Context(), snapshotID, customerID)
	if err != nil {
		h.logger.Error("failed to restore snapshot",
			"snapshot_id", snapshotID,
			"vm_id", snapshot.VMID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SNAPSHOT_RESTORE_FAILED", "Internal server error")
		return
	}

	h.logger.Info("snapshot restore initiated via customer API",
		"snapshot_id", snapshotID,
		"vm_id", snapshot.VMID,
		"customer_id", customerID,
		"task_id", taskID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusAccepted, gin.H{
		"message":     "Snapshot restore initiated",
		"snapshot_id": snapshotID,
		"vm_id":       snapshot.VMID,
		"task_id":     taskID,
	})
}

// verifySnapshotOwnership verifies that a VM belongs to the customer.
// This is an alias for verifyVMOwnership for semantic clarity in snapshot contexts.
func (h *CustomerHandler) verifySnapshotOwnership(ctx context.Context, vmID, customerID string) bool {
	return h.verifyVMOwnership(ctx, vmID, customerID)
}
