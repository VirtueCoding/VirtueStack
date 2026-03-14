package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/tasks"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"google.golang.org/grpc"
)

// NodeAgentGRPCClient implements NodeAgentClient interface with gRPC communication
// and caching for metrics to reduce load on node agents.
type NodeAgentGRPCClient struct {
	nodeRepo *repository.NodeRepository
	connPool GRPCConnectionPool
	cache    *metricsCache
	logger   *slog.Logger
}

// GRPCConnectionPool defines the interface for managing gRPC connections to nodes.
type GRPCConnectionPool interface {
	GetConnection(ctx context.Context, nodeID, address string) (*grpc.ClientConn, error)
	ReleaseConnection(nodeID string, conn *grpc.ClientConn)
}

// metricsCache holds cached node metrics with expiration.
type metricsCache struct {
	data  map[string]*cachedMetrics
	mutex sync.RWMutex
	ttl   time.Duration
}

// cachedMetrics stores metrics with their expiration time.
type cachedMetrics struct {
	metrics   *models.NodeHeartbeat
	expiresAt time.Time
}

// NodeHealthResponse represents the health response from node agent.
// This mirrors the node agent's response structure.
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

// NodeResourcesResponse represents the resources response from node agent.
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

// NewNodeAgentGRPCClient creates a new NodeAgentGRPCClient with caching.
func NewNodeAgentGRPCClient(
	nodeRepo *repository.NodeRepository,
	connPool GRPCConnectionPool,
	logger *slog.Logger,
) *NodeAgentGRPCClient {
	return &NodeAgentGRPCClient{
		nodeRepo: nodeRepo,
		connPool: connPool,
		cache: &metricsCache{
			data: make(map[string]*cachedMetrics),
			ttl:  5 * time.Second, // 5-second cache as required
		},
		logger: logger.With("component", "node-agent-client"),
	}
}

// GetNodeMetrics retrieves real-time metrics from a node with 5-second caching.
func (c *NodeAgentGRPCClient) GetNodeMetrics(ctx context.Context, nodeID string) (*models.NodeHeartbeat, error) {
	// Check cache first
	if cached := c.cache.get(nodeID); cached != nil {
		c.logger.Debug("returning cached metrics", "node_id", nodeID)
		return cached, nil
	}

	// Get node details for connection
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	// Get connection to node agent
	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return nil, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	// Fetch metrics from node agent
	metrics, err := c.fetchMetricsFromNode(ctx, conn, nodeID)
	if err != nil {
		return nil, fmt.Errorf("fetching metrics from node %s: %w", nodeID, err)
	}

	// Cache the metrics
	c.cache.set(nodeID, metrics)

	return metrics, nil
}

// fetchMetricsFromNode makes the actual gRPC call to get node metrics.
func (c *NodeAgentGRPCClient) fetchMetricsFromNode(ctx context.Context, conn *grpc.ClientConn, nodeID string) (*models.NodeHeartbeat, error) {
	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.GetNodeHealth(ctx, &nodeagentpb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("calling GetNodeHealth: %w", err)
	}
	return convertProtoHealthToHeartbeat(resp), nil
}

// PingNode checks if a node is reachable via gRPC.
func (c *NodeAgentGRPCClient) PingNode(ctx context.Context, nodeID string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	_, err = client.Ping(ctx, &nodeagentpb.Empty{})
	if err != nil {
		return fmt.Errorf("pinging node %s: %w", nodeID, err)
	}
	return nil
}

// EvacuateNode evacuates all VMs from a node via gRPC.
func (c *NodeAgentGRPCClient) EvacuateNode(ctx context.Context, nodeID string) error {
	c.logger.Info("starting node evacuation", "node_id", nodeID)

	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	node.Status = "draining"
	if err := c.nodeRepo.UpdateStatus(ctx, nodeID, node.Status); err != nil {
		return fmt.Errorf("updating node status to draining: %w", err)
	}

	c.logger.Info("node marked for evacuation, VM migration should be initiated", "node_id", nodeID)
	return nil
}

