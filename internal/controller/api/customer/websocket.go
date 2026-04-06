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
	sharedcrypto "github.com/AbuGosok/VirtueStack/internal/shared/crypto"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
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

// Default allowed origins for WebSocket upgrades.
// F-079: localhost origins are excluded from the default list because they
// are not valid in production. If localhost is required for local development,
// set the CUSTOMER_WEBSOCKET_ORIGINS environment variable explicitly.
// A startup check (InitWebSocketOrigins) will fail fast if the configured
// origins contain localhost in a non-development context.
var defaultAllowedOrigins = []string{
	"https://virtuestack.com",
	"https://app.virtuestack.com",
}

var (
	wsConnectionCounts = make(map[string]int)
	wsConnectionMu     sync.RWMutex

	// allowedOrigins is populated eagerly at startup by InitWebSocketOrigins.
	// F-034: Using lazy sync.Once initialization meant that a misconfigured
	// CUSTOMER_WEBSOCKET_ORIGINS env var would cause every subsequent WebSocket
	// upgrade to fail permanently and silently. Fail-fast at startup instead.
	allowedOrigins []string
)

// InitWebSocketOrigins parses and validates the CUSTOMER_WEBSOCKET_ORIGINS
// environment variable and stores the result for use by the WebSocket upgrader.
// F-034: Must be called at server startup before any WebSocket connections are
// accepted. Returns an error if the env var is set but malformed.
// F-079: Logs a warning (or returns an error in production mode) if any origin
// contains "localhost".
func InitWebSocketOrigins(isProduction bool) error {
	envValue := os.Getenv(envWebSocketOrigins)
	if envValue == "" {
		allowedOrigins = defaultAllowedOrigins
		return nil
	}

	var parsed []string
	for _, origin := range strings.Split(envValue, ",") {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}

		u, err := url.Parse(origin)
		if err != nil {
			return fmt.Errorf("invalid WebSocket origin %q: %w", origin, err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("WebSocket origin %q has unsupported scheme %q (must be http or https)", origin, u.Scheme)
		}

		// F-079: Reject localhost in production; warn in non-production.
		if strings.Contains(u.Hostname(), "localhost") || u.Hostname() == "127.0.0.1" {
			if isProduction {
				return fmt.Errorf("WebSocket origin %q contains localhost which is not allowed in production", origin)
			}
			wsLogger.Warn("WebSocket origin contains localhost; this is not safe for production", "origin", origin)
		}

		parsed = append(parsed, origin)
	}

	if len(parsed) == 0 {
		allowedOrigins = defaultAllowedOrigins
	} else {
		allowedOrigins = parsed
	}
	return nil
}

