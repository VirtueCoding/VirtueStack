package nodeagent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/nodeagent/metrics"
	"github.com/AbuGosok/VirtueStack/internal/nodeagent/network"
	"github.com/AbuGosok/VirtueStack/internal/nodeagent/storage"
	"github.com/AbuGosok/VirtueStack/internal/nodeagent/vm"
	"github.com/AbuGosok/VirtueStack/internal/shared/config"
	"github.com/AbuGosok/VirtueStack/internal/shared/logging"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
)

// initializeStorage creates the appropriate storage backend based on configuration.
// Returns the StorageBackend interface, storage type, and template manager.
func initializeStorage(cfg *config.NodeAgentConfig, logger *slog.Logger) (storage.StorageBackend, storage.StorageType, any, error) {
	storageBackend := cfg.StorageBackend
	if storageBackend == "" {
		storageBackend = "ceph"
	}

	switch storageBackend {
	case "qcow":
		qcowMgr, err := storage.NewQCOWManager(cfg.StoragePath, logger)
		if err != nil {
			return nil, storage.StorageTypeQCOW, nil, fmt.Errorf("creating QCOW manager: %w", err)
		}

		templatesPath := cfg.StoragePath + "/templates"
		vmsPath := cfg.StoragePath + "/vms"
		qcowTemplateMgr, err := storage.NewQCOWTemplateManager(templatesPath, vmsPath, logger)
		if err != nil {
			return nil, storage.StorageTypeQCOW, nil, fmt.Errorf("creating QCOW template manager: %w", err)
		}

		logger.Info("initialized QCOW storage backend", "path", cfg.StoragePath)
		return qcowMgr, storage.StorageTypeQCOW, qcowTemplateMgr, nil

	default:
		rbdMgr, err := storage.NewRBDManager(cfg.CephConf, cfg.CephUser, cfg.CephPool, logger)
		if err != nil {
			return nil, storage.StorageTypeCEPH, nil, fmt.Errorf("connecting to ceph: %w", err)
		}

		logger.Info("initialized Ceph RBD storage backend", "pool", cfg.CephPool)
		return rbdMgr, storage.StorageTypeCEPH, nil, nil
	}
}

// Server represents the VirtueStack Node Agent gRPC server.
type Server struct {
	config             *config.NodeAgentConfig
	libvirtConn        *libvirt.Connect
	grpcServer         *grpc.Server
	vmManager          *vm.Manager
	storageBackend     storage.StorageBackend
	storageType        storage.StorageType
	templateMgr        any // *storage.TemplateManager for ceph, *storage.QCOWTemplateManager for qcow
	abusePreventionMgr *network.AbusePreventionManager
	logger             *slog.Logger
	listenAddr         string
	metricsAddr        string
	bgWg               sync.WaitGroup
}