// GetNodeResources retrieves aggregate resource information from a node.
func (c *NodeAgentGRPCClient) GetNodeResources(ctx context.Context, nodeID string) (*NodeResourcesResponse, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return nil, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.GetNodeResources(ctx, &nodeagentpb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("calling GetNodeResources: %w", err)
	}

	return &NodeResourcesResponse{
		TotalVCPU:     resp.GetTotalVcpu(),
		UsedVCPU:      resp.GetUsedVcpu(),
		TotalMemoryMB: resp.GetTotalMemoryMb(),
		UsedMemoryMB:  resp.GetUsedMemoryMb(),
		TotalDiskGB:   resp.GetTotalDiskGb(),
		UsedDiskGB:    resp.GetUsedDiskGb(),
		VMCount:       resp.GetVmCount(),
		LoadAverage:   resp.GetLoadAverage(),
		UptimeSeconds: resp.GetUptimeSeconds(),
	}, nil
}

// cache methods

func (mc *metricsCache) get(nodeID string) *models.NodeHeartbeat {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	cached, exists := mc.data[nodeID]
	if !exists {
		return nil
	}

	// Check if expired
	if time.Now().After(cached.expiresAt) {
		return nil
	}

	return cached.metrics
}

func (mc *metricsCache) set(nodeID string, metrics *models.NodeHeartbeat) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	mc.data[nodeID] = &cachedMetrics{
		metrics:   metrics,
		expiresAt: time.Now().Add(mc.ttl),
	}
}

// ClearCache clears the metrics cache for a specific node or all nodes if nodeID is empty.
func (c *NodeAgentGRPCClient) ClearCache(nodeID string) {
	c.cache.mutex.Lock()
	defer c.cache.mutex.Unlock()

	if nodeID == "" {
		// Clear all
		c.cache.data = make(map[string]*cachedMetrics)
	} else {
		delete(c.cache.data, nodeID)
	}
}

// convertHealthResponse converts NodeHealthResponse to NodeHeartbeat.
// Used when gRPC is fully implemented.
func convertHealthResponse(resp *NodeHealthResponse) *models.NodeHeartbeat {
	loadAvg := make([]float32, len(resp.LoadAverage))
	for i, v := range resp.LoadAverage {
		loadAvg[i] = float32(v)
	}

	return &models.NodeHeartbeat{
		CPUPercent:    float32(resp.CPUPercent),
		MemoryPercent: float32(resp.MemoryPercent),
		DiskPercent:   float32(resp.DiskPercent),
		CephConnected: resp.CephConnected,
		VMCount:       int(resp.VMCount),
		LoadAverage:   loadAvg,
	}
}

