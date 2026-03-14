package customer

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
)

const (
	maxConcurrentConnectionsPerIP = 5
	webSocketIdleTimeout          = 30 * time.Second
	webSocketTotalTimeout         = 5 * time.Minute
	webSocketBufferSize           = 32 * 1024
)

var (
	wsConnectionCounts = make(map[string]int)
	wsConnectionMu     sync.RWMutex
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  webSocketBufferSize,
	WriteBufferSize: webSocketBufferSize,
	CheckOrigin: func(r *http.Request) bool {
		allowedOrigins := []string{
			"https://localhost",
			"https://localhost:3000",
			"https://virtuestack.com",
			"https://app.virtuestack.com",
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			return false
		}
		for _, allowed := range allowedOrigins {
			if origin == allowed {
				return true
			}
		}
		return false
	},
}

type consoleType string

const (
	consoleTypeVNC    consoleType = "vnc"
	consoleTypeSerial consoleType = "serial"
)

func (h *CustomerHandler) validateConsoleAccess(ctx context.Context, customerID, vmID string) (*models.VM, error) {
	vm, err := h.vmService.GetVM(ctx, vmID, customerID, false)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrForbidden) || sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, sharederrors.ErrNotFound
		}
		return nil, err
	}

	if vm.Status != models.VMStatusRunning {
		return nil, sharederrors.New(sharederrors.ErrConflict, "VM must be running to access console")
	}

	if vm.NodeID == nil {
		return nil, sharederrors.New(sharederrors.ErrConflict, "VM has no node assigned")
	}

	return vm, nil
}

func (h *CustomerHandler) getNodeConnection(ctx context.Context, nodeID string) (*grpc.ClientConn, error) {
	node, err := h.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := h.nodeAgent.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return nil, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	return conn, nil
}

func (h *CustomerHandler) HandleVNCWebSocket(c *gin.Context) {
	h.handleConsoleWebSocket(c, consoleTypeVNC)
}

func (h *CustomerHandler) HandleSerialWebSocket(c *gin.Context) {
	h.handleConsoleWebSocket(c, consoleTypeSerial)
}

func (h *CustomerHandler) handleConsoleWebSocket(c *gin.Context, ct consoleType) {
	vmID := c.Param("vmId")
	correlationID := middleware.GetCorrelationID(c)
	clientIP := c.ClientIP()

	if _, err := uuid.Parse(vmID); err != nil {
		h.logger.Warn("invalid VM ID format in WebSocket request",
			"vm_id", vmID,
			"correlation_id", correlationID,
			"client_ip", clientIP)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid VM ID format"})
		return
	}

	token := c.Query("token")
	if token == "" {
		h.logger.Warn("missing console token in WebSocket request",
			"vm_id", vmID,
			"correlation_id", correlationID,
			"client_ip", clientIP)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing token"})
		return
	}

	customerID := middleware.GetUserID(c)
	if customerID == "" {
		h.logger.Warn("unauthenticated WebSocket connection attempt",
			"vm_id", vmID,
			"correlation_id", correlationID,
			"client_ip", clientIP)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication required"})
		return
	}

	if !checkConnectionLimit(clientIP) {
		h.logger.Warn("WebSocket connection limit exceeded",
			"client_ip", clientIP,
			"correlation_id", correlationID)
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many connections from this IP"})
		return
	}
	defer releaseConnection(clientIP)

	vm, err := h.validateConsoleAccess(c.Request.Context(), customerID, vmID)
	if err != nil {
		h.logger.Error("console access validation failed",
			"vm_id", vmID,
			"customer_id", customerID,
			"error", err,
			"correlation_id", correlationID)

		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "VM not found"})
		} else if sharederrors.Is(err, sharederrors.ErrConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate access"})
		}
		return
	}

	nodeID := *vm.NodeID

	conn, err := h.getNodeConnection(c.Request.Context(), nodeID)
	if err != nil {
		h.logger.Error("failed to connect to node agent",
			"vm_id", vmID,
			"node_id", nodeID,
			"error", err,
			"correlation_id", correlationID)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Node agent unavailable"})
		return
	}

	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("WebSocket upgrade failed",
			"vm_id", vmID,
			"error", err,
			"correlation_id", correlationID)
		return
	}

	ws.SetReadLimit(webSocketBufferSize)
	ws.SetReadDeadline(time.Now().Add(webSocketIdleTimeout))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(webSocketIdleTimeout))
		return nil
	})

	h.logger.Info("WebSocket connection established",
		"vm_id", vmID,
		"node_id", nodeID,
		"customer_id", customerID,
		"console_type", ct,
		"correlation_id", correlationID)

	if ct == consoleTypeVNC {
		h.proxyVNCStream(c.Request.Context(), ws, conn, vmID, correlationID)
	} else {
		h.proxySerialStream(c.Request.Context(), ws, conn, vmID, correlationID)
	}
}

