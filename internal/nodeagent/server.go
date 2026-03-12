package nodeagent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"syscall"

	"github.com/AbuGosok/VirtueStack/internal/nodeagent/storage"
	"github.com/AbuGosok/VirtueStack/internal/nodeagent/vm"
	"github.com/AbuGosok/VirtueStack/internal/shared/config"
	"github.com/AbuGosok/VirtueStack/internal/shared/logging"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"libvirt.org/go/libvirt"
)

// Server constants.
const (
	// DefaultListenAddr is the default gRPC listen address for the node agent.
	DefaultListenAddr = ":50052"
	// LibvirtURI is the libvirt connection URI.
	LibvirtURI = "qemu:///system"
)

// Server represents the VirtueStack Node Agent gRPC server.
type Server struct {
	config      *config.NodeAgentConfig
	libvirtConn *libvirt.Connect
	grpcServer  *grpc.Server
	vmManager   *vm.Manager
	rbdManager  *storage.RBDManager
	logger      *slog.Logger
	listenAddr  string
}

// NewServer creates a new Node Agent server.
// It connects to libvirt, sets up mTLS, and initializes the VM manager.
func NewServer(cfg *config.NodeAgentConfig, logger *slog.Logger) (*Server, error) {
	// Connect to libvirt
	libvirtConn, err := libvirt.NewConnect(LibvirtURI)
	if err != nil {
		return nil, fmt.Errorf("connecting to libvirt at %s: %w", LibvirtURI, err)
	}

	// Create VM manager with data directory for persistence
	dataDir := "/var/lib/virtuestack"
	if cfg.CloudInitPath != "" {
		// Use parent directory of CloudInitPath as data directory
		dataDir = filepath.Dir(cfg.CloudInitPath)
	}
	vmManager := vm.NewManager(libvirtConn, logger, dataDir)

	// Create RBD manager for Ceph storage operations
	rbdManager, err := storage.NewRBDManager(cfg.CephConf, cfg.CephUser, cfg.CephPool, logger)
	if err != nil {
		libvirtConn.Close()
		return nil, fmt.Errorf("connecting to ceph: %w", err)
	}

	// Determine listen address
	listenAddr := DefaultListenAddr
	if cfg.ControllerGRPCAddr != "" {
		// Use the configured address if available
		// Note: In production, you may want a separate ListenAddr config
		listenAddr = DefaultListenAddr
	}

	s := &Server{
		config:      cfg,
		libvirtConn: libvirtConn,
		vmManager:   vmManager,
		rbdManager:  rbdManager,
		logger:      logging.WithComponent(logger, "node-agent"),
		listenAddr:  listenAddr,
	}

	// Setup gRPC server with mTLS
	grpcServer, err := s.createGRPCServer()
	if err != nil {
		libvirtConn.Close()
		rbdManager.Close()
		return nil, fmt.Errorf("creating gRPC server: %w", err)
	}
	s.grpcServer = grpcServer

	// Register the gRPC handler
	// Note: When proto is generated, we'll register the generated service.
	// For now, we use a placeholder registration.
	s.registerServices()

	return s, nil
}

// createGRPCServer creates a gRPC server with mTLS configuration.
func (s *Server) createGRPCServer() (*grpc.Server, error) {
	// Load TLS credentials
	tlsConfig, err := s.loadTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("loading TLS config: %w", err)
	}

	// Create gRPC server with TLS credentials
	creds := credentials.NewTLS(tlsConfig)
	opts := []grpc.ServerOption{
		grpc.Creds(creds),
	}

	return grpc.NewServer(opts...), nil
}

// loadTLSConfig loads the mTLS configuration from certificate files.
func (s *Server) loadTLSConfig() (*tls.Config, error) {
	// Load CA certificate
	caCert, err := os.ReadFile(s.config.TLSCAFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append CA certificate")
	}

	// Load node certificate and key
	cert, err := tls.LoadX509KeyPair(s.config.TLSCertFile, s.config.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading server certificate: %w", err)
	}

	// Create TLS config with mutual TLS
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
		MinVersion:   tls.VersionTLS12,
	}

	return tlsConfig, nil
}

// registerServices registers the gRPC services.
func (s *Server) registerServices() {
	handler := newGRPCHandler(s)
	nodeagentpb.RegisterNodeAgentServiceServer(s.grpcServer, handler)
}

// Start starts the gRPC server and begins listening for connections.
func (s *Server) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.listenAddr, err)
	}

	s.logger.Info("starting gRPC server", "address", s.listenAddr, "node_id", s.config.NodeID)

	// Start serving in a goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := s.grpcServer.Serve(listener); err != nil {
			errChan <- fmt.Errorf("serving gRPC: %w", err)
		}
	}()

	// Wait for either context cancellation or serve error
	select {
	case <-ctx.Done():
		s.logger.Info("context cancelled, stopping server")
		s.grpcServer.GracefulStop()
		return ctx.Err()
	case err := <-errChan:
		return err
	}
}