// NewServer creates a new Node Agent server.
// It connects to libvirt, sets up mTLS, and initializes the VM manager.
func NewServer(cfg *config.NodeAgentConfig, logger *slog.Logger) (*Server, error) {
	// Connect to libvirt using configured URI or default
	libvirtURI := cfg.LibvirtURI
	if libvirtURI == "" {
		libvirtURI = "qemu:///system"
	}
	libvirtConn, err := libvirt.NewConnect(libvirtURI)
	if err != nil {
		return nil, fmt.Errorf("connecting to libvirt at %s: %w", libvirtURI, err)
	}

	// Create VM manager with data directory for persistence
	dataDir := cfg.DataDir
	if dataDir == "" && cfg.CloudInitPath != "" {
		// Use parent directory of CloudInitPath as data directory
		dataDir = filepath.Dir(cfg.CloudInitPath)
	}
	if dataDir == "" {
		dataDir = "/var/lib/virtuestack"
	}
	vmManager := vm.NewManager(libvirtConn, logger, dataDir)

	// Initialize storage backend based on configuration
	storageBackend, storageType, templateMgr, err := initializeStorage(cfg, logger)
	if err != nil {
		if _, closeErr := libvirtConn.Close(); closeErr != nil {
			logger.Error("error closing libvirt connection during cleanup", "error", closeErr)
		}
		return nil, fmt.Errorf("initializing storage: %w", err)
	}

	// Determine listen address
	listenAddr := DefaultListenAddr
	if cfg.ControllerGRPCAddr != "" {
		listenAddr = cfg.ControllerGRPCAddr
	}

	metricsAddr := cfg.MetricsAddr
	if metricsAddr == "" {
		metricsAddr = ":9091"
	}

	s := &Server{
		config:             cfg,
		libvirtConn:        libvirtConn,
		vmManager:          vmManager,
		storageBackend:     storageBackend,
		storageType:        storageType,
		templateMgr:        templateMgr,
		abusePreventionMgr: network.NewAbusePreventionManager(logger),
		logger:             logging.WithComponent(logger, "node-agent"),
		listenAddr:         listenAddr,
		metricsAddr:        metricsAddr,
	}

	// Setup gRPC server with mTLS
	grpcServer, err := s.createGRPCServer()
	if err != nil {
		if _, closeErr := libvirtConn.Close(); closeErr != nil {
			s.logger.Error("error closing libvirt connection during cleanup", "error", closeErr)
		}
		s.closeStorage()
		return nil, fmt.Errorf("creating gRPC server: %w", err)
	}
	s.grpcServer = grpcServer

	// Register the gRPC handler
	s.registerServices()

	return s, nil
}

// grpcMaxMsgSize is the maximum allowed gRPC message size (64 MiB).
// This prevents a malicious or buggy client from triggering OOM by sending
// arbitrarily large messages.
const grpcMaxMsgSize = 64 * 1024 * 1024

// createGRPCServer creates a gRPC server with mTLS configuration.
func (s *Server) createGRPCServer() (*grpc.Server, error) {
	// Load TLS credentials
	tlsConfig, err := s.loadTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("loading TLS config: %w", err)
	}

	// Create gRPC server with TLS credentials and bounded message sizes.
	creds := credentials.NewTLS(tlsConfig)
	opts := []grpc.ServerOption{
		grpc.Creds(creds),
		grpc.MaxRecvMsgSize(grpcMaxMsgSize),
		grpc.MaxSendMsgSize(grpcMaxMsgSize),
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

	// Create TLS config with mutual TLS.
	// MinVersion is set to TLS 1.3 to avoid known weaknesses in TLS 1.2
	// (BEAST, CRIME, POODLE, LUCKY13, RC4, CBC-mode cipher suites).
	// If legacy node agents that support only TLS 1.2 must be supported,
	// temporarily lower this to tls.VersionTLS12 during the transition.
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
		MinVersion:   tls.VersionTLS13,
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
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.listenAddr, err)
	}

	s.logger.Info("starting gRPC server", "address", s.listenAddr, "node_id", s.config.NodeID)

	s.startMetricsCollector(ctx)
	s.startMetricsHTTPServer(ctx)

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

	// Wait for background goroutines to complete
	s.bgWg.Wait()

	s.closeStorage()

	if s.libvirtConn != nil {
		if _, err := s.libvirtConn.Close(); err != nil {
			s.logger.Error("error closing libvirt connection", "error", err)
		}
	}

	s.logger.Info("node agent server stopped")
}

// Default metrics collection interval (can be overridden via config).
const defaultMetricsCollectInterval = 60 * time.Second

func (s *Server) startMetricsCollector(ctx context.Context) {
	// Parse metrics collect interval from config (default to 60s)
	collectInterval := defaultMetricsCollectInterval
	if s.config.MetricsCollectInterval != "" {
		if parsed, err := time.ParseDuration(s.config.MetricsCollectInterval); err == nil {
			collectInterval = parsed
		}
	}

	s.trackBackgroundGoroutine(func() {
		ticker := time.NewTicker(collectInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.collectVMMetrics(ctx)
			}
		}
	})
}