func (h *CustomerHandler) proxyVNCStream(ctx context.Context, ws *websocket.Conn, conn *grpc.ClientConn, vmID, correlationID string) {
	defer ws.Close()

	client := nodeagentpb.NewNodeAgentServiceClient(conn)

	streamCtx, cancel := context.WithTimeout(ctx, webSocketTotalTimeout)
	defer cancel()

	stream, err := client.StreamVNCConsole(streamCtx)
	if err != nil {
		h.logger.Error("failed to create VNC stream",
			"vm_id", vmID,
			"error", err,
			"correlation_id", correlationID)
		return
	}
	defer stream.CloseSend()

	if err := stream.Send(&nodeagentpb.VNCFrame{Data: []byte(vmID)}); err != nil {
		h.logger.Error("failed to send VM ID to VNC stream",
			"vm_id", vmID,
			"error", err,
			"correlation_id", correlationID)
		return
	}

	errChan := make(chan error, 2)
	gCtx, gCancel := context.WithCancel(streamCtx)
	defer gCancel()

	go func() {
		defer gCancel()

		for {
			select {
			case <-gCtx.Done():
				return
			default:
			}

			ws.SetReadDeadline(time.Now().Add(webSocketIdleTimeout))

			messageType, msg, err := ws.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					h.logger.Debug("WebSocket closed",
						"vm_id", vmID,
						"correlation_id", correlationID)
				}
				errChan <- err
				return
			}

			if messageType != websocket.BinaryMessage {
				continue
			}

			if err := stream.Send(&nodeagentpb.VNCFrame{Data: msg}); err != nil {
				h.logger.Error("failed to send VNC frame to gRPC",
					"vm_id", vmID,
					"error", err,
					"correlation_id", correlationID)
				errChan <- err
				return
			}
		}
	}()

	go func() {
		defer gCancel()

		for {
			select {
			case <-gCtx.Done():
				return
			default:
			}

			frame, err := stream.Recv()
			if err != nil {
				h.logger.Debug("VNC stream ended",
					"vm_id", vmID,
					"error", err,
					"correlation_id", correlationID)
				errChan <- err
				return
			}

			ws.SetWriteDeadline(time.Now().Add(webSocketIdleTimeout))

			if err := ws.WriteMessage(websocket.BinaryMessage, frame.Data); err != nil {
				h.logger.Error("failed to write VNC frame to WebSocket",
					"vm_id", vmID,
					"error", err,
					"correlation_id", correlationID)
				errChan <- err
				return
			}
		}
	}()

	<-errChan

	h.logger.Info("VNC WebSocket connection closed",
		"vm_id", vmID,
		"correlation_id", correlationID)
}

func (h *CustomerHandler) proxySerialStream(ctx context.Context, ws *websocket.Conn, conn *grpc.ClientConn, vmID, correlationID string) {
	defer ws.Close()

	client := nodeagentpb.NewNodeAgentServiceClient(conn)

	streamCtx, cancel := context.WithTimeout(ctx, webSocketTotalTimeout)
	defer cancel()

	stream, err := client.StreamSerialConsole(streamCtx)
	if err != nil {
		h.logger.Error("failed to create serial stream",
			"vm_id", vmID,
			"error", err,
			"correlation_id", correlationID)
		return
	}
	defer stream.CloseSend()

	if err := stream.Send(&nodeagentpb.SerialData{Data: []byte(vmID)}); err != nil {
		h.logger.Error("failed to send VM ID to serial stream",
			"vm_id", vmID,
			"error", err,
			"correlation_id", correlationID)
		return
	}

	errChan := make(chan error, 2)
	gCtx, gCancel := context.WithCancel(streamCtx)
	defer gCancel()

	go func() {
		defer gCancel()

		for {
			select {
			case <-gCtx.Done():
				return
			default:
			}

			ws.SetReadDeadline(time.Now().Add(webSocketIdleTimeout))

			messageType, msg, err := ws.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					h.logger.Debug("WebSocket closed",
						"vm_id", vmID,
						"correlation_id", correlationID)
				}
				errChan <- err
				return
			}

			if messageType != websocket.BinaryMessage && messageType != websocket.TextMessage {
				continue
			}

			if err := stream.Send(&nodeagentpb.SerialData{Data: msg}); err != nil {
				h.logger.Error("failed to send serial data to gRPC",
					"vm_id", vmID,
					"error", err,
					"correlation_id", correlationID)
				errChan <- err
				return
			}
		}
	}()

	go func() {
		defer gCancel()

		for {
			select {
			case <-gCtx.Done():
				return
			default:
			}

			data, err := stream.Recv()
			if err != nil {
				h.logger.Debug("Serial stream ended",
					"vm_id", vmID,
					"error", err,
					"correlation_id", correlationID)
				errChan <- err
				return
			}

			ws.SetWriteDeadline(time.Now().Add(webSocketIdleTimeout))

			if err := ws.WriteMessage(websocket.TextMessage, data.Data); err != nil {
				h.logger.Error("failed to write serial data to WebSocket",
					"vm_id", vmID,
					"error", err,
					"correlation_id", correlationID)
				errChan <- err
				return
			}
		}
	}()

	<-errChan

	h.logger.Info("Serial WebSocket connection closed",
		"vm_id", vmID,
		"correlation_id", correlationID)
}

func checkConnectionLimit(ip string) bool {
	wsConnectionMu.Lock()
	defer wsConnectionMu.Unlock()

	if wsConnectionCounts[ip] >= maxConcurrentConnectionsPerIP {
		return false
	}
	wsConnectionCounts[ip]++
	return true
}

func releaseConnection(ip string) {
	wsConnectionMu.Lock()
	defer wsConnectionMu.Unlock()

	if wsConnectionCounts[ip] > 0 {
		wsConnectionCounts[ip]--
		if wsConnectionCounts[ip] == 0 {
			delete(wsConnectionCounts, ip)
		}
	}
}
