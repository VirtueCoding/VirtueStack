package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
)

const (
	DefaultHeartbeatCheckInterval = 15 * time.Second
	DefaultHeartbeatTimeout       = 5 * time.Minute
)

type HeartbeatCheckerConfig struct {
	CheckInterval time.Duration
	Timeout       time.Duration
	Enabled       bool
}

func DefaultHeartbeatCheckerConfig() HeartbeatCheckerConfig {
	return HeartbeatCheckerConfig{
		CheckInterval: DefaultHeartbeatCheckInterval,
		Timeout:       DefaultHeartbeatTimeout,
		Enabled:       true,
	}
}

type HeartbeatChecker struct {
	nodeRepo *repository.NodeRepository
	logger   *slog.Logger
	config   HeartbeatCheckerConfig
}

func NewHeartbeatChecker(nodeRepo *repository.NodeRepository, logger *slog.Logger, config HeartbeatCheckerConfig) *HeartbeatChecker {
	return &HeartbeatChecker{
		nodeRepo: nodeRepo,
		logger:   logger.With("component", "heartbeat-checker"),
		config:   config,
	}
}

func (hc *HeartbeatChecker) Start(ctx context.Context) {
	if !hc.config.Enabled {
		hc.logger.Info("heartbeat checker disabled")
		return
	}

	hc.logger.Info("heartbeat checker started",
		"check_interval", hc.config.CheckInterval,
		"timeout", hc.config.Timeout)

	ticker := time.NewTicker(hc.config.CheckInterval)
	defer ticker.Stop()

	hc.checkAllNodes(ctx)

	for {
		select {
		case <-ctx.Done():
			hc.logger.Info("heartbeat checker stopped")
			return
		case <-ticker.C:
			hc.checkAllNodes(ctx)
		}
	}
}

func (hc *HeartbeatChecker) checkAllNodes(ctx context.Context) {
	onlineNodes, err := hc.nodeRepo.ListByStatus(ctx, models.NodeStatusOnline)
	if err != nil {
		hc.logger.Error("failed to list online nodes", "error", err)
		return
	}

	degradedNodes, err := hc.nodeRepo.ListByStatus(ctx, models.NodeStatusDegraded)
	if err != nil {
		hc.logger.Warn("failed to list degraded nodes", "error", err)
	}

	allNodes := append(onlineNodes, degradedNodes...)
	cutoff := time.Now().Add(-hc.config.Timeout)

	for _, node := range allNodes {
		if node.LastHeartbeatAt == nil {
			misses := node.ConsecutiveHeartbeatMisses + 1
			if err := hc.nodeRepo.UpdateHeartbeatMisses(ctx, node.ID, misses); err != nil {
				hc.logger.Warn("failed to increment heartbeat misses", "node_id", node.ID, "error", err)
				continue
			}

			if misses == 1 {
				hc.logger.Warn("node has no heartbeat recorded, incrementing misses",
					"node_id", node.ID, "hostname", node.Hostname, "misses", misses)
			}
			continue
		}

		if node.LastHeartbeatAt.Before(cutoff) {
			misses := node.ConsecutiveHeartbeatMisses + 1
			if err := hc.nodeRepo.UpdateHeartbeatMisses(ctx, node.ID, misses); err != nil {
				hc.logger.Warn("failed to increment heartbeat misses", "node_id", node.ID, "error", err)
				continue
			}

			newStatus := node.Status
			if misses >= 3 && newStatus == models.NodeStatusOnline {
				newStatus = models.NodeStatusDegraded
				if err := hc.nodeRepo.UpdateStatus(ctx, node.ID, newStatus); err != nil {
					hc.logger.Warn("failed to set node to degraded", "node_id", node.ID, "error", err)
				}
			}

			hc.logger.Warn("node missed heartbeat",
				"node_id", node.ID,
				"hostname", node.Hostname,
				"misses", misses,
				"last_heartbeat", node.LastHeartbeatAt,
				"status", newStatus)
		}
	}
}
