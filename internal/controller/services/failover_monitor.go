// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
)

// DefaultFailover configuration values.
const (
	DefaultFailoverCheckInterval = 30 * time.Second
	DefaultFailoverThreshold     = 3
)

// FailoverMonitorConfig holds configuration for the failover monitor.
type FailoverMonitorConfig struct {
	// CheckInterval is how often to check for failed nodes.
	CheckInterval time.Duration
	// Threshold is the number of consecutive heartbeat misses before triggering failover.
	Threshold int
	// Enabled controls whether auto-failover is active.
	Enabled bool
}

// DefaultFailoverMonitorConfig returns a FailoverMonitorConfig with sensible defaults.
func DefaultFailoverMonitorConfig() FailoverMonitorConfig {
	return FailoverMonitorConfig{
		CheckInterval: DefaultFailoverCheckInterval,
		Threshold:     DefaultFailoverThreshold,
		Enabled:       true,
	}
}

// FailoverMonitor is a background service that monitors node heartbeats
// and automatically triggers failover when consecutive_heartbeat_misses exceeds threshold.
type FailoverMonitor struct {
	nodeRepo       *repository.NodeRepository
	failoverSvc    *FailoverService
	logger         *slog.Logger
	config         FailoverMonitorConfig
	mu             sync.Mutex
	failoverCount  int64
	lastFailoverAt *time.Time
	recoveryTimeMs int64
}

// NewFailoverMonitor creates a new FailoverMonitor with the given dependencies.
func NewFailoverMonitor(
	nodeRepo *repository.NodeRepository,
	failoverSvc *FailoverService,
	logger *slog.Logger,
	config FailoverMonitorConfig,
) *FailoverMonitor {
	return &FailoverMonitor{
		nodeRepo:    nodeRepo,
		failoverSvc: failoverSvc,
		logger:      logger.With("component", "failover-monitor"),
		config:      config,
	}
}

// Start begins the failover monitoring loop.
// It runs until the context is cancelled.
func (m *FailoverMonitor) Start(ctx context.Context) {
	if !m.config.Enabled {
		m.logger.Info("auto-failover monitor disabled, not starting")
		return
	}

	m.logger.Info("failover monitor started",
		"check_interval", m.config.CheckInterval,
		"threshold", m.config.Threshold)

	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	// Run immediately on start
	m.checkNodes(ctx)

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("failover monitor stopped", "reason", ctx.Err())
			return
		case <-ticker.C:
			m.checkNodes(ctx)
		}
	}
}

// checkNodes queries all nodes and triggers failover for those that have exceeded
// the heartbeat miss threshold.
func (m *FailoverMonitor) checkNodes(ctx context.Context) error {
	// Generate correlation ID for this check cycle
	correlationID := uuid.New().String()

	m.logger.Debug("checking nodes for failover conditions",
		"correlation_id", correlationID,
		"threshold", m.config.Threshold)

	// Query all active nodes where consecutive_heartbeat_misses >= threshold
	nodes, err := m.getNodesNeedingFailover(ctx)
	if err != nil {
		m.logger.Error("failed to query nodes for failover",
			"correlation_id", correlationID,
			"error", err)
		return fmt.Errorf("querying nodes for failover: %w", err)
	}

	if len(nodes) == 0 {
		m.logger.Debug("no nodes need failover",
			"correlation_id", correlationID)
		return nil
	}

	m.logger.Info("found nodes needing failover",
		"correlation_id", correlationID,
		"count", len(nodes))

	// Process each node
	for _, node := range nodes {
		m.processNodeFailover(ctx, node, correlationID)
	}

	return nil
}

// getNodesNeedingFailover returns all nodes that have exceeded the heartbeat miss threshold
// and are still in active status.
func (m *FailoverMonitor) getNodesNeedingFailover(ctx context.Context) ([]models.Node, error) {
	// Get all nodes with active status
	activeNodes, err := m.nodeRepo.ListByStatus(ctx, models.NodeStatusOnline)
	if err != nil {
		return nil, fmt.Errorf("listing online nodes: %w", err)
	}

	// Also check degraded nodes as they might fail
	degradedNodes, err := m.nodeRepo.ListByStatus(ctx, models.NodeStatusDegraded)
	if err != nil {
		// Log but continue - degraded is less critical
		m.logger.Warn("failed to list degraded nodes", "error", err)
	}

	// Combine and filter
	allNodes := append(activeNodes, degradedNodes...)
	var needsFailover []models.Node

	for _, node := range allNodes {
		if node.ConsecutiveHeartbeatMisses >= m.config.Threshold {
			needsFailover = append(needsFailover, node)
		}
	}

	return needsFailover, nil
}

// processNodeFailover handles the failover process for a single node.
func (m *FailoverMonitor) processNodeFailover(ctx context.Context, node models.Node, correlationID string) {
	startTime := time.Now()

	m.logger.Info("initiating auto-failover for node",
		"correlation_id", correlationID,
		"node_id", node.ID,
		"hostname", node.Hostname,
		"consecutive_heartbeat_misses", node.ConsecutiveHeartbeatMisses)

	// Use AutoFailover which doesn't require admin approval for automated failover
	result, err := m.failoverSvc.AutoFailover(ctx, node.ID, correlationID)
	if err != nil {
		m.logger.Error("auto-failover failed for node",
			"correlation_id", correlationID,
			"node_id", node.ID,
			"hostname", node.Hostname,
			"error", err)
		return
	}

	// Record metrics
	duration := time.Since(startTime)
	m.mu.Lock()
	m.failoverCount++
	m.lastFailoverAt = &startTime
	m.recoveryTimeMs = duration.Milliseconds()
	m.mu.Unlock()

	m.logger.Info("auto-failover completed for node",
		"correlation_id", correlationID,
		"node_id", node.ID,
		"hostname", node.Hostname,
		"duration_ms", duration.Milliseconds(),
		"total_vms", result.TotalVMs,
		"migrated_vms", len(result.MigratedVMs),
		"failed_migrations", len(result.FailedMigrations),
		"stonith_executed", result.STONITHExecuted,
		"blocklist_added", result.BlocklistAdded)
}

// Metrics returns current failover monitor metrics.
func (m *FailoverMonitor) Metrics() FailoverMonitorMetrics {
	m.mu.Lock()
	defer m.mu.Unlock()
	return FailoverMonitorMetrics{
		FailoverCount:  m.failoverCount,
		LastFailoverAt: m.lastFailoverAt,
		RecoveryTimeMs: m.recoveryTimeMs,
		CheckInterval:  m.config.CheckInterval,
		Threshold:      m.config.Threshold,
		Enabled:        m.config.Enabled,
	}
}

// FailoverMonitorMetrics holds metrics about the failover monitor.
type FailoverMonitorMetrics struct {
	FailoverCount  int64         `json:"failover_count"`
	LastFailoverAt *time.Time    `json:"last_failover_at,omitempty"`
	RecoveryTimeMs int64         `json:"recovery_time_ms"`
	CheckInterval  time.Duration `json:"check_interval"`
	Threshold      int           `json:"threshold"`
	Enabled        bool          `json:"enabled"`
}