func (c *NodeAgentGRPCClient) StartVM(ctx context.Context, nodeID, vmID string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.StartVM(ctx, &nodeagentpb.VMIdentifier{VmId: vmID})
	if err != nil {
		return fmt.Errorf("calling StartVM: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to start VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) StopVM(ctx context.Context, nodeID, vmID string, timeoutSeconds int) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.StopVM(ctx, &nodeagentpb.StopVMRequest{
		VmId:           vmID,
		TimeoutSeconds: int32(timeoutSeconds),
	})
	if err != nil {
		return fmt.Errorf("calling StopVM: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to stop VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) ForceStopVM(ctx context.Context, nodeID, vmID string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.ForceStopVM(ctx, &nodeagentpb.VMIdentifier{VmId: vmID})
	if err != nil {
		return fmt.Errorf("calling ForceStopVM: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to force stop VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) DeleteVM(ctx context.Context, nodeID, vmID string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.DeleteVM(ctx, &nodeagentpb.VMIdentifier{VmId: vmID})
	if err != nil {
		return fmt.Errorf("calling DeleteVM: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to delete VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) ResizeVM(ctx context.Context, nodeID, vmID string, vcpu, memoryMB, diskGB int) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.ResizeVM(ctx, &nodeagentpb.ResizeVMRequest{
		VmId:        vmID,
		NewVcpu:     int32(vcpu),
		NewMemoryMb: int32(memoryMB),
		NewDiskGb:   int32(diskGB),
	})
	if err != nil {
		return fmt.Errorf("calling ResizeVM: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to resize VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) GetVMMetrics(ctx context.Context, nodeID, vmID string) (*models.VMMetrics, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return nil, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.GetVMMetrics(ctx, &nodeagentpb.VMIdentifier{VmId: vmID})
	if err != nil {
		return nil, fmt.Errorf("calling GetVMMetrics: %w", err)
	}

	return convertProtoMetrics(resp), nil
}

func (c *NodeAgentGRPCClient) GetVMStatus(ctx context.Context, nodeID, vmID string) (string, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return "", fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return "", fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.GetVMStatus(ctx, &nodeagentpb.VMIdentifier{VmId: vmID})
	if err != nil {
		return "", fmt.Errorf("calling GetVMStatus: %w", err)
	}

	return convertProtoStatus(resp.GetStatus()), nil
}

// MigrateVM initiates a live migration of a VM to a target node.
func (c *NodeAgentGRPCClient) MigrateVM(ctx context.Context, sourceNodeID, targetNodeID, vmID string, opts *tasks.MigrateVMOptions) error {
	// Get source node details for connection
	sourceNode, err := c.nodeRepo.GetByID(ctx, sourceNodeID)
	if err != nil {
		return fmt.Errorf("getting source node %s: %w", sourceNodeID, err)
	}

	// Get connection to source node
	conn, err := c.connPool.GetConnection(ctx, sourceNodeID, sourceNode.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to source node %s: %w", sourceNodeID, err)
	}
	defer c.connPool.ReleaseConnection(sourceNodeID, conn)

	// Get target node details for address
	targetNode, err := c.nodeRepo.GetByID(ctx, targetNodeID)
	if err != nil {
		return fmt.Errorf("getting target node %s: %w", targetNodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.MigrateVM(ctx, &nodeagentpb.MigrateVMRequest{
		VmId:                   vmID,
		DestinationNodeAddress: targetNode.GRPCAddress,
		Live:                   opts != nil && opts.TargetNodeAddress != "",
	})
	if err != nil {
		return fmt.Errorf("migrating VM: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("migration failed: %s", resp.GetErrorMessage())
	}
	return nil
}

// AbortMigration aborts an in-progress migration on the specified node.
func (c *NodeAgentGRPCClient) AbortMigration(ctx context.Context, nodeID, vmID string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}
	defer c.connPool.ReleaseConnection(nodeID, conn)

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.AbortMigration(ctx, &nodeagentpb.VMIdentifier{VmId: vmID})
	if err != nil {
		return fmt.Errorf("aborting migration: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("abort migration failed: %s", resp.GetErrorMessage())
	}
	return nil
}

// PostMigrateSetup re-applies tc throttling and nwfilter on the target node after migration.
func (c *NodeAgentGRPCClient) PostMigrateSetup(ctx context.Context, nodeID, vmID string, bandwidthMbps int) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}
	defer c.connPool.ReleaseConnection(nodeID, conn)

	client := nodeagentpb.NewNodeAgentServiceClient(conn)

	// Build bandwidth limits from bandwidthMbps
	var bandwidth *nodeagentpb.BandwidthLimits
	var isThrottled bool
	var throttleRateKbps int32

	if bandwidthMbps > 0 {
		isThrottled = true
		throttleRateKbps = int32(bandwidthMbps * 1000)
		bandwidth = &nodeagentpb.BandwidthLimits{
			AverageKbps: throttleRateKbps,
			PeakKbps:    throttleRateKbps,
			BurstKb:     throttleRateKbps,
		}
	}

	resp, err := client.PostMigrateSetup(ctx, &nodeagentpb.PostMigrateSetupRequest{
		VmUuid:           vmID,
		Bandwidth:        bandwidth,
		IsThrottled:      isThrottled,
		ThrottleRateKbps: throttleRateKbps,
	})
	if err != nil {
		return fmt.Errorf("post-migrate setup: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("post-migrate setup failed: %s", resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) CreateVM(ctx context.Context, nodeID string, req *tasks.CreateVMRequest) (*tasks.CreateVMResponse, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return nil, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.CreateVM(ctx, &nodeagentpb.CreateVMRequest{
		VmId:                req.VMID,
		Hostname:            req.Hostname,
		Vcpu:                int32(req.VCPU),
		MemoryMb:            int32(req.MemoryMB),
		DiskGb:              int32(req.DiskGB),
		TemplateRbdImage:    req.TemplateRBDImage,
		TemplateRbdSnapshot: req.TemplateRBDSnapshot,
		RootPasswordHash:    req.RootPasswordHash,
		SshPublicKeys:       req.SSHPublicKeys,
		Ipv4Address:         req.IPv4Address,
		Ipv4Gateway:         req.IPv4Gateway,
		Ipv6Address:         req.IPv6Address,
		Ipv6Gateway:         req.IPv6Gateway,
		MacAddress:          req.MACAddress,
		PortSpeedMbps:       int32(req.PortSpeedMbps),
		CephMonitors:        req.CephMonitors,
		CephUser:            req.CephUser,
		CephSecretUuid:      req.CephSecretUUID,
		CephPool:            req.CephPool,
		Nameservers:         req.Nameservers,
	})
	if err != nil {
		return nil, fmt.Errorf("calling CreateVM: %w", err)
	}

	return &tasks.CreateVMResponse{
		DomainName: resp.GetLibvirtDomainName(),
		VNCPort:    resp.GetVncPort(),
	}, nil
}

func (c *NodeAgentGRPCClient) CreateSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) (*tasks.SnapshotResponse, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return nil, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.CreateSnapshot(ctx, &nodeagentpb.SnapshotRequest{VmId: vmID, Name: snapshotName})
	if err != nil {
		return nil, fmt.Errorf("calling CreateSnapshot: %w", err)
	}

	return &tasks.SnapshotResponse{
		SnapshotID:      resp.GetSnapshotId(),
		RBDSnapshotName: resp.GetRbdSnapshotName(),
		SizeBytes:       resp.GetSizeBytes(),
	}, nil
}

func (c *NodeAgentGRPCClient) DeleteSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.DeleteSnapshot(ctx, &nodeagentpb.SnapshotIdentifier{VmId: vmID, SnapshotId: snapshotName})
	if err != nil {
		return fmt.Errorf("calling DeleteSnapshot: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to delete snapshot %s for VM %s: %s", snapshotName, vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) RestoreSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.RevertSnapshot(ctx, &nodeagentpb.SnapshotIdentifier{VmId: vmID, SnapshotId: snapshotName})
	if err != nil {
		return fmt.Errorf("calling RevertSnapshot: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to restore snapshot %s for VM %s: %s", snapshotName, vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) CloneFromBackup(ctx context.Context, nodeID, vmID, backupSnapshot string, diskGB int) error {
	return errors.New("CloneFromBackup is not supported by node agent gRPC API")
}

func (c *NodeAgentGRPCClient) DeleteDisk(ctx context.Context, nodeID, vmID string) error {
	return errors.New("DeleteDisk is not supported by node agent gRPC API")
}

func (c *NodeAgentGRPCClient) CloneFromTemplate(ctx context.Context, nodeID, vmID, templateImage, templateSnapshot string, diskGB int) error {
	return errors.New("CloneFromTemplate is not supported by node agent gRPC API")
}

func (c *NodeAgentGRPCClient) GenerateCloudInit(ctx context.Context, nodeID string, cfg *tasks.CloudInitConfig) (string, error) {
	return "", errors.New("GenerateCloudInit is not supported by node agent gRPC API")
}

func (c *NodeAgentGRPCClient) GuestFreezeFilesystems(ctx context.Context, nodeID, vmID string) (int, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return 0, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return 0, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.GuestFreezeFilesystems(ctx, &nodeagentpb.VMIdentifier{VmId: vmID})
	if err != nil {
		return 0, fmt.Errorf("calling GuestFreezeFilesystems: %w", err)
	}
	if !resp.GetSuccess() {
		return 0, fmt.Errorf("failed to freeze filesystems for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return 0, nil
}

func (c *NodeAgentGRPCClient) GuestThawFilesystems(ctx context.Context, nodeID, vmID string) (int, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return 0, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return 0, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.GuestThawFilesystems(ctx, &nodeagentpb.VMIdentifier{VmId: vmID})
	if err != nil {
		return 0, fmt.Errorf("calling GuestThawFilesystems: %w", err)
	}
	if !resp.GetSuccess() {
		return 0, fmt.Errorf("failed to thaw filesystems for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return 0, nil
}

func (c *NodeAgentGRPCClient) ProtectSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error {
	return errors.New("ProtectSnapshot is not supported by node agent gRPC API")
}

func (c *NodeAgentGRPCClient) UnprotectSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error {
	return errors.New("UnprotectSnapshot is not supported by node agent gRPC API")
}

func (c *NodeAgentGRPCClient) CloneSnapshot(ctx context.Context, nodeID, vmID, snapshotName, targetPool string) (string, error) {
	return "", errors.New("CloneSnapshot is not supported by node agent gRPC API")
}

func (c *NodeAgentGRPCClient) GetVMNodeID(ctx context.Context, vmID string) (string, error) {
	return "", errors.New("GetVMNodeID is not supported by node agent gRPC client")
}

func convertProtoHealthToHeartbeat(resp *nodeagentpb.NodeHealthResponse) *models.NodeHeartbeat {
	loadAvg := make([]float32, len(resp.GetLoadAverage()))
	for i, v := range resp.GetLoadAverage() {
		loadAvg[i] = float32(v)
	}

	return &models.NodeHeartbeat{
		CPUPercent:    float32(resp.GetCpuPercent()),
		MemoryPercent: float32(resp.GetMemoryPercent()),
		DiskPercent:   float32(resp.GetDiskPercent()),
		CephConnected: resp.GetCephConnected(),
		VMCount:       int(resp.GetVmCount()),
		LoadAverage:   loadAvg,
	}
}

func convertProtoMetrics(resp *nodeagentpb.VMMetricsResponse) *models.VMMetrics {
	var timestamp time.Time
	if resp.GetTimestamp() != nil {
		timestamp = resp.GetTimestamp().AsTime()
	}

	return &models.VMMetrics{
		VMID:             resp.GetVmId(),
		CPUUsagePercent:  resp.GetCpuUsagePercent(),
		MemoryUsageBytes: resp.GetMemoryUsageBytes(),
		MemoryTotalBytes: resp.GetMemoryTotalBytes(),
		DiskReadBytes:    resp.GetDiskReadBytes(),
		DiskWriteBytes:   resp.GetDiskWriteBytes(),
		NetworkRxBytes:   resp.GetNetworkRxBytes(),
		NetworkTxBytes:   resp.GetNetworkTxBytes(),
		Timestamp:        timestamp,
	}
}

func convertProtoStatus(status nodeagentpb.VMStatus) string {
	switch status {
	case nodeagentpb.VMStatus_VM_STATUS_RUNNING:
		return models.VMStatusRunning
	case nodeagentpb.VMStatus_VM_STATUS_STOPPED:
		return models.VMStatusStopped
	case nodeagentpb.VMStatus_VM_STATUS_PAUSED:
		return models.VMStatusSuspended
	case nodeagentpb.VMStatus_VM_STATUS_SHUTTING_DOWN:
		return models.VMStatusRunning
	case nodeagentpb.VMStatus_VM_STATUS_CRASHED:
		return models.VMStatusError
	case nodeagentpb.VMStatus_VM_STATUS_MIGRATING:
		return models.VMStatusMigrating
	default:
		return models.VMStatusError
	}
}
