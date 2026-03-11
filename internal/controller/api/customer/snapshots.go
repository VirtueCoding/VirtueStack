package customer

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CreateSnapshotRequest represents the request body for creating a snapshot.
type CreateSnapshotRequest struct {
	VMID string `json:"vm_id" validate:"required,uuid"`
	Name string `json:"name" validate:"required,max=100"`
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
				respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
				return
			}
			respondWithError(c, http.StatusInternalServerError, "SNAPSHOT_LIST_FAILED", "Failed to verify VM")
			return
		}
		filter.VMID = &vmID
	}

	// Get snapshots
	snapshots, total, err := h.backupRepo.ListSnapshots(c.Request.Context(), filter)
	if err != nil {
		h.logger.Error("failed to list snapshots",
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "SNAPSHOT_LIST_FAILED", "Failed to retrieve snapshots")
		return
	}

	// Filter snapshots to only include those for VMs owned by the customer
	customerSnapshots := h.filterSnapshotsByCustomer(c.Request.Context(), snapshots, customerID)

	c.JSON(http.StatusOK, models.ListResponse{
		Data: customerSnapshots,
		Meta: models.NewPaginationMeta(pagination.Page, pagination.PerPage, total),
	})
}

// CreateSnapshot handles POST /snapshots - creates a snapshot for a VM.
// Snapshots are quick point-in-time copies stored in Ceph RBD.
func (h *CustomerHandler) CreateSnapshot(c *gin.Context) {
	customerID := middleware.GetUserID(c)

	var req CreateSnapshotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body: "+err.Error())
		return
	}

	// Validate UUID
	if _, err := uuid.Parse(req.VMID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Verify VM belongs to customer
	vm, err := h.vmService.GetVM(c.Request.Context(), req.VMID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "SNAPSHOT_CREATE_FAILED", "Failed to verify VM")
		return
	}

	// Create snapshot
	snapshot, err := h.backupService.CreateSnapshot(c.Request.Context(), vm.ID, req.Name)
	if err != nil {
		h.logger.Error("failed to create snapshot",
			"vm_id", req.VMID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "SNAPSHOT_CREATE_FAILED", err.Error())
		return
	}

	h.logger.Info("snapshot created via customer API",
		"snapshot_id", snapshot.ID,
		"vm_id", req.VMID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusCreated, models.Response{Data: snapshot})
}

// DeleteSnapshot handles DELETE /snapshots/:id - deletes a snapshot.
// Returns 200 OK on success.
func (h *CustomerHandler) DeleteSnapshot(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	snapshotID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(snapshotID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_SNAPSHOT_ID", "Snapshot ID must be a valid UUID")
		return
	}

	// Get snapshot to verify ownership
	snapshot, err := h.backupRepo.GetSnapshotByID(c.Request.Context(), snapshotID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "SNAPSHOT_NOT_FOUND", "Snapshot not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "SNAPSHOT_DELETE_FAILED", "Failed to retrieve snapshot")
		return
	}

	// Verify snapshot belongs to a VM owned by the customer
	if !h.verifySnapshotOwnership(c.Request.Context(), snapshot.VMID, customerID) {
		respondWithError(c, http.StatusNotFound, "SNAPSHOT_NOT_FOUND", "Snapshot not found")
		return
	}

	// Delete snapshot
	if err := h.backupService.DeleteSnapshot(c.Request.Context(), snapshotID); err != nil {
		h.logger.Error("failed to delete snapshot",
			"snapshot_id", snapshotID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "SNAPSHOT_DELETE_FAILED", err.Error())
		return
	}

	h.logger.Info("snapshot deleted via customer API",
		"snapshot_id", snapshotID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: gin.H{"message": "Snapshot deleted successfully"}})
}

// RestoreSnapshot handles POST /snapshots/:id/restore - restores a VM from a snapshot.
// This is an async operation. The VM may need to be stopped during restore.
func (h *CustomerHandler) RestoreSnapshot(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	snapshotID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(snapshotID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_SNAPSHOT_ID", "Snapshot ID must be a valid UUID")
		return
	}

	// Get snapshot to verify ownership
	snapshot, err := h.backupRepo.GetSnapshotByID(c.Request.Context(), snapshotID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "SNAPSHOT_NOT_FOUND", "Snapshot not found")
			return
		}
		respondWithError(c, http.StatusInternalServerError, "SNAPSHOT_RESTORE_FAILED", "Failed to retrieve snapshot")
		return
	}

	// Verify snapshot belongs to a VM owned by the customer
	if !h.verifySnapshotOwnership(c.Request.Context(), snapshot.VMID, customerID) {
		respondWithError(c, http.StatusNotFound, "SNAPSHOT_NOT_FOUND", "Snapshot not found")
		return
	}

	// Get VM to find the node
	vm, err := h.vmService.GetVM(c.Request.Context(), snapshot.VMID, customerID, false)
	if err != nil {
		respondWithError(c, http.StatusInternalServerError, "SNAPSHOT_RESTORE_FAILED", "Failed to get VM")
		return
	}

	if vm.NodeID == nil {
		respondWithError(c, http.StatusConflict, "VM_NO_NODE", "VM has no node assigned")
		return
	}

	// Restore snapshot - this would typically call the backup service
	// For now, we'll log and return a placeholder response
	h.logger.Info("snapshot restore initiated via customer API",
		"snapshot_id", snapshotID,
		"vm_id", snapshot.VMID,
		"customer_id", customerID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusAccepted, models.Response{Data: gin.H{
		"message":     "Snapshot restore initiated",
		"snapshot_id": snapshotID,
		"vm_id":       snapshot.VMID,
	}})
}

// verifySnapshotOwnership verifies that a VM belongs to the customer.
func (h *CustomerHandler) verifySnapshotOwnership(ctx interface{}, vmID, customerID string) bool {
	_, err := h.vmService.GetVM(ctx, vmID, customerID, false)
	return err == nil
}

// filterSnapshotsByCustomer filters snapshots to only include those for VMs owned by the customer.
func (h *CustomerHandler) filterSnapshotsByCustomer(ctx interface{}, snapshots []models.Snapshot, customerID string) []models.Snapshot {
	var result []models.Snapshot
	for _, snapshot := range snapshots {
		if h.verifySnapshotOwnership(ctx, snapshot.VMID, customerID) {
			result = append(result, snapshot)
		}
	}
	return result
}