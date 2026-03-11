package nodeagent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net"
	"os"

	"github.com/AbuGosok/VirtueStack/internal/shared/config"
	"github.com/AbuGosok/VirtueStack/internal/shared/logging"
	"github.com/AbuGosok/VirtueStack/internal/nodeagent/vm"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
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
	config        *config.NodeAgentConfig
	libvirtConn   *libvirt.Connect
	grpcServer    *grpc.Server
	vmManager     *vm.Manager
	logger        *slog.Logger
	listenAddr    string
}

// NewServer creates a new Node Agent server.
// It connects to libvirt, sets up mTLS, and initializes the VM manager.
func NewServer(cfg *config.NodeAgentConfig, logger *slog.Logger) (*Server, error) {
	// Connect to libvirt
	libvirtConn, err := libvirt.NewConnect(LibvirtURI)
	if err != nil {
		return nil, fmt.Errorf("connecting to libvirt at %s: %w", LibvirtURI, err)
	}

	// Create VM manager
	vmManager := vm.NewManager(libvirtConn, logger)

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
		logger:      logging.WithComponent(logger, "node-agent"),
		listenAddr:  listenAddr,
	}

	// Setup gRPC server with mTLS
	grpcServer, err := s.createGRPCServer()
	if err != nil {
		libvirtConn.Close()
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
// Note: When proto is generated, this will register the NodeAgentServiceServer.
func (s *Server) registerServices() {
	// TODO: Register the generated proto service when available.
	// The service will be registered like:
	// nodeagentpb.RegisterNodeAgentServiceServer(s.grpcServer, &grpcHandler{...})
	//
	// For now, we prepare the handler that will implement the service interface.
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

	if s.libvirtConn != nil {
		if err := s.libvirtConn.Close(); err != nil {
			s.logger.Error("error closing libvirt connection", "error", err)
		}
	}

	s.logger.Info("node agent server stopped")
}

// grpcHandler implements the NodeAgentService gRPC service.
// It will satisfy the generated proto interface when available.
type grpcHandler struct {
	server *Server
}

// newGRPCHandler creates a new gRPC handler.
func newGRPCHandler(server *Server) *grpcHandler {
	return &grpcHandler{server: server}
}

// Ping verifies the node agent service is responsive.
func (h *grpcHandler) Ping(ctx context.Context, req *PingRequest) (*PingResponse, error) {
	return &PingResponse{
		NodeID:    h.server.config.NodeID,
		Timestamp: nil, // Will be set to current time when proto is generated
	}, nil
}

// GetNodeHealth retrieves comprehensive health status of the node.
func (h *grpcHandler) GetNodeHealth(ctx context.Context, req *Empty) (*NodeHealthResponse, error) {
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

	return &NodeHealthResponse{
		NodeID:          h.server.config.NodeID,
		Healthy:         true,
		CPUPercent:      cpuPercent,
		MemoryPercent:   memoryPercent,
		DiskPercent:     0, // TODO: Implement disk usage calculation
		VMCount:         resources.VMCount,
		LoadAverage:     resources.LoadAverage[:],
		UptimeSeconds:   resources.UptimeSeconds,
		LibvirtConnected: h.server.libvirtConn != nil && h.server.libvirtConn.IsAlive() == 1,
		CephConnected:   false, // TODO: Implement Ceph connection check
	}, nil
}

// GetVMStatus retrieves the current operational state of a virtual machine.
func (h *grpcHandler) GetVMStatus(ctx context.Context, req *VMIdentifier) (*VMStatusResponse, error) {
	if req.VMID == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	status, err := h.server.vmManager.GetStatus(ctx, req.VMID)
	if err != nil {
		return nil, h.mapError(err, "getting VM status")
	}

	return &VMStatusResponse{
		VMID:          status.VMID,
		Status:        mapStatusToProto(status.Status),
		VCPU:          status.VCPU,
		MemoryMB:      status.MemoryMB,
		UptimeSeconds: status.UptimeSeconds,
	}, nil
}

// GetVMMetrics retrieves real-time resource utilization metrics for a VM.
func (h *grpcHandler) GetVMMetrics(ctx context.Context, req *VMIdentifier) (*VMMetricsResponse, error) {
	if req.VMID == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	metrics, err := h.server.vmManager.GetMetrics(ctx, req.VMID)
	if err != nil {
		return nil, h.mapError(err, "getting VM metrics")
	}

	return &VMMetricsResponse{
		VMID:             metrics.VMID,
		CPUUsagePercent:  metrics.CPUUsagePercent,
		MemoryUsageBytes: metrics.MemoryUsageBytes,
		MemoryTotalBytes: metrics.MemoryTotalBytes,
		DiskReadBytes:    metrics.DiskReadBytes,
		DiskWriteBytes:   metrics.DiskWriteBytes,
		NetworkRXBytes:   metrics.NetworkRXBytes,
		NetworkTXBytes:   metrics.NetworkTXBytes,
	}, nil
}

// GetNodeResources retrieves aggregate resource information for the node.
func (h *grpcHandler) GetNodeResources(ctx context.Context, req *Empty) (*NodeResourcesResponse, error) {
	resources, err := h.server.vmManager.GetNodeResources(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "getting node resources: %v", err)
	}

	return &NodeResourcesResponse{
		TotalVCPU:     resources.TotalVCPU,
		UsedVCPU:      resources.UsedVCPU,
		TotalMemoryMB: resources.TotalMemoryMB,
		UsedMemoryMB:  resources.UsedMemoryMB,
		TotalDiskGB:   0, // TODO: Implement disk calculation
		UsedDiskGB:    0, // TODO: Implement disk calculation
		VMCount:       resources.VMCount,
		LoadAverage:   resources.LoadAverage[:],
		UptimeSeconds: resources.UptimeSeconds,
	}, nil
}

