package services

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
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
	NodeID           string
	Healthy          bool
	CPUPercent       float64
	MemoryPercent    float64
	DiskPercent      float64
	VMCount          int32
	LoadAverage      []float64
	UptimeSeconds    int64
	LibvirtConnected bool
	StorageConnected bool
	StorageTotalGB   int64
	StorageUsedGB    int64
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

// CreateQCOWBackup creates an internal QCOW disk snapshot via the node agent.
// NOTE: The node-agent gRPC API (CreateDiskSnapshot) does not yet expose
// compression or an explicit backup destination path — the `compress` and
// `backupPath` parameters are accepted to satisfy the BackupNodeAgentClient
// interface but are not forwarded to the remote call. Compression and
// destination-path support require a future gRPC API extension.

// RestoreQCOWBackup restores a VM from a QCOW backup by preparing the node
// agent with the source backup file (backupPath). The gRPC PrepareMigratedVM
// call uses backupPath as the disk source; the `targetPath` parameter is
// accepted to satisfy the BackupNodeAgentClient interface but is not yet
// forwarded — the node agent derives the final disk placement from its local
// storage configuration. Full targetPath support requires a future gRPC API
// extension.

// RestoreLVMBackup restores an LVM thin LV from a backup file.
// The thin LV must already exist; this uses dd to overwrite it in-place.
// The VM must be stopped before calling this method.

// CreateDiskSnapshot creates a disk snapshot for migration purposes.
// The storageBackend parameter determines the snapshot type (qcow internal snapshot, rbd snapshot, or lvm thin snapshot).

// DeleteDiskSnapshot removes a disk snapshot created for migration.

// CreateLVMBackup creates a sparse backup file from an LVM thin snapshot.
// The node agent uses dd with conv=sparse to efficiently copy the snapshot
// to a backup file, skipping zero blocks to minimize backup size.

// DeleteLVMBackupFile deletes an LVM backup file from the backup storage.

// BuildTemplateFromISO builds a VM template from an ISO on the specified node.

// EnsureTemplateCached ensures a template image is available locally on a QCOW/LVM node.

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