// getInitializedOrigins returns the currently configured allowed origins.
// If InitWebSocketOrigins has not been called yet, returns the defaults.
func getInitializedOrigins() []string {
	if len(allowedOrigins) == 0 {
		return defaultAllowedOrigins
	}
	return allowedOrigins
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  webSocketBufferSize,
	WriteBufferSize: webSocketBufferSize,
	CheckOrigin: func(r *http.Request) bool {
		// F-034: allowedOrigins is populated eagerly by InitWebSocketOrigins at
		// startup so misconfigured origins fail fast rather than silently
		// blocking all connections after the first upgrade attempt.
		origins := getInitializedOrigins()
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

// @Tags Customer
// @Summary Open VNC websocket
// @Description Upgrades connection to a VNC websocket stream for a customer VM console. Uses normal customer auth; query tokens are optional for legacy compatibility only.
// @Security BearerAuth
// @Security APIKeyAuth
// @Param vmId path string true "VM ID"
// @Success 101 {string} string "Switching Protocols"
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/customer/ws/vnc/{vmId} [get]
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
		middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_VM_ID", "VM ID must be a valid UUID")
		return
	}

	customerID := middleware.GetUserID(c)
	if customerID == "" {
		h.logger.Warn("unauthenticated WebSocket connection attempt",
			"vm_id", vmID,
			"correlation_id", correlationID,
			"client_ip", clientIP)
		middleware.RespondWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	token := c.Query("token")
	if token != "" && !h.tokenStore.Validate(token, vmID, customerID) {
		h.logger.Warn("invalid or expired console token in WebSocket request",
			"vm_id", vmID,
			"console_type", ct,
			"correlation_id", correlationID,
			"client_ip", clientIP)
		middleware.RespondWithError(c, http.StatusUnauthorized, "INVALID_TOKEN", "console token is invalid or expired")
		return
	}

	if !checkConnectionLimit(clientIP) {
		h.logger.Warn("WebSocket connection limit exceeded",
			"client_ip", clientIP,
			"correlation_id", correlationID)
		middleware.RespondWithError(c, http.StatusTooManyRequests, "TOO_MANY_CONNECTIONS", "Too many connections from this IP")
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

		switch {
		case sharederrors.Is(err, sharederrors.ErrNotFound):
			middleware.RespondWithError(c, http.StatusNotFound, "NOT_FOUND", "VM not found")
		case sharederrors.Is(err, sharederrors.ErrConflict):
			middleware.RespondWithError(c, http.StatusConflict, "CONFLICT", err.Error())
		default:
			middleware.RespondWithError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to validate access")
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
		middleware.RespondWithError(c, http.StatusServiceUnavailable, "NODE_AGENT_UNAVAILABLE", "Node agent unavailable")
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
	if err := ws.SetReadDeadline(time.Now().Add(webSocketIdleTimeout)); err != nil {
		h.logger.Warn("failed to set initial WebSocket read deadline",
			"vm_id", vmID,
			"error", err,
			"correlation_id", correlationID)
	}
	ws.SetPongHandler(func(string) error {
		if err := ws.SetReadDeadline(time.Now().Add(webSocketIdleTimeout)); err != nil {
			h.logger.Debug("failed to update WebSocket read deadline in pong handler",
				"vm_id", vmID,
				"error", err,
				"correlation_id", correlationID)
		}
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
	defer func() {
		if err := ws.Close(); err != nil {
			h.logger.Debug("failed to close WebSocket connection",
				"vm_id", vmID,
				"error", err,
				"correlation_id", correlationID)
		}
	}()

	config := h.getConsoleConfig(ct)
	// F-027: Receive and defer the cancel function from createConsoleStream so the
	// stream context lives for the duration of the proxy, not just until createConsoleStream returns.
	stream, streamCancel, err := h.createConsoleStream(ctx, conn, ct, vmID)
	if err != nil {
		h.logger.Error("failed to create "+string(ct)+" stream",
			"vm_id", vmID,
			"error", err,
			"correlation_id", correlationID)
		return
	}
	defer streamCancel()
	defer func() {
		if err := stream.CloseSend(); err != nil {
			h.logger.Debug("failed to close gRPC stream",
				"vm_id", vmID,
				"error", err,
				"correlation_id", correlationID)
		}
	}()

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
// F-027: The cancel function is returned to the caller so that it can be
// deferred in the long-running proxy goroutine. Previously, cancel() was
// deferred inside this function and fired immediately on return, which
// cancelled the stream context before any proxying occurred.
func (h *CustomerHandler) createConsoleStream(ctx context.Context, conn *grpc.ClientConn, ct consoleType, vmID string) (consoleStream, context.CancelFunc, error) {
	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	streamCtx, cancel := context.WithTimeout(ctx, webSocketTotalTimeout)
	streamCtx, err := h.consoleGuestOpContext(streamCtx, vmID)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	if ct == consoleTypeVNC {
		stream, err := client.StreamVNCConsole(streamCtx)
		if err != nil {
			cancel()
			return nil, nil, err
		}
		return &vncStream{stream: stream}, cancel, nil
	}

	stream, err := client.StreamSerialConsole(streamCtx)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return &serialStream{stream: stream}, cancel, nil
}

func (h *CustomerHandler) consoleGuestOpContext(ctx context.Context, vmID string) (context.Context, error) {
	token, err := sharedcrypto.GenerateGuestOpToken(h.guestOpHMACSecret, vmID, time.Now())
	if err != nil {
		return nil, err
	}

	return metadata.AppendToOutgoingContext(ctx, "x-guest-op-token", token), nil
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

		if err := ws.SetReadDeadline(time.Now().Add(webSocketIdleTimeout)); err != nil {
			h.logger.Debug("failed to set WebSocket read deadline",
				"vm_id", vmID,
				"error", err,
				"correlation_id", correlationID)
		}

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

		if err := ws.SetWriteDeadline(time.Now().Add(webSocketIdleTimeout)); err != nil {
			h.logger.Debug("failed to set WebSocket write deadline",
				"vm_id", vmID,
				"error", err,
				"correlation_id", correlationID)
		}

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
