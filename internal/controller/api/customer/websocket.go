package customer

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	controllermetrics "github.com/AbuGosok/VirtueStack/internal/controller/metrics"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
)

var wsLogger = slog.Default()

const (
	maxConcurrentConnectionsPerIP = 10
	maxIPTrackerEntries           = 100000
	webSocketIdleTimeout          = 5 * time.Minute
	webSocketTotalTimeout         = 5 * time.Minute
	webSocketBufferSize           = 32 * 1024

	// Environment variable for configurable WebSocket origins
	envWebSocketOrigins = "CUSTOMER_WEBSOCKET_ORIGINS"
)

// Default allowed origins for backward compatibility
var defaultAllowedOrigins = []string{
	"https://localhost",
	"https://localhost:3000",
	"https://virtuestack.com",
	"https://app.virtuestack.com",
}

var (
	wsConnectionCounts = make(map[string]int)
	wsConnectionMu     sync.RWMutex

	// Allowed origins loaded from environment variable
	allowedOrigins     []string
	allowedOriginsInit sync.Once
	allowedOriginsErr  error
)

// loadAllowedOrigins loads allowed origins from environment variable or falls back to defaults
func loadAllowedOrigins() ([]string, error) {
	allowedOriginsInit.Do(func() {
		envValue := os.Getenv(envWebSocketOrigins)
		if envValue == "" {
			allowedOrigins = defaultAllowedOrigins
			return
		}

		// Parse comma-separated origins
		origins := strings.Split(envValue, ",")
		for _, origin := range origins {
			origin = strings.TrimSpace(origin)
			if origin == "" {
				continue
			}

			// Validate that origin is a valid URL with http/https scheme
			parsed, err := url.Parse(origin)
			if err != nil {
				allowedOriginsErr = fmt.Errorf("invalid origin %q: %w", origin, err)
				return
			}
			if parsed.Scheme != "http" && parsed.Scheme != "https" {
				allowedOriginsErr = fmt.Errorf("origin %q has unsupported scheme %q (must be http or https)", origin, parsed.Scheme)
				return
			}

			allowedOrigins = append(allowedOrigins, origin)
		}

		if len(allowedOrigins) == 0 {
			allowedOrigins = defaultAllowedOrigins
		}
	})

	return allowedOrigins, allowedOriginsErr
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  webSocketBufferSize,
	WriteBufferSize: webSocketBufferSize,
	CheckOrigin: func(r *http.Request) bool {
		origins, err := loadAllowedOrigins()
		if err != nil {
			return false
		}
		origin := r.Header.Get("Origin")
		if origin == "" {
			return false
		}
		for _, allowed := range origins {
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
		return nil, fmt.Errorf("%w: VM must be running to access console", sharederrors.ErrConflict)
	}

	if vm.NodeID == nil {
		return nil, fmt.Errorf("%w: VM has no node assigned", sharederrors.ErrConflict)
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

	// Validate the console token server-side: it must have been issued for this
	// exact VM and customer, must not be expired, and is invalidated on first use.
	if !h.tokenStore.Validate(token, vmID, customerID) {
		h.logger.Warn("invalid or expired console token in WebSocket request",
			"vm_id", vmID,
			"correlation_id", correlationID,
			"client_ip", clientIP)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
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
	controllermetrics.WSConnectionsActive.Inc()
	defer controllermetrics.WSConnectionsActive.Dec()
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

	h.proxyConsoleStream(c.Request.Context(), ws, conn, vmID, correlationID, ct)
}

// consoleStreamConfig holds console-type-specific configuration for stream proxying.
type consoleStreamConfig struct {
	name           string
	writeMsgType   int
	acceptMsgTypes map[int]bool
}

// consoleStream implements bidirectional streaming between WebSocket and gRPC.
type consoleStream interface {
	Send(data []byte) error
	Recv() ([]byte, error)
	CloseSend() error
}

// vncStream wraps a VNC gRPC stream to implement consoleStream.
type vncStream struct {
	stream nodeagentpb.NodeAgentService_StreamVNCConsoleClient
}

func (s *vncStream) Send(data []byte) error {
	return s.stream.Send(&nodeagentpb.VNCFrame{Data: data})
}

func (s *vncStream) Recv() ([]byte, error) {
	frame, err := s.stream.Recv()
	if err != nil {
		return nil, err
	}
	return frame.Data, nil
}

func (s *vncStream) CloseSend() error {
	return s.stream.CloseSend()
}

// serialStream wraps a Serial gRPC stream to implement consoleStream.
type serialStream struct {
	stream nodeagentpb.NodeAgentService_StreamSerialConsoleClient
}

func (s *serialStream) Send(data []byte) error {
	return s.stream.Send(&nodeagentpb.SerialData{Data: data})
}

func (s *serialStream) Recv() ([]byte, error) {
	data, err := s.stream.Recv()
	if err != nil {
		return nil, err
	}
	return data.Data, nil
}

func (s *serialStream) CloseSend() error {
	return s.stream.CloseSend()
}

// proxyConsoleStream proxies data bidirectionally between WebSocket and gRPC console stream.
// This is a generic implementation that handles both VNC and Serial console types.
func (h *CustomerHandler) proxyConsoleStream(ctx context.Context, ws *websocket.Conn, conn *grpc.ClientConn, vmID, correlationID string, ct consoleType) {
	defer ws.Close()

	config := h.getConsoleConfig(ct)
	stream, err := h.createConsoleStream(ctx, conn, ct)
	if err != nil {
		h.logger.Error("failed to create "+string(ct)+" stream",
			"vm_id", vmID,
			"error", err,
			"correlation_id", correlationID)
		return
	}
	defer stream.CloseSend()

	if err := stream.Send([]byte(vmID)); err != nil {
		h.logger.Error("failed to send VM ID to "+string(ct)+" stream",
			"vm_id", vmID,
			"error", err,
			"correlation_id", correlationID)
		return
	}

	h.runStreamProxy(ctx, ws, stream, vmID, correlationID, config)

	h.logger.Info(string(ct)+" WebSocket connection closed",
		"vm_id", vmID,
		"correlation_id", correlationID)
}

// getConsoleConfig returns console-type-specific configuration.
func (h *CustomerHandler) getConsoleConfig(ct consoleType) consoleStreamConfig {
	if ct == consoleTypeVNC {
		return consoleStreamConfig{
			name:         "VNC",
			writeMsgType: websocket.BinaryMessage,
			acceptMsgTypes: map[int]bool{
				websocket.BinaryMessage: true,
			},
		}
	}
	return consoleStreamConfig{
		name:         "Serial",
		writeMsgType: websocket.TextMessage,
		acceptMsgTypes: map[int]bool{
			websocket.BinaryMessage: true,
			websocket.TextMessage:   true,
		},
	}
}

// createConsoleStream creates a console stream for the given console type.
func (h *CustomerHandler) createConsoleStream(ctx context.Context, conn *grpc.ClientConn, ct consoleType) (consoleStream, error) {
	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	streamCtx, cancel := context.WithTimeout(ctx, webSocketTotalTimeout)
	defer cancel()

	if ct == consoleTypeVNC {
		stream, err := client.StreamVNCConsole(streamCtx)
		if err != nil {
			return nil, err
		}
		return &vncStream{stream: stream}, nil
	}

	stream, err := client.StreamSerialConsole(streamCtx)
	if err != nil {
		return nil, err
	}
	return &serialStream{stream: stream}, nil
}

// runStreamProxy runs the bidirectional proxy between WebSocket and gRPC stream.
func (h *CustomerHandler) runStreamProxy(ctx context.Context, ws *websocket.Conn, stream consoleStream, vmID, correlationID string, config consoleStreamConfig) {
	errChan := make(chan error, 2)
	gCtx, gCancel := context.WithCancel(ctx)
	defer gCancel()

	go h.readFromWebSocket(gCtx, gCancel, ws, stream, vmID, correlationID, config, errChan)
	go h.readFromStream(gCtx, gCancel, ws, stream, vmID, correlationID, config, errChan)

	<-errChan
}

// readFromWebSocket reads messages from WebSocket and sends to gRPC stream.
func (h *CustomerHandler) readFromWebSocket(ctx context.Context, cancel context.CancelFunc, ws *websocket.Conn, stream consoleStream, vmID, correlationID string, config consoleStreamConfig, errChan chan error) {
	defer cancel()

	for {
		select {
		case <-ctx.Done():
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

		if !config.acceptMsgTypes[messageType] {
			continue
		}

		if err := stream.Send(msg); err != nil {
			h.logger.Error("failed to send "+config.name+" data to gRPC",
				"vm_id", vmID,
				"error", err,
				"correlation_id", correlationID)
			errChan <- err
			return
		}
	}
}

// readFromStream reads from gRPC stream and writes to WebSocket.
func (h *CustomerHandler) readFromStream(ctx context.Context, cancel context.CancelFunc, ws *websocket.Conn, stream consoleStream, vmID, correlationID string, config consoleStreamConfig, errChan chan error) {
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		data, err := stream.Recv()
		if err != nil {
			h.logger.Debug(config.name+" stream ended",
				"vm_id", vmID,
				"error", err,
				"correlation_id", correlationID)
			errChan <- err
			return
		}

		ws.SetWriteDeadline(time.Now().Add(webSocketIdleTimeout))

		if err := ws.WriteMessage(config.writeMsgType, data); err != nil {
			h.logger.Error("failed to write "+config.name+" data to WebSocket",
				"vm_id", vmID,
				"error", err,
				"correlation_id", correlationID)
			errChan <- err
			return
		}
	}
}

// proxyVNCStream is kept for backward compatibility and delegates to proxyConsoleStream.
func (h *CustomerHandler) proxyVNCStream(ctx context.Context, ws *websocket.Conn, conn *grpc.ClientConn, vmID, correlationID string) {
	h.proxyConsoleStream(ctx, ws, conn, vmID, correlationID, consoleTypeVNC)
}

// proxySerialStream is kept for backward compatibility and delegates to proxyConsoleStream.
func (h *CustomerHandler) proxySerialStream(ctx context.Context, ws *websocket.Conn, conn *grpc.ClientConn, vmID, correlationID string) {
	h.proxyConsoleStream(ctx, ws, conn, vmID, correlationID, consoleTypeSerial)
}

func checkConnectionLimit(ip string) bool {
	wsConnectionMu.Lock()
	defer wsConnectionMu.Unlock()

	if wsConnectionCounts[ip] >= maxConcurrentConnectionsPerIP {
		return false
	}

	if len(wsConnectionCounts) >= maxIPTrackerEntries {
		if wsConnectionCounts[ip] == 0 {
			wsLogger.Warn("WebSocket IP tracker at max capacity, rejecting new IP",
				"max_entries", maxIPTrackerEntries)
			return false
		}
	}

	wsConnectionCounts[ip]++

	// Inline cleanup of zero-count entries to avoid unbounded map growth.
	// The mutex is already held, so this is safe without a goroutine.
	for trackedIP, count := range wsConnectionCounts {
		if count <= 0 {
			delete(wsConnectionCounts, trackedIP)
		}
	}

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
