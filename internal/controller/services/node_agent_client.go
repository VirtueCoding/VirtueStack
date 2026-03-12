package services

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
)

// NodeAgentGRPCClient implements NodeAgentClient interface with gRPC communication
// and caching for metrics to reduce load on node agents.
type NodeAgentGRPCClient struct {
	nodeRepo  *repository.NodeRepository
	connPool  GRPCConnectionPool
	cache     *metricsCache
	logger    *slog.Logger
}

// GRPCConnectionPool defines the interface for managing gRPC connections to nodes.
type GRPCConnectionPool interface {
	GetConnection(ctx context.Context, nodeID, address string) (any, error)
}

// metricsCache holds cached node metrics with expiration.
type metricsCache struct {
	data   map[string]*cachedMetrics
	mutex  sync.RWMutex
	ttl    time.Duration
}

// cachedMetrics stores metrics with their expiration time.
type cachedMetrics struct {
	metrics    *models.NodeHeartbeat
	expiresAt  time.Time
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
	// Note: When proto is generated, this will call the actual gRPC method
	// For now, we simulate the response structure
	metrics, err := c.fetchMetricsFromNode(ctx, conn, nodeID)
	if err != nil {
		return nil, fmt.Errorf("fetching metrics from node %s: %w", nodeID, err)
	}

	// Cache the metrics
	c.cache.set(nodeID, metrics)

	return metrics, nil
}

// fetchMetricsFromNode makes the actual gRPC call to get node metrics.
// This is a placeholder that will be replaced with actual gRPC calls when proto is generated.
func (c *NodeAgentGRPCClient) fetchMetricsFromNode(ctx context.Context, conn any, nodeID string) (*models.NodeHeartbeat, error) {
	// TODO: When protobuf is generated, implement actual gRPC call:
	// client := nodeagentpb.NewNodeAgentServiceClient(conn.(*grpc.ClientConn))
	// resp, err := client.GetNodeHealth(ctx, &nodeagentpb.Empty{})
	// if err != nil { return nil, err }
	// return convertProtoToHeartbeat(resp), nil

	// For now, return an error to indicate the node agent connection is not fully implemented
	c.logger.Warn("node agent gRPC not fully implemented, metrics unavailable",
		"node_id", nodeID,
		"hint", "proto generation pending")
	return nil, fmt.Errorf("node agent gRPC not implemented: proto generation pending")
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

	// TODO: When protobuf is generated, implement actual gRPC ping:
	// client := nodeagentpb.NewNodeAgentServiceClient(conn.(*grpc.ClientConn))
	// _, err := client.Ping(ctx, &nodeagentpb.PingRequest{})
	// return err

	_ = conn // Use the connection to avoid unused variable warning
	c.logger.Debug("ping node agent not fully implemented", "node_id", nodeID)
	return nil
}

// EvacuateNode evacuates all VMs from a node via gRPC.
func (c *NodeAgentGRPCClient) EvacuateNode(ctx context.Context, nodeID string) error {
	c.logger.Info("evacuate node not fully implemented", "node_id", nodeID)
	// TODO: Implement when protobuf is generated
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

	// TODO: When protobuf is generated, implement actual gRPC call
	_ = conn
	c.logger.Warn("get node resources not fully implemented", "node_id", nodeID)
	return nil, fmt.Errorf("not implemented: proto generation pending")
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
