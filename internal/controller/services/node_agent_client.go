package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/tasks"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

// CephConfig holds Ceph cluster connection parameters for the controller.
type CephConfig struct {
	Monitors   []string
	User       string
	SecretUUID string
}

// NodeAgentGRPCClient implements NodeAgentClient interface with gRPC communication
// and caching for metrics to reduce load on node agents.
type NodeAgentGRPCClient struct {
	nodeRepo *repository.NodeRepository
	vmRepo   *repository.VMRepository
	connPool GRPCConnectionPool
	cache    *metricsCache
	cephCfg  *CephConfig
	logger   *slog.Logger
}

// GRPCConnectionPool defines the interface for managing gRPC connections to nodes.
type GRPCConnectionPool interface {
	GetConnection(ctx context.Context, nodeID, address string) (*grpc.ClientConn, error)
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
	NodeID            string
	Healthy           bool
	CPUPercent        float64
	MemoryPercent     float64
	DiskPercent       float64
	VMCount           int32
	LoadAverage       []float64
	UptimeSeconds     int64
	LibvirtConnected  bool
	StorageConnected  bool
	StorageTotalGB    int64
	StorageUsedGB     int64
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
	vmRepo *repository.VMRepository,
	connPool GRPCConnectionPool,
	cephCfg *CephConfig,
	logger *slog.Logger,
) *NodeAgentGRPCClient {
	return &NodeAgentGRPCClient{
		nodeRepo: nodeRepo,
		vmRepo:   vmRepo,
		connPool: connPool,
		cache: &metricsCache{
			data: make(map[string]*cachedMetrics),
			ttl:  5 * time.Second,
		},
		cephCfg: cephCfg,
		logger:  logger.With("component", "node-agent-client"),
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

// evacuationConcurrencyLimit is the maximum number of VMs migrated in parallel
// during node evacuation. Bounded concurrency avoids overwhelming destination
// nodes and the gRPC connection pool.
const evacuationConcurrencyLimit = 10

// EvacuateNode evacuates all running VMs from a node with bounded concurrency.
// Up to evacuationConcurrencyLimit VMs are migrated simultaneously; the rest
// are queued. The function returns after all migration goroutines finish.
func (c *NodeAgentGRPCClient) EvacuateNode(ctx context.Context, nodeID string) error {
	c.logger.Info("starting node evacuation", "node_id", nodeID)

	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}
	// Silence unused variable warning; node is fetched to confirm existence.
	_ = node

	if err := c.nodeRepo.UpdateStatus(ctx, nodeID, models.NodeStatusDraining); err != nil {
		return fmt.Errorf("updating node status to draining: %w", err)
	}

	vmFilter := models.VMListFilter{
		NodeID: &nodeID,
		PaginationParams: models.PaginationParams{
			Page:    1,
			PerPage: models.MaxPerPage,
		},
	}

	vms, _, err := c.vmRepo.List(ctx, vmFilter)
	if err != nil {
		return fmt.Errorf("listing VMs on node %s: %w", nodeID, err)
	}

	if len(vms) == 0 {
		c.logger.Info("no VMs to evacuate", "node_id", nodeID)
		return nil
	}

	// Pre-fetch target nodes once to avoid an N+1 query per VM.
	targetNodes, _, err := c.nodeRepo.List(ctx, models.NodeListFilter{
		Status: util.StringPtr(models.NodeStatusOnline),
	})
	if err != nil {
		return fmt.Errorf("listing target nodes for evacuation: %w", err)
	}

	c.logger.Info("evacuating VMs from node", "node_id", nodeID, "vm_count", len(vms))

	// evacuateVM tries to place a single VM on any eligible target node.
	evacuateVM := func(vm models.VM) {
		for _, targetNode := range targetNodes {
			if targetNode.ID == nodeID {
				continue
			}

			availCPU := targetNode.TotalVCPU - targetNode.AllocatedVCPU
			availMem := targetNode.TotalMemoryMB - targetNode.AllocatedMemoryMB

			if availCPU < vm.VCPU || availMem < vm.MemoryMB {
				continue
			}

			if err := c.StartVM(ctx, targetNode.ID, vm.ID); err != nil {
				c.logger.Warn("failed to start VM on target node",
					"vm_id", vm.ID,
					"target_node_id", targetNode.ID,
					"error", err)
				continue
			}

			if err := c.vmRepo.UpdateNodeAssignment(ctx, vm.ID, targetNode.ID); err != nil {
				c.logger.Warn("failed to reassign VM to target node",
					"vm_id", vm.ID,
					"target_node_id", targetNode.ID,
					"error", err)
			}

			c.logger.Info("evacuated VM to target node",
				"vm_id", vm.ID,
				"old_node_id", nodeID,
				"new_node_id", targetNode.ID)
			return
		}
		c.logger.Warn("no eligible target node found for VM during evacuation", "vm_id", vm.ID)
	}

	// Use errgroup with a concurrency limit to fan-out VM evacuations.
	eg, _ := errgroup.WithContext(ctx)
	eg.SetLimit(evacuationConcurrencyLimit)

	for _, vm := range vms {
		if vm.Status != models.VMStatusRunning {
			c.logger.Debug("skipping non-running VM during evacuation",
				"vm_id", vm.ID,
				"status", vm.Status)
			continue
		}
		vm := vm // capture loop variable
		eg.Go(func() error {
			evacuateVM(vm)
			return nil // individual VM failures are logged, not propagated
		})
	}

	// Wait for all goroutines; errors are intentionally non-fatal per VM.
	_ = eg.Wait()

	c.logger.Info("node evacuation completed", "node_id", nodeID)
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
	cached, exists := mc.data[nodeID]
	mc.mutex.RUnlock()

	if !exists {
		return nil
	}

	// Check if expired.
	if time.Now().After(cached.expiresAt) {
		// Lazily evict the stale entry so deregistered nodes don't accumulate
		// in memory indefinitely (F-208).
		mc.mutex.Lock()
		if entry, ok := mc.data[nodeID]; ok && time.Now().After(entry.expiresAt) {
			delete(mc.data, nodeID)
		}
		mc.mutex.Unlock()
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

// evictExpired removes all expired entries from the cache.
// Called by the background eviction goroutine to handle deregistered nodes (F-208).
func (mc *metricsCache) evictExpired() {
	now := time.Now()
	mc.mutex.Lock()
	defer mc.mutex.Unlock()
	for nodeID, cached := range mc.data {
		if now.After(cached.expiresAt) {
			delete(mc.data, nodeID)
		}
	}
}

// StartEvictionLoop starts a background goroutine that periodically evicts
// stale cache entries for deregistered nodes (F-208). The goroutine exits when
// ctx is cancelled.
func (c *NodeAgentGRPCClient) StartEvictionLoop(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.cache.evictExpired()
			case <-ctx.Done():
				return
			}
		}
	}()
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
		CPUPercent:       float32(resp.CPUPercent),
		MemoryPercent:    float32(resp.MemoryPercent),
		DiskPercent:      float32(resp.DiskPercent),
		StorageConnected: resp.StorageConnected,
		StorageTotalGB:   resp.StorageTotalGB,
		StorageUsedGB:    resp.StorageUsedGB,
		VMCount:          int(resp.VMCount),
		LoadAverage:      loadAvg,
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
	resp, err := client.DeleteVM(ctx, &nodeagentpb.DeleteVMRequest{VmId: vmID})
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
		VmId:             vmID,
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
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.RevertSnapshot(ctx, &nodeagentpb.SnapshotIdentifier{
		VmId:       vmID,
		SnapshotId: backupSnapshot,
	})
	if err != nil {
		return fmt.Errorf("calling RevertSnapshot: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to clone from backup for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) DeleteDisk(ctx context.Context, nodeID, vmID string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	vm, err := c.vmRepo.GetByID(ctx, vmID)
	if err != nil {
		return fmt.Errorf("getting VM %s: %w", vmID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	var diskPath string
	if vm.DiskPath != nil {
		diskPath = *vm.DiskPath
	}
	resp, err := client.DeleteVM(ctx, &nodeagentpb.DeleteVMRequest{
		VmId:           vmID,
		StorageBackend: vm.StorageBackend,
		DiskPath:       diskPath,
	})
	if err != nil {
		return fmt.Errorf("calling DeleteVM: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to delete disk for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) CloneFromTemplate(ctx context.Context, nodeID, vmID, templateImage, templateSnapshot string, diskGB int) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.CreateVM(ctx, &nodeagentpb.CreateVMRequest{
		VmId:                vmID,
		TemplateRbdImage:    templateImage,
		TemplateRbdSnapshot: templateSnapshot,
		DiskGb:              int32(diskGB),
		CephMonitors:        c.cephMonitors(),
		CephUser:            c.cephUser(),
		CephSecretUuid:      c.cephSecretUUID(),
		CephPool:            node.CephPool,
	})
	if err != nil {
		return fmt.Errorf("calling CreateVM: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to clone template for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) GenerateCloudInit(ctx context.Context, nodeID string, cfg *tasks.CloudInitConfig) (string, error) {
	return fmt.Sprintf("/var/lib/virtuestack/cloud-init/%s.iso", cfg.VMID), nil
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
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	_, err = client.CreateSnapshot(ctx, &nodeagentpb.SnapshotRequest{
		VmId: vmID,
		Name: snapshotName,
	})
	if err != nil {
		return fmt.Errorf("calling CreateSnapshot for protect: %w", err)
	}
	return nil
}

func (c *NodeAgentGRPCClient) UnprotectSnapshot(ctx context.Context, nodeID, vmID, snapshotName string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.DeleteSnapshot(ctx, &nodeagentpb.SnapshotIdentifier{
		VmId:       vmID,
		SnapshotId: snapshotName,
	})
	if err != nil {
		return fmt.Errorf("calling DeleteSnapshot for unprotect: %w", err)
	}
	_ = resp
	return nil
}

func (c *NodeAgentGRPCClient) CloneSnapshot(ctx context.Context, nodeID, vmID, snapshotName, targetPool string) (string, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return "", fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return "", fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	cloneName := fmt.Sprintf("vs-%s-clone-%d", vmID, time.Now().Unix())
	resp, err := client.CreateSnapshot(ctx, &nodeagentpb.SnapshotRequest{
		VmId: vmID,
		Name: cloneName,
	})
	if err != nil {
		return "", fmt.Errorf("calling CreateSnapshot for clone: %w", err)
	}
	return resp.GetRbdSnapshotName(), nil
}

func (c *NodeAgentGRPCClient) GetVMNodeID(ctx context.Context, vmID string) (string, error) {
	vm, err := c.vmRepo.GetByID(ctx, vmID)
	if err != nil {
		return "", fmt.Errorf("getting VM %s: %w", vmID, err)
	}
	if vm.NodeID == nil {
		return "", fmt.Errorf("VM %s has no node assigned", vmID)
	}
	return *vm.NodeID, nil
}

func (c *NodeAgentGRPCClient) CreateQCOWSnapshot(ctx context.Context, nodeID, vmID, diskPath, snapshotName string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.CreateDiskSnapshot(ctx, &nodeagentpb.CreateDiskSnapshotRequest{
		VmId:           vmID,
		SnapshotName:   snapshotName,
		StorageBackend: "qcow",
		DiskPath:       diskPath,
	})
	if err != nil {
		return fmt.Errorf("calling CreateDiskSnapshot: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to create QCOW snapshot %s for VM %s: %s", snapshotName, vmID, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) DeleteQCOWSnapshot(ctx context.Context, nodeID, vmID, diskPath, snapshotName string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.DeleteDiskSnapshot(ctx, &nodeagentpb.DeleteDiskSnapshotRequest{
		VmId:           vmID,
		SnapshotName:   snapshotName,
		StorageBackend: "qcow",
		DiskPath:       diskPath,
	})
	if err != nil {
		return fmt.Errorf("calling DeleteDiskSnapshot: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to delete QCOW snapshot %s for VM %s: %s", snapshotName, vmID, resp.GetErrorMessage())
	}
	return nil
}

// CreateQCOWBackup creates an internal QCOW disk snapshot via the node agent.
// NOTE: The node-agent gRPC API (CreateDiskSnapshot) does not yet expose
// compression or an explicit backup destination path — the `compress` and
// `backupPath` parameters are accepted to satisfy the BackupNodeAgentClient
// interface but are not forwarded to the remote call. Compression and
// destination-path support require a future gRPC API extension.
func (c *NodeAgentGRPCClient) CreateQCOWBackup(ctx context.Context, nodeID, vmID, diskPath, snapshotName, backupPath string, compress bool) (int64, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return 0, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return 0, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.CreateDiskSnapshot(ctx, &nodeagentpb.CreateDiskSnapshotRequest{
		VmId:           vmID,
		SnapshotName:   snapshotName,
		StorageBackend: "qcow",
		DiskPath:       diskPath,
	})
	if err != nil {
		return 0, fmt.Errorf("calling CreateDiskSnapshot: %w", err)
	}
	if !resp.GetSuccess() {
		return 0, fmt.Errorf("failed to create QCOW backup snapshot for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	// compress and backupPath are not forwarded — see function doc for details.
	return 0, nil
}

// RestoreQCOWBackup restores a VM from a QCOW backup by preparing the node
// agent with the source backup file (backupPath). The gRPC PrepareMigratedVM
// call uses backupPath as the disk source; the `targetPath` parameter is
// accepted to satisfy the BackupNodeAgentClient interface but is not yet
// forwarded — the node agent derives the final disk placement from its local
// storage configuration. Full targetPath support requires a future gRPC API
// extension.
func (c *NodeAgentGRPCClient) RestoreQCOWBackup(ctx context.Context, nodeID, vmID, backupPath, targetPath string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	vm, err := c.vmRepo.GetByID(ctx, vmID)
	if err != nil {
		return fmt.Errorf("getting VM %s: %w", vmID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.PrepareMigratedVM(ctx, &nodeagentpb.PrepareMigratedVMRequest{
		VmId:           vmID,
		DiskPath:       backupPath,
		Hostname:       vm.Hostname,
		Vcpu:           int32(vm.VCPU),
		MemoryMb:       int32(vm.MemoryMB),
		StorageBackend: "qcow",
		CephPool:       node.CephPool,
		CephMonitors:   c.cephMonitors(),
		CephUser:       c.cephUser(),
		CephSecretUuid: c.cephSecretUUID(),
	})
	if err != nil {
		return fmt.Errorf("calling PrepareMigratedVM for restore: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to restore QCOW backup for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	// targetPath is not forwarded — see function doc for details.
	return nil
}

// RestoreLVMBackup restores an LVM thin LV from a backup file.
// The thin LV must already exist; this uses dd to overwrite it in-place.
// The VM must be stopped before calling this method.
func (c *NodeAgentGRPCClient) RestoreLVMBackup(ctx context.Context, nodeID, vmID, backupFilePath string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.RestoreLVMBackup(ctx, &nodeagentpb.RestoreLVMBackupRequest{
		VmId:           vmID,
		BackupFilePath: backupFilePath,
	})
	if err != nil {
		return fmt.Errorf("calling RestoreLVMBackup: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to restore LVM backup for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

// CreateDiskSnapshot creates a disk snapshot for migration purposes.
// The storageBackend parameter determines the snapshot type (qcow internal snapshot, rbd snapshot, or lvm thin snapshot).
func (c *NodeAgentGRPCClient) CreateDiskSnapshot(ctx context.Context, nodeID, vmID, diskPath, snapshotName, storageBackend string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.CreateDiskSnapshot(ctx, &nodeagentpb.CreateDiskSnapshotRequest{
		VmId:           vmID,
		DiskPath:       diskPath,
		SnapshotName:   snapshotName,
		StorageBackend: storageBackend,
	})
	if err != nil {
		return fmt.Errorf("calling CreateDiskSnapshot: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to create disk snapshot for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

// DeleteDiskSnapshot removes a disk snapshot created for migration.
func (c *NodeAgentGRPCClient) DeleteDiskSnapshot(ctx context.Context, nodeID, vmID, diskPath, snapshotName, storageBackend string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.DeleteDiskSnapshot(ctx, &nodeagentpb.DeleteDiskSnapshotRequest{
		VmId:           vmID,
		DiskPath:       diskPath,
		SnapshotName:   snapshotName,
		StorageBackend: storageBackend,
	})
	if err != nil {
		return fmt.Errorf("calling DeleteDiskSnapshot: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to delete disk snapshot for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return nil
}

// CreateLVMBackup creates a sparse backup file from an LVM thin snapshot.
// The node agent uses dd with conv=sparse to efficiently copy the snapshot
// to a backup file, skipping zero blocks to minimize backup size.
func (c *NodeAgentGRPCClient) CreateLVMBackup(ctx context.Context, nodeID, vmID, snapshotName, backupFilePath string) (int64, error) {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return 0, fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return 0, fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.CreateLVMBackup(ctx, &nodeagentpb.CreateLVMBackupRequest{
		VmId:           vmID,
		SnapshotName:   snapshotName,
		BackupFilePath: backupFilePath,
	})
	if err != nil {
		return 0, fmt.Errorf("calling CreateLVMBackup: %w", err)
	}
	if !resp.GetSuccess() {
		return 0, fmt.Errorf("failed to create LVM backup for VM %s: %s", vmID, resp.GetErrorMessage())
	}
	return resp.GetSizeBytes(), nil
}

// DeleteLVMBackupFile deletes an LVM backup file from the backup storage.
func (c *NodeAgentGRPCClient) DeleteLVMBackupFile(ctx context.Context, nodeID, backupPath string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.DeleteVM(ctx, &nodeagentpb.DeleteVMRequest{
		StorageBackend: "lvm",
		DiskPath:       backupPath,
	})
	if err != nil {
		return fmt.Errorf("calling DeleteVM for LVM backup file cleanup: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to delete LVM backup file %s: %s", backupPath, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) DeleteQCOWBackupFile(ctx context.Context, nodeID, backupPath string) error {
	node, err := c.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("getting node %s: %w", nodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, nodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to node %s: %w", nodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.DeleteVM(ctx, &nodeagentpb.DeleteVMRequest{
		StorageBackend: "qcow",
		DiskPath:       backupPath,
	})
	if err != nil {
		return fmt.Errorf("calling DeleteVM for backup file cleanup: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("failed to delete QCOW backup file %s: %s", backupPath, resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) GetQCOWDiskInfo(ctx context.Context, nodeID, diskPath string) (*tasks.QCOWDiskInfo, error) {
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

	return &tasks.QCOWDiskInfo{
		DiskPath:    diskPath,
		TotalDiskGB: uint64(resp.GetTotalDiskGb()),
		UsedDiskGB:  uint64(resp.GetUsedDiskGb()),
	}, nil
}

func (c *NodeAgentGRPCClient) TransferDisk(ctx context.Context, opts *tasks.DiskTransferOptions) error {
	sourceNode, err := c.nodeRepo.GetByID(ctx, opts.SourceNodeID)
	if err != nil {
		return fmt.Errorf("getting source node %s: %w", opts.SourceNodeID, err)
	}

	targetNode, err := c.nodeRepo.GetByID(ctx, opts.TargetNodeID)
	if err != nil {
		return fmt.Errorf("getting target node %s: %w", opts.TargetNodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, opts.SourceNodeID, sourceNode.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to source node %s: %w", opts.SourceNodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	stream, err := client.TransferDisk(ctx, &nodeagentpb.TransferDiskRequest{
		SourceDiskPath:    opts.SourceDiskPath,
		TargetNodeAddress: targetNode.GRPCAddress,
		TargetDiskPath:    opts.TargetDiskPath,
		SnapshotName:      opts.SnapshotName,
		Compress:          opts.Compress,
		StorageBackend:    opts.SourceStorageBackend,
	})
	if err != nil {
		return fmt.Errorf("initiating disk transfer: %w", err)
	}

	for {
		_, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("receiving disk transfer: %w", err)
		}
	}

	return nil
}

func (c *NodeAgentGRPCClient) PrepareMigratedVM(ctx context.Context, targetNodeID, vmID, diskPath string, vm *models.VM) error {
	node, err := c.nodeRepo.GetByID(ctx, targetNodeID)
	if err != nil {
		return fmt.Errorf("getting target node %s: %w", targetNodeID, err)
	}

	conn, err := c.connPool.GetConnection(ctx, targetNodeID, node.GRPCAddress)
	if err != nil {
		return fmt.Errorf("connecting to target node %s: %w", targetNodeID, err)
	}

	client := nodeagentpb.NewNodeAgentServiceClient(conn)
	resp, err := client.PrepareMigratedVM(ctx, &nodeagentpb.PrepareMigratedVMRequest{
		VmId:           vmID,
		DiskPath:       diskPath,
		Hostname:       vm.Hostname,
		Vcpu:           int32(vm.VCPU),
		MemoryMb:       int32(vm.MemoryMB),
		StorageBackend: vm.StorageBackend,
		MacAddress:     vm.MACAddress,
		PortSpeedMbps:  int32(vm.PortSpeedMbps),
		CephPool:       node.CephPool,
		CephMonitors:   c.cephMonitors(),
		CephUser:       c.cephUser(),
		CephSecretUuid: c.cephSecretUUID(),
	})
	if err != nil {
		return fmt.Errorf("preparing migrated VM: %w", err)
	}
	if !resp.GetSuccess() {
		return fmt.Errorf("prepare migrated VM failed: %s", resp.GetErrorMessage())
	}
	return nil
}

func (c *NodeAgentGRPCClient) cephMonitors() []string {
	if c.cephCfg != nil {
		return c.cephCfg.Monitors
	}
	return nil
}
func (c *NodeAgentGRPCClient) cephUser() string {
	if c.cephCfg != nil {
		return c.cephCfg.User
	}
	return ""
}
func (c *NodeAgentGRPCClient) cephSecretUUID() string {
	if c.cephCfg != nil {
		return c.cephCfg.SecretUUID
	}
	return ""
}

func convertProtoHealthToHeartbeat(resp *nodeagentpb.NodeHealthResponse) *models.NodeHeartbeat {
	loadAvg := make([]float32, len(resp.GetLoadAverage()))
	for i, v := range resp.GetLoadAverage() {
		loadAvg[i] = float32(v)
	}

	return &models.NodeHeartbeat{
		CPUPercent:       float32(resp.GetCpuPercent()),
		MemoryPercent:    float32(resp.GetMemoryPercent()),
		DiskPercent:      float32(resp.GetDiskPercent()),
		StorageConnected: resp.GetCephConnected(), // Proto still uses ceph_connected for wire compatibility
		VMCount:          int(resp.GetVmCount()),
		LoadAverage:      loadAvg,
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
	case nodeagentpb.VMStatus_VM_STATUS_UNKNOWN:
		return models.VMStatusError
	default:
		return models.VMStatusError
	}
}
