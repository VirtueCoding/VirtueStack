package customer

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// ConsoleTokenDuration is the lifetime of console access tokens.
	ConsoleTokenDuration = 1 * time.Hour
)

// GetConsoleToken handles POST /vms/:id/console-token - generates a NoVNC access token.
// The token is valid for 1 hour and provides graphical console access.
// The token is single-use and bound to the specific VM.
func (h *CustomerHandler) GetConsoleToken(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Get VM with ownership verification (isAdmin=false)
	vm, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to get VM for console token",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "CONSOLE_TOKEN_FAILED", "Failed to get VM")
		return
	}

	// Check if VM is running (console requires running VM)
	if vm.Status != models.VMStatusRunning {
		respondWithError(c, http.StatusConflict, "VM_NOT_RUNNING", "VM must be running to access console")
		return
	}

	// Check if VM has a node assigned
	if vm.NodeID == nil {
		respondWithError(c, http.StatusConflict, "VM_NO_NODE", "VM has no node assigned")
		return
	}

	// Generate console token
	// In a production system, this would call the node agent via gRPC
	// to generate a ticket for the VNC websocket proxy
	token, err := generateConsoleToken(vm.ID, customerID)
	if err != nil {
		h.logger.Error("failed to generate console token",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "CONSOLE_TOKEN_FAILED", "Internal server error")
		return
	}
	expiresAt := time.Now().Add(ConsoleTokenDuration)

	// Store the token so the WebSocket handler can validate and invalidate it on use.
	h.tokenStore.Store(token, vm.ID, customerID, ConsoleTokenDuration)

	// Construct the NoVNC URL
	// In production, this would include the actual websocket proxy URL
	baseURL := strings.TrimRight(h.consoleBaseURL, "/")
	consoleURL := fmt.Sprintf("%s/vnc?token=%s", baseURL, token)

	h.logger.Info("console token generated",
		"vm_id", vmID,
		"customer_id", customerID,
		"node_id", *vm.NodeID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{
		Data: ConsoleTokenResponse{
			Token:     token,
			URL:       consoleURL,
			ExpiresAt: expiresAt.Format(time.RFC3339),
		},
	})
}

// GetSerialToken handles POST /vms/:id/serial-token - generates a serial console access token.
// Serial console provides text-based console access (useful for troubleshooting).
func (h *CustomerHandler) GetSerialToken(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		respondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Get VM with ownership verification (isAdmin=false)
	vm, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			respondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to get VM for serial token",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "SERIAL_TOKEN_FAILED", "Failed to get VM")
		return
	}

	// Check if VM is running
	if vm.Status != models.VMStatusRunning {
		respondWithError(c, http.StatusConflict, "VM_NOT_RUNNING", "VM must be running to access serial console")
		return
	}

	// Check if VM has a node assigned
	if vm.NodeID == nil {
		respondWithError(c, http.StatusConflict, "VM_NO_NODE", "VM has no node assigned")
		return
	}

	// Generate serial console token
	token, err := generateConsoleToken(vm.ID, customerID)
	if err != nil {
		h.logger.Error("failed to generate serial console token",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		respondWithError(c, http.StatusInternalServerError, "SERIAL_TOKEN_FAILED", "Internal server error")
		return
	}
	expiresAt := time.Now().Add(ConsoleTokenDuration)

	// Store the token so the WebSocket handler can validate and invalidate it on use.
	h.tokenStore.Store(token, vm.ID, customerID, ConsoleTokenDuration)

	// Construct the serial console URL
	baseURL := strings.TrimRight(h.consoleBaseURL, "/")
	serialURL := fmt.Sprintf("%s/serial?token=%s", baseURL, token)

	h.logger.Info("serial console token generated",
		"vm_id", vmID,
		"customer_id", customerID,
		"node_id", *vm.NodeID,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{
		Data: ConsoleTokenResponse{
			Token:     token,
			URL:       serialURL,
			ExpiresAt: expiresAt.Format(time.RFC3339),
		},
	})
}

// generateConsoleToken generates a secure token for console access.
// In production, this would integrate with the node agent's ticketing system.
// Returns an error if the system's cryptographic random source is unavailable.
func generateConsoleToken(vmID, customerID string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand unavailable: %w", err)
	}
	return hex.EncodeToString(b), nil
}