func (s *Server) collectVMMetrics(ctx context.Context) {
	domains, err := s.libvirtConn.ListAllDomains(0)
	if err != nil {
		s.logger.Warn("failed to list domains for metrics collection", "error", err)
		return
	}
	defer func() {
		for _, dom := range domains {
			if freeErr := dom.Free(); freeErr != nil {
				s.logger.Warn("error freeing domain resource", "error", freeErr)
			}
		}
	}()

	vmCount := len(domains)
	metrics.NodeVMCount.Set(float64(vmCount))

	for _, dom := range domains {
		name, err := dom.GetName()
		if err != nil {
			continue
		}

		vmID := strings.TrimPrefix(name, "vs-")
		if vmID == name {
			continue
		}

		state, _, err := dom.GetState()
		if err != nil {
			continue
		}

		statusStr := vmMapLibvirtState(state)
		metrics.VMStatus.WithLabelValues(vmID).Set(metrics.StatusToValue(statusStr))

		if state != libvirt.DOMAIN_RUNNING {
			continue
		}

		vmMetrics, err := s.vmManager.GetMetrics(ctx, vmID)
		if err != nil {
			continue
		}

		metrics.VMCPUUsagePercent.WithLabelValues(vmID).Set(vmMetrics.CPUUsagePercent)
		metrics.VMMemoryUsageBytes.WithLabelValues(vmID).Set(float64(vmMetrics.MemoryUsageBytes))
		metrics.VMMemoryLimitBytes.WithLabelValues(vmID).Set(float64(vmMetrics.MemoryTotalBytes))
		metrics.VMDiskReadBytesTotal.WithLabelValues(vmID).Set(float64(vmMetrics.DiskReadBytes))
		metrics.VMDiskWriteBytesTotal.WithLabelValues(vmID).Set(float64(vmMetrics.DiskWriteBytes))
		metrics.VMDiskReadOpsTotal.WithLabelValues(vmID).Set(float64(vmMetrics.DiskReadOps))
		metrics.VMDiskWriteOpsTotal.WithLabelValues(vmID).Set(float64(vmMetrics.DiskWriteOps))
		metrics.VMNetworkRXBytesTotal.WithLabelValues(vmID).Set(float64(vmMetrics.NetworkRXBytes))
		metrics.VMNetworkTXBytesTotal.WithLabelValues(vmID).Set(float64(vmMetrics.NetworkTXBytes))
		metrics.VMNetworkRXPacketsTotal.WithLabelValues(vmID).Set(float64(vmMetrics.NetworkRXPkts))
		metrics.VMNetworkTXPacketsTotal.WithLabelValues(vmID).Set(float64(vmMetrics.NetworkTXPkts))
		metrics.VMNetworkRXErrorsTotal.WithLabelValues(vmID).Set(float64(vmMetrics.NetworkRXErrs))
		metrics.VMNetworkTXErrorsTotal.WithLabelValues(vmID).Set(float64(vmMetrics.NetworkTXErrs))
		metrics.VMNetworkRXDropsTotal.WithLabelValues(vmID).Set(float64(vmMetrics.NetworkRXDrop))
		metrics.VMNetworkTXDropsTotal.WithLabelValues(vmID).Set(float64(vmMetrics.NetworkTXDrop))
	}
}

func (s *Server) startMetricsHTTPServer(ctx context.Context) {
	s.trackBackgroundGoroutine(func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())

		srv := &http.Server{
			Addr:    s.metricsAddr,
			Handler: mux,
		}

		go func() {
			<-ctx.Done()
			// Derive shutdown context from parent to maintain context chain.
			// Even though ctx is done, context.WithoutCancel preserves trace
			// correlation while allowing the shutdown to proceed.
			shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		}()

		s.logger.Info("starting metrics HTTP server", "address", s.metricsAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("metrics HTTP server error", "error", err)
		}
	})
}