// CreateVM provisions a new virtual machine.
func (h *grpcHandler) CreateVM(ctx context.Context, req *CreateVMRequest) (*CreateVMResponse, error) {
	if req.VMID == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	// Convert request to DomainConfig
	cfg := &vm.DomainConfig{
		VMID:           req.VMID,
		Hostname:       req.Hostname,
		VCPU:           int(req.VCPU),
		MemoryMB:       int(req.MemoryMB),
		CephPool:       req.CephPool,
		CephMonitors:   req.CephMonitors,
		CephUser:       req.CephUser,
		CephSecretUUID: req.CephSecretUUID,
		MACAddress:     req.MACAddress,
		IPv4Address:    req.IPv4Address,
		IPv6Address:    req.IPv6Address,
		PortSpeedKbps:  int(req.PortSpeedMbps) * 1000, // Convert Mbps to Kbps
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

	return &CreateVMResponse{
		VMID:             req.VMID,
		Success:          true,
		LibvirtDomainName: result.DomainName,
		VNCPort:          result.VNCPort,
	}, nil
}

// StartVM powers on a stopped virtual machine.
func (h *grpcHandler) StartVM(ctx context.Context, req *VMIdentifier) (*VMOperationResponse, error) {
	if req.VMID == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	if err := h.server.vmManager.StartVM(ctx, req.VMID); err != nil {
		return nil, h.mapError(err, "starting VM")
	}

	return &VMOperationResponse{
		VMID:    req.VMID,
		Success: true,
	}, nil
}

// StopVM gracefully shuts down a running virtual machine using ACPI.
func (h *grpcHandler) StopVM(ctx context.Context, req *StopVMRequest) (*VMOperationResponse, error) {
	if req.VMID == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	timeout := int(req.TimeoutSeconds)
	if timeout <= 0 {
		timeout = 120 // Default timeout
	}

	if err := h.server.vmManager.StopVM(ctx, req.VMID, timeout); err != nil {
		return nil, h.mapError(err, "stopping VM")
	}

	return &VMOperationResponse{
		VMID:    req.VMID,
		Success: true,
	}, nil
}

// ForceStopVM immediately terminates a virtual machine (power off).
func (h *grpcHandler) ForceStopVM(ctx context.Context, req *VMIdentifier) (*VMOperationResponse, error) {
	if req.VMID == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	if err := h.server.vmManager.ForceStopVM(ctx, req.VMID); err != nil {
		return nil, h.mapError(err, "force stopping VM")
	}

	return &VMOperationResponse{
		VMID:    req.VMID,
		Success: true,
	}, nil
}

// DeleteVM permanently removes a virtual machine and its disk image.
func (h *grpcHandler) DeleteVM(ctx context.Context, req *VMIdentifier) (*VMOperationResponse, error) {
	if req.VMID == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	if err := h.server.vmManager.DeleteVM(ctx, req.VMID); err != nil {
		return nil, h.mapError(err, "deleting VM")
	}

	return &VMOperationResponse{
		VMID:    req.VMID,
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
// This will use the generated proto enum when available.
func mapStatusToProto(status string) int32 {
	switch status {
	case "running":
		return 1 // VM_STATUS_RUNNING
	case "stopped":
		return 2 // VM_STATUS_STOPPED
	case "paused":
		return 3 // VM_STATUS_PAUSED
	case "shutting_down":
		return 4 // VM_STATUS_SHUTTING_DOWN
	case "crashed":
		return 5 // VM_STATUS_CRASHED
	case "migrating":
		return 6 // VM_STATUS_MIGRATING
	default:
		return 0 // VM_STATUS_UNKNOWN
	}
}

// ============================================================================
// Local message types (mirrors proto messages until proto is generated)
// ============================================================================

// Empty represents an empty message.
type Empty struct{}

// VMIdentifier uniquely identifies a virtual machine.
type VMIdentifier struct {
	VMID string
}

// VMOperationResponse is the standard response for VM operations.
type VMOperationResponse struct {
	VMID         string
	Success      bool
	ErrorMessage string
}

// PingResponse verifies that the node agent is responsive.
type PingResponse struct {
	NodeID    string
	Timestamp any // Will be *timestamppb.Timestamp when proto is generated
}

// NodeHealthResponse contains comprehensive health information for the node.
type NodeHealthResponse struct {
	NodeID           string
	Healthy          bool
	CPUPercent       float64
	MemoryPercent    float64
	DiskPercent      float64
	VMCount          int32
	LoadAverage      []float64
	UptimeSeconds    int64
	LibvirtConnected bool
	CephConnected    bool
}

// VMStatusResponse contains the current state of a virtual machine.
type VMStatusResponse struct {
	VMID          string
	Status        int32
	VCPU          int32
	MemoryMB      int32
	UptimeSeconds int64
}

// VMMetricsResponse contains real-time resource utilization for a VM.
type VMMetricsResponse struct {
	VMID             string
	CPUUsagePercent  float64
	MemoryUsageBytes int64
	MemoryTotalBytes int64
	DiskReadBytes    int64
	DiskWriteBytes   int64
	NetworkRXBytes   int64
	NetworkTXBytes   int64
}

// NodeResourcesResponse contains aggregate resource information for the node.
type NodeResourcesResponse struct {
	TotalVCPU     int32
	UsedVCPU      int32
	TotalMemoryMB int64
	UsedMemoryMB  int64
	TotalDiskGB   int64
	UsedDiskGB    int64
	VMCount       int32
	LoadAverage   []float64
	UptimeSeconds int64
}

// CreateVMRequest contains all parameters needed to provision a new VM.
type CreateVMRequest struct {
	VMID             string
	Hostname         string
	VCPU             int32
	MemoryMB         int32
	DiskGB           int32
	TemplateRBDImage string
	TemplateRBDSnapshot string
	RootPasswordHash string
	SSHPublicKeys    []string
	IPv4Address      string
	IPv4Gateway      string
	IPv4Netmask      string
	IPv6Address      string
	IPv6Gateway      string
	MACAddress       string
	PortSpeedMbps    int32
	BandwidthLimitGB int64
	CephMonitors     []string
	CephUser         string
	CephSecretUUID   string
	CephPool         string
	Nameservers      []string
}

// CreateVMResponse is returned after a VM creation attempt.
type CreateVMResponse struct {
	VMID              string
	Success           bool
	LibvirtDomainName string
	VNCPort           int32
	ErrorMessage      string
}

// StopVMRequest specifies parameters for a graceful VM shutdown.
type StopVMRequest struct {
	VMID           string
	TimeoutSeconds int32
}