// Stop gracefully stops the gRPC server and closes the libvirt connection.
func (s *Server) Stop() {
	s.logger.Info("stopping node agent server")

	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}

	if s.rbdManager != nil {
		s.rbdManager.Close()
	}

	if s.libvirtConn != nil {
		if _, err := s.libvirtConn.Close(); err != nil {
			s.logger.Error("error closing libvirt connection", "error", err)
		}
	}

	s.logger.Info("node agent server stopped")
}

// getDiskUsage returns the local disk usage percentage for the root filesystem.
func (s *Server) getDiskUsage() float64 {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		s.logger.Warn("could not get disk usage", "error", err)
		return 0
	}
	total := stat.Blocks * uint64(stat.Bsize)
	used := (stat.Blocks - stat.Bavail) * uint64(stat.Bsize)
	if total == 0 {
		return 0
	}
	return float64(used) / float64(total) * 100
}

// getCephPoolStats returns the Ceph pool storage statistics.
func (s *Server) getCephPoolStats() (totalGB, usedGB int64) {
	if s.rbdManager == nil {
		return 0, 0
	}
	stats, err := s.rbdManager.GetPoolStats(context.Background())
	if err != nil {
		s.logger.Warn("could not get ceph pool stats", "error", err)
		return 0, 0
	}
	gb := int64(1024 * 1024 * 1024)
	return stats.TotalBytes / gb, stats.UsedBytes / gb
}

// isCephConnected returns true if the Ceph connection is healthy.
func (s *Server) isCephConnected() bool {
	if s.rbdManager == nil {
		return false
	}
	return s.rbdManager.IsConnected()
}

func (s *Server) isLibvirtAlive() bool {
	alive, err := s.libvirtConn.IsAlive()
	if err != nil {
		return false
	}
	return alive
}

// grpcHandler implements the NodeAgentService gRPC service.
// It satisfies the generated proto interface.
type grpcHandler struct {
	nodeagentpb.UnimplementedNodeAgentServiceServer
	server *Server
}

// newGRPCHandler creates a new gRPC handler.
func newGRPCHandler(server *Server) *grpcHandler {
	return &grpcHandler{server: server}
}

// Ping verifies the node agent service is responsive.
func (h *grpcHandler) Ping(ctx context.Context, req *nodeagentpb.Empty) (*nodeagentpb.PingResponse, error) {
	return &nodeagentpb.PingResponse{
		NodeId:    h.server.config.NodeID,
		Timestamp: timestamppb.Now(),
	}, nil
}

// GetNodeHealth retrieves comprehensive health status of the node.
func (h *grpcHandler) GetNodeHealth(ctx context.Context, req *nodeagentpb.Empty) (*nodeagentpb.NodeHealthResponse, error) {
	resources, err := h.server.vmManager.GetNodeResources(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "getting node resources: %v", err)
	}

	// Calculate percentages
	var cpuPercent, memoryPercent float64
	if resources.TotalVCPU > 0 {
		cpuPercent = float64(resources.UsedVCPU) / float64(resources.TotalVCPU) * 100
	}
	if resources.TotalMemoryMB > 0 {
		memoryPercent = float64(resources.UsedMemoryMB) / float64(resources.TotalMemoryMB) * 100
	}

	// Get local disk usage percentage
	diskPercent := h.server.getDiskUsage()

	return &nodeagentpb.NodeHealthResponse{
		NodeId:           h.server.config.NodeID,
		Healthy:          true,
		CpuPercent:       cpuPercent,
		MemoryPercent:    memoryPercent,
		DiskPercent:      diskPercent,
		VmCount:          resources.VMCount,
		LoadAverage:      resources.LoadAverage[:],
		UptimeSeconds:    resources.UptimeSeconds,
		LibvirtConnected: h.server.libvirtConn != nil && h.server.isLibvirtAlive(),
		CephConnected:    h.server.isCephConnected(),
	}, nil
}

// GetVMStatus retrieves the current operational state of a virtual machine.
func (h *grpcHandler) GetVMStatus(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMStatusResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	status, err := h.server.vmManager.GetStatus(ctx, req.GetVmId())
	if err != nil {
		return nil, h.mapError(err, "getting VM status")
	}

	return &nodeagentpb.VMStatusResponse{
		VmId:          status.VMID,
		Status:        mapStatusToProto(status.Status),
		Vcpu:          status.VCPU,
		MemoryMb:      status.MemoryMB,
		UptimeSeconds: status.UptimeSeconds,
	}, nil
}

// GetVMMetrics retrieves real-time resource utilization metrics for a VM.
func (h *grpcHandler) GetVMMetrics(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMMetricsResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	metrics, err := h.server.vmManager.GetMetrics(ctx, req.GetVmId())
	if err != nil {
		return nil, h.mapError(err, "getting VM metrics")
	}

	return &nodeagentpb.VMMetricsResponse{
		VmId:             metrics.VMID,
		CpuUsagePercent:  metrics.CPUUsagePercent,
		MemoryUsageBytes: metrics.MemoryUsageBytes,
		MemoryTotalBytes: metrics.MemoryTotalBytes,
		DiskReadBytes:    metrics.DiskReadBytes,
		DiskWriteBytes:   metrics.DiskWriteBytes,
		NetworkRxBytes:   metrics.NetworkRXBytes,
		NetworkTxBytes:   metrics.NetworkTXBytes,
	}, nil
}