func vmMapLibvirtState(state libvirt.DomainState) string {
	switch state {
	case libvirt.DOMAIN_NOSTATE:
		return "no_state"
	case libvirt.DOMAIN_RUNNING:
		return "running"
	case libvirt.DOMAIN_BLOCKED:
		return "blocked"
	case libvirt.DOMAIN_PAUSED:
		return "paused"
	case libvirt.DOMAIN_SHUTDOWN:
		return "shutting_down"
	case libvirt.DOMAIN_SHUTOFF:
		return "stopped"
	case libvirt.DOMAIN_CRASHED:
		return "crashed"
	case libvirt.DOMAIN_PMSUSPENDED:
		return "suspended"
	default:
		return "unknown"
	}
}

// closeStorage closes the storage backend if it has a Close method.
func (s *Server) closeStorage() {
	if s.storageBackend == nil {
		return
	}

	// RBDManager has a Close method, QCOWManager doesn't need one
	if closer, ok := s.storageBackend.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			s.logger.Error("error closing storage backend", "error", err)
		}
	}
}

// trackBackgroundGoroutine tracks a background goroutine for graceful shutdown.
func (s *Server) trackBackgroundGoroutine(fn func()) {
	s.bgWg.Add(1)
	go func() {
		defer s.bgWg.Done()
		fn()
	}()
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

// getStoragePoolStats returns the storage pool statistics.
func (s *Server) getStoragePoolStats(ctx context.Context) (totalGB, usedGB int64) {
	if s.storageBackend == nil {
		return 0, 0
	}
	stats, err := s.storageBackend.GetPoolStats(ctx)
	if err != nil {
		s.logger.Warn("could not get storage pool stats", "error", err, "backend", s.storageType)
		return 0, 0
	}
	gb := int64(1024 * 1024 * 1024)
	return stats.Total / gb, stats.Used / gb
}

// isStorageConnected returns true if the storage backend connection is healthy.
func (s *Server) isStorageConnected() bool {
	if s.storageBackend == nil {
		return false
	}
	if s.storageType == storage.StorageTypeCEPH {
		if rbdMgr, ok := s.storageBackend.(*storage.RBDManager); ok {
			return rbdMgr.IsConnected()
		}
	}
	return true
}

func (s *Server) isLibvirtAlive() bool {
	alive, err := s.libvirtConn.IsAlive()
	if err != nil {
		return false
	}
	return alive
}

// newBandwidthManager creates a new NodeBandwidthManager for bandwidth operations.
func (s *Server) newBandwidthManager() *network.NodeBandwidthManager {
	return network.NewNodeBandwidthManager(s.libvirtConn, s.logger)
}

// getVMTapInterface looks up the tap interface name for a VM from its domain XML.
func (s *Server) getVMTapInterface(ctx context.Context, vmID string) (string, error) {
	bwm := s.newBandwidthManager()
	domainName := vm.DomainNameFromID(vmID)
	return bwm.GetInterfaceName(ctx, domainName)
}

// getVNCPort extracts the VNC port from a running domain's XML.
func (s *Server) getVNCPort(domain *libvirt.Domain) (int32, error) {
	xmlDesc, err := domain.GetXMLDesc(0)
	if err != nil {
		return 0, fmt.Errorf("getting domain XML: %w", err)
	}

	type graphicsXML struct {
		Type string `xml:"type,attr"`
		Port string `xml:"port,attr"`
	}
	type devicesXML struct {
		Graphics []graphicsXML `xml:"graphics"`
	}
	type domainXML struct {
		Devices devicesXML `xml:"devices"`
	}

	var domDef domainXML
	if err := xml.Unmarshal([]byte(xmlDesc), &domDef); err != nil {
		return 0, fmt.Errorf("parsing domain XML: %w", err)
	}

	for _, gfx := range domDef.Devices.Graphics {
		if gfx.Type == "vnc" {
			port, err := strconv.ParseInt(gfx.Port, 10, 32)
			if err != nil {
				return 0, fmt.Errorf("parsing VNC port: %w", err)
			}
			return int32(port), nil
		}
	}
	return 0, fmt.Errorf("VNC graphics not found")
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
		CephConnected:    h.server.isStorageConnected(),
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

	totalDiskGB, usedDiskGB := h.server.getStoragePoolStats(ctx)

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

	// Validate resource fields (QG-05: input validation)
	if req.GetVcpu() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "vcpu must be positive")
	}
	if req.GetMemoryMb() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "memory_mb must be positive")
	}
	if req.GetDiskGb() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "disk_gb must be positive")
	}

	// Validate storage_backend if provided
	storageBackend := req.GetStorageBackend()
	if storageBackend != "" && storageBackend != vm.StorageBackendCeph && storageBackend != vm.StorageBackendQcow {
		return nil, status.Errorf(codes.InvalidArgument, "invalid storage_backend: %s (must be 'ceph' or 'qcow')", storageBackend)
	}

	logger := h.server.logger.With("vm_id", req.GetVmId(), "operation", "create")
	logger.Info("creating VM", "hostname", req.GetHostname(), "vcpu", req.GetVcpu(), "memory_mb", req.GetMemoryMb())

	if storageBackend == "" {
		storageBackend = h.server.config.StorageBackend
		if storageBackend == "" {
			storageBackend = vm.StorageBackendCeph
		}
	}

	cfg := &vm.DomainConfig{
		VMID:           req.GetVmId(),
		Hostname:       req.GetHostname(),
		VCPU:           int(req.GetVcpu()),
		MemoryMB:       int(req.GetMemoryMb()),
		StorageBackend: storageBackend,
		MACAddress:     req.GetMacAddress(),
		IPv4Address:    req.GetIpv4Address(),
		IPv6Address:    req.GetIpv6Address(),
		PortSpeedKbps:  int(req.GetPortSpeedMbps()) * 1000,
	}

	switch storageBackend {
	case vm.StorageBackendQcow:
		templatePath := req.GetTemplateFilePath()
		if templatePath == "" {
			return nil, status.Error(codes.InvalidArgument, "template_file_path is required for qcow storage backend")
		}
		if err := validatePath(templatePath, h.server.config.StoragePath); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid template_file_path: %v", err)
		}

		templateMgr, ok := h.server.templateMgr.(*storage.QCOWTemplateManager)
		if !ok {
			return nil, status.Error(codes.Internal, "QCOW template manager not available")
		}

		diskPath, err := templateMgr.CloneForVM(ctx, templatePath, req.GetVmId(), int(req.GetDiskGb()))
		if err != nil {
			return nil, status.Errorf(codes.Internal, "cloning QCOW template: %v", err)
		}
		cfg.DiskPath = diskPath
		logger.Info("cloned QCOW template", "template", templatePath, "disk_path", diskPath)

	case vm.StorageBackendCeph:
		cfg.CephPool = req.GetCephPool()
		cfg.CephMonitors = req.GetCephMonitors()
		cfg.CephUser = req.GetCephUser()
		cfg.CephSecretUUID = req.GetCephSecretUuid()

		if cfg.CephPool == "" {
			cfg.CephPool = h.server.config.CephPool
		}
		if cfg.CephUser == "" {
			cfg.CephUser = h.server.config.CephUser
		}

		if req.GetTemplateRbdImage() != "" && req.GetTemplateRbdSnapshot() != "" {
			diskName := fmt.Sprintf(storage.VMDiskNameFmt, req.GetVmId())
			if err := h.server.storageBackend.CloneFromTemplate(ctx, req.GetCephPool(), req.GetTemplateRbdImage(), req.GetTemplateRbdSnapshot(), diskName); err != nil {
				return nil, status.Errorf(codes.Internal, "cloning RBD template: %v", err)
			}
			logger.Info("cloned RBD template", "template", req.GetTemplateRbdImage(), "snapshot", req.GetTemplateRbdSnapshot())
		}

	default:
		return nil, status.Errorf(codes.InvalidArgument, "unsupported storage backend: %s", storageBackend)
	}

	result, err := h.server.vmManager.CreateVM(ctx, cfg)
	if err != nil {
		return nil, h.mapError(err, "creating VM")
	}

	// Apply abuse prevention nftables rules (block SMTP, block metadata endpoint)
	if tapIface, tapErr := h.server.getVMTapInterface(ctx, req.GetVmId()); tapErr != nil {
		logger.Warn("failed to get tap interface for abuse prevention", "error", tapErr)
	} else if tapErr := h.server.abusePreventionMgr.ApplyVMRules(ctx, tapIface); tapErr != nil {
		logger.Warn("failed to apply abuse prevention rules", "error", tapErr, "tap", tapIface)
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
func (h *grpcHandler) DeleteVM(ctx context.Context, req *nodeagentpb.DeleteVMRequest) (*nodeagentpb.VMOperationResponse, error) {
	if req.GetVmId() == "" {
		return nil, status.Error(codes.InvalidArgument, "vm_id is required")
	}

	logger := h.server.logger.With("vm_id", req.GetVmId(), "operation", "delete")
	logger.Info("deleting VM")

	// Remove abuse prevention nftables rules before deletion
	if tapIface, tapErr := h.server.getVMTapInterface(ctx, req.GetVmId()); tapErr != nil {
		logger.Warn("failed to get tap interface for abuse prevention cleanup", "error", tapErr)
	} else if tapErr := h.server.abusePreventionMgr.RemoveVMRules(ctx, tapIface); tapErr != nil {
		logger.Warn("failed to remove abuse prevention rules", "error", tapErr, "tap", tapIface)
	}

	if err := h.server.vmManager.DeleteVM(ctx, req.GetVmId()); err != nil {
		return nil, h.mapError(err, "deleting VM domain")
	}

	storageBackend := req.GetStorageBackend()
	if storageBackend == "" {
		storageBackend = h.server.config.StorageBackend
		if storageBackend == "" {
			storageBackend = vm.StorageBackendCeph
		}
	}

	switch storageBackend {
	case vm.StorageBackendQcow:
		diskPath := req.GetDiskPath()
		if diskPath == "" {
			diskPath = fmt.Sprintf("%s/vms/%s-disk0.qcow2", h.server.config.StoragePath, req.GetVmId())
		}
		if err := validatePath(diskPath, h.server.config.StoragePath); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid disk_path: %v", err)
		}
		if err := h.server.storageBackend.Delete(ctx, diskPath); err != nil {
			logger.Warn("failed to delete QCOW disk", "error", err, "path", diskPath)
		} else {
			logger.Info("QCOW disk deleted", "path", diskPath)
		}

	case vm.StorageBackendCeph:
		diskName := fmt.Sprintf(storage.VMDiskNameFmt, req.GetVmId())
		if err := h.server.storageBackend.Delete(ctx, diskName); err != nil {
			logger.Warn("failed to delete RBD disk", "error", err, "name", diskName)
		} else {
			logger.Info("RBD disk deleted", "name", diskName)
		}
	}

	return &nodeagentpb.VMOperationResponse{
		VmId:    req.GetVmId(),
		Success: true,
	}, nil
}

// mapError maps internal errors to safe gRPC status codes.
// The original error is logged server-side and only a generic message is
// returned to the caller to prevent leaking internal details such as file
// paths or stack traces.
func (h *grpcHandler) mapError(err error, operation string) error {
	h.server.logger.Error("gRPC handler error", "operation", operation, "error", err)
	return status.Errorf(codes.Internal, "%s failed", operation)
}

// validatePath checks that path is non-empty and, after cleaning, is located
// within allowedPrefix. This prevents path-traversal attacks (e.g. "../..") on
// disk or ISO paths that arrive from the controller over gRPC.
// allowedPrefix must end without a trailing slash.
func validatePath(path, allowedPrefix string) error {
	if path == "" {
		return fmt.Errorf("path must not be empty")
	}
	cleaned := filepath.Clean(path)
	if !strings.HasPrefix(cleaned, allowedPrefix+"/") && cleaned != allowedPrefix {
		return fmt.Errorf("path %q is outside the allowed directory %q", cleaned, allowedPrefix)
	}
	return nil
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
