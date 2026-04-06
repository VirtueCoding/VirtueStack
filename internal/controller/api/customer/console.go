package customer

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
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

// GetConsoleToken handles POST /vms/:id/console-token - generates a legacy-compatible
// VNC console token and returns the direct authenticated websocket URL.
// The token is valid for 1 hour, single-use, and bound to the specific VM, but
// first-party clients should connect to the returned URL without placing secrets
// in the query string.
// @Tags Customer
// @Summary Get VNC console access details
// @Description Returns the direct authenticated VNC websocket URL for a customer VM and a legacy-compatible short-lived token.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/vms/{id}/console-token [post]
func (h *CustomerHandler) GetConsoleToken(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Get VM with ownership verification (isAdmin=false)
	vm, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false)
	if err != nil {
		h.logFailedAudit(c, "console.vnc_token.issue", "vm", vmID, nil, err)
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to get VM for console token",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "CONSOLE_TOKEN_FAILED", "Failed to get VM")
		return
	}

	// Check if VM is running (console requires running VM)
	if vm.Status != models.VMStatusRunning {
		h.logFailedAudit(c, "console.vnc_token.issue", "vm", vmID, nil, fmt.Errorf("VM must be running to access console"))
		middleware.RespondWithError(c, http.StatusConflict, "VM_NOT_RUNNING", "VM must be running to access console")
		return
	}

	// Check if VM has a node assigned
	if vm.NodeID == nil {
		h.logFailedAudit(c, "console.vnc_token.issue", "vm", vmID, nil, fmt.Errorf("VM has no node assigned"))
		middleware.RespondWithError(c, http.StatusConflict, "VM_NO_NODE", "VM has no node assigned")
		return
	}

	// Generate console token
	// In a production system, this would call the node agent via gRPC
	// to generate a ticket for the VNC websocket proxy
	token, err := generateConsoleToken(vm.ID, customerID)
	if err != nil {
		h.logFailedAudit(c, "console.vnc_token.issue", "vm", vmID, nil, err)
		h.logger.Error("failed to generate console token",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "CONSOLE_TOKEN_FAILED", "Internal server error")
		return
	}
	expiresAt := time.Now().Add(ConsoleTokenDuration)

	// Store the token so the WebSocket handler can validate and invalidate it on use.
	h.tokenStore.Store(token, vm.ID, customerID, ConsoleTokenDuration)

	consoleURL, err := buildConsoleWebSocketURL(h.consoleBaseURL, consoleTypeVNC, vm.ID)
	if err != nil {
		h.logFailedAudit(c, "console.vnc_token.issue", "vm", vmID, nil, err)
		h.logger.Error("failed to build console websocket url",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "CONSOLE_TOKEN_FAILED", "Internal server error")
		return
	}

	h.logger.Info("console token generated",
		"vm_id", vmID,
		"customer_id", customerID,
		"node_id", *vm.NodeID,
		"correlation_id", middleware.GetCorrelationID(c))

	h.logAudit(c, "console.vnc_token.issue", "vm", vmID, nil, true)

	c.JSON(http.StatusOK, models.Response{
		Data: ConsoleTokenResponse{
			Token:     token,
			URL:       consoleURL,
			ExpiresAt: expiresAt.Format(time.RFC3339),
		},
	})
}

// GetSerialToken handles POST /vms/:id/serial-token - generates a legacy-compatible
// serial console token and returns the direct authenticated websocket URL.
// Serial console provides text-based console access (useful for troubleshooting).
// @Tags Customer
// @Summary Get serial console access details
// @Description Returns the direct authenticated serial websocket URL for a customer VM and a legacy-compatible short-lived token.
// @Produce json
// @Security BearerAuth
// @Security APIKeyAuth
// @Param id path string true "VM ID"
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/vms/{id}/serial-token [post]
func (h *CustomerHandler) GetSerialToken(c *gin.Context) {
	customerID := middleware.GetUserID(c)
	vmID := c.Param("id")

	// Validate UUID
	if _, err := uuid.Parse(vmID); err != nil {
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	// Get VM with ownership verification (isAdmin=false)
	vm, err := h.vmService.GetVM(c.Request.Context(), vmID, customerID, false)
	if err != nil {
		h.logFailedAudit(c, "console.serial_token.issue", "vm", vmID, nil, err)
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			middleware.RespondWithError(c, http.StatusNotFound, "VM_NOT_FOUND", "VM not found")
			return
		}
		h.logger.Error("failed to get VM for serial token",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SERIAL_TOKEN_FAILED", "Failed to get VM")
		return
	}

	// Check if VM is running
	if vm.Status != models.VMStatusRunning {
		h.logFailedAudit(c, "console.serial_token.issue", "vm", vmID, nil, fmt.Errorf("VM must be running to access serial console"))
		middleware.RespondWithError(c, http.StatusConflict, "VM_NOT_RUNNING", "VM must be running to access serial console")
		return
	}

	// Check if VM has a node assigned
	if vm.NodeID == nil {
		h.logFailedAudit(c, "console.serial_token.issue", "vm", vmID, nil, fmt.Errorf("VM has no node assigned"))
		middleware.RespondWithError(c, http.StatusConflict, "VM_NO_NODE", "VM has no node assigned")
		return
	}

	// Generate serial console token
	token, err := generateConsoleToken(vm.ID, customerID)
	if err != nil {
		h.logFailedAudit(c, "console.serial_token.issue", "vm", vmID, nil, err)
		h.logger.Error("failed to generate serial console token",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SERIAL_TOKEN_FAILED", "Internal server error")
		return
	}
	expiresAt := time.Now().Add(ConsoleTokenDuration)

	// Store the token so the WebSocket handler can validate and invalidate it on use.
	h.tokenStore.Store(token, vm.ID, customerID, ConsoleTokenDuration)

	serialURL, err := buildConsoleWebSocketURL(h.consoleBaseURL, consoleTypeSerial, vm.ID)
	if err != nil {
		h.logFailedAudit(c, "console.serial_token.issue", "vm", vmID, nil, err)
		h.logger.Error("failed to build serial websocket url",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", middleware.GetCorrelationID(c))
		middleware.RespondWithError(c, http.StatusInternalServerError, "SERIAL_TOKEN_FAILED", "Internal server error")
		return
	}

	h.logger.Info("serial console token generated",
		"vm_id", vmID,
		"customer_id", customerID,
		"node_id", *vm.NodeID,
		"correlation_id", middleware.GetCorrelationID(c))

	h.logAudit(c, "console.serial_token.issue", "vm", vmID, nil, true)

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

func buildConsoleWebSocketURL(baseURL string, ct consoleType, vmID string) (string, error) {
	base := strings.TrimSpace(baseURL)
	if base == "" {
		return "", fmt.Errorf("console base url is empty")
	}

	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse console base url: %w", err)
	}

	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	default:
		return "", fmt.Errorf("unsupported console base url scheme %q", parsed.Scheme)
	}

	pathSuffix := "/api/v1/customer/ws/" + string(ct) + "/" + vmID
	parsed.Path = strings.TrimRight(parsed.Path, "/") + pathSuffix
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed.String(), nil
}