// GetNodeResources retrieves aggregate resource information for the node.
func (h *grpcHandler) GetNodeResources(ctx context.Context, req *nodeagentpb.Empty) (*nodeagentpb.NodeResourcesResponse, error) {
	resources, err := h.server.vmManager.GetNodeResources(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "getting node resources: %v", err)
	}

	totalDiskGB, usedDiskGB := h.server.getCephPoolStats()

	return &nodeagentpb.NodeResourcesResponse{
		TotalVcpu:     resources.TotalVCPU,
		UsedVcpu:      resources.UsedVCPU,
		TotalMemoryMb: resources.TotalMemoryMB,
		UsedMemoryMb:  resources.UsedMemoryMB,
		TotalDiskGb:   totalDiskGB,
		UsedDiskGb:    usedDiskGB,
		VmCount:       resources.VMCount,
		LoadAverage:   resources.LoadAverage[:],
		UptimeSeconds: resources.UptimeSeconds,
	}, nil
}

// CreateVM provisions a new virtual machine.
func (h *grpcHandler) CreateVM(ctx context.Context, req *nodeagentpb.CreateVMRequest) (*nodeagentpb.CreateVMResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	// Convert request to DomainConfig
	cfg := &vm.DomainConfig{
		VMID:           req.GetVmId(),
		Hostname:       req.GetHostname(),
		VCPU:           int(req.GetVcpu()),
		MemoryMB:       int(req.GetMemoryMb()),
		CephPool:       req.GetCephPool(),
		CephMonitors:   req.GetCephMonitors(),
		CephUser:       req.GetCephUser(),
		CephSecretUUID: req.GetCephSecretUuid(),
		MACAddress:     req.GetMacAddress(),
		IPv4Address:    req.GetIpv4Address(),
		IPv6Address:    req.GetIpv6Address(),
		PortSpeedKbps:  int(req.GetPortSpeedMbps()) * 1000, // Convert Mbps to Kbps
	}

	// Use config defaults if not provided
	if cfg.CephPool == "" {
		cfg.CephPool = h.server.config.CephPool
	}
	if cfg.CephUser == "" {
		cfg.CephUser = h.server.config.CephUser
	}

	result, err := h.server.vmManager.CreateVM(ctx, cfg)
	if err != nil {
		return nil, h.mapError(err, "creating VM")
	}

	return &nodeagentpb.CreateVMResponse{
		VmId:              req.GetVmId(),
		Success:           true,
		LibvirtDomainName: result.DomainName,
		VncPort:           result.VNCPort,
	}, nil
}

// StartVM powers on a stopped virtual machine.
func (h *grpcHandler) StartVM(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	if err := h.server.vmManager.StartVM(ctx, req.GetVmId()); err != nil {
		return nil, h.mapError(err, "starting VM")
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

// StopVM gracefully shuts down a running virtual machine using ACPI.
func (h *grpcHandler) StopVM(ctx context.Context, req *nodeagentpb.StopVMRequest) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	timeout := int(req.GetTimeoutSeconds())
	if timeout <= 0 {
		timeout = 120 // Default timeout
	}

	if err := h.server.vmManager.StopVM(ctx, req.GetVmId(), timeout); err != nil {
		return nil, h.mapError(err, "stopping VM")
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

// ForceStopVM immediately terminates a virtual machine (power off).
func (h *grpcHandler) ForceStopVM(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	if err := h.server.vmManager.ForceStopVM(ctx, req.GetVmId()); err != nil {
		return nil, h.mapError(err, "force stopping VM")
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

// DeleteVM permanently removes a virtual machine and its disk image.
func (h *grpcHandler) DeleteVM(ctx context.Context, req *nodeagentpb.VMIdentifier) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	if err := h.server.vmManager.DeleteVM(ctx, req.GetVmId()); err != nil {
		return nil, h.mapError(err, "deleting VM")
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

// mapError maps internal errors to gRPC status codes.
func (h *grpcHandler) mapError(err error, operation string) error {
	// Import errors package for checking
	// This maps our errors to appropriate gRPC codes
	return status.Errorf(codes.Internal, "%s: %v", operation, err)
}

// mapStatusToProto maps a string status to the proto VMStatus enum.
func mapStatusToProto(status string) nodeagentpb.VMStatus {
	switch status {
	case "running":
		return nodeagentpb.VMStatus_VM_STATUS_RUNNING
	case "stopped":
		return nodeagentpb.VMStatus_VM_STATUS_STOPPED
	case "paused":
		return nodeagentpb.VMStatus_VM_STATUS_PAUSED
	case "shutting_down":
		return nodeagentpb.VMStatus_VM_STATUS_SHUTTING_DOWN
	case "crashed":
		return nodeagentpb.VMStatus_VM_STATUS_CRASHED
	case "migrating":
		return nodeagentpb.VMStatus_VM_STATUS_MIGRATING
	default:
		return nodeagentpb.VMStatus_VM_STATUS_UNKNOWN
	}
}
