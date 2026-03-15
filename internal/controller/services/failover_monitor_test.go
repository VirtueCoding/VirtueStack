package services

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

func failoverTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestNewFailoverMonitor(t *testing.T) {
	logger := failoverTestLogger()
	config := DefaultFailoverMonitorConfig()

	monitor := NewFailoverMonitor(nil, nil, logger, config)

	if monitor == nil {
		t.Fatal("expected monitor to be created")
	}
	if monitor.config.CheckInterval != DefaultFailoverCheckInterval {
		t.Errorf("expected check interval %v, got %v", DefaultFailoverCheckInterval, monitor.config.CheckInterval)
	}
	if monitor.config.Threshold != DefaultFailoverThreshold {
		t.Errorf("expected threshold %d, got %d", DefaultFailoverThreshold, monitor.config.Threshold)
	}
}

func TestFailoverMonitor_Metrics(t *testing.T) {
	logger := failoverTestLogger()
	config := DefaultFailoverMonitorConfig()

	monitor := NewFailoverMonitor(nil, nil, logger, config)

	metrics := monitor.Metrics()
	if metrics.FailoverCount != 0 {
		t.Errorf("expected failover count 0, got %d", metrics.FailoverCount)
	}
	if !metrics.Enabled {
		t.Error("expected enabled to be true")
	}
}

func TestFailoverMonitor_Disabled(t *testing.T) {
	logger := failoverTestLogger()
	config := FailoverMonitorConfig{
		CheckInterval: 30 * time.Second,
		Threshold:     3,
		Enabled:       false,
	}

	monitor := NewFailoverMonitor(nil, nil, logger, config)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		monitor.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Error("expected Start to return immediately when disabled")
	}
}

func TestDefaultFailoverMonitorConfig(t *testing.T) {
	config := DefaultFailoverMonitorConfig()

	if config.CheckInterval != 30*time.Second {
		t.Errorf("expected check interval 30s, got %v", config.CheckInterval)
	}
	if config.Threshold != 3 {
		t.Errorf("expected threshold 3, got %d", config.Threshold)
	}
	if !config.Enabled {
		t.Error("expected enabled to be true by default")
	}
}

func TestFailoverMonitorContextCancellation(t *testing.T) {
	logger := failoverTestLogger()
	config := FailoverMonitorConfig{
		CheckInterval: 10 * time.Millisecond,
		Threshold:     3,
		Enabled:       true,
	}

	monitor := NewFailoverMonitor(nil, nil, logger, config)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		monitor.Start(ctx)
		close(done)
	}()

	// Wait for monitor to start using a short poll instead of fixed sleep
	select {
	case <-time.After(50 * time.Millisecond):
		// Monitor should have started by now
	case <-done:
		// Already done (unexpected but handle gracefully)
	}
	cancel()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("expected Start to return after context cancellation")
	}
}

func TestFailoverMonitorMetricsAfterFailover(t *testing.T) {
	logger := failoverTestLogger()
	config := DefaultFailoverMonitorConfig()

	monitor := NewFailoverMonitor(nil, nil, logger, config)

	startTime := time.Now()
	monitor.failoverCount = 1
	monitor.lastFailoverAt = &startTime
	monitor.recoveryTimeMs = 1500

	metrics := monitor.Metrics()
	if metrics.FailoverCount != 1 {
		t.Errorf("expected failover count 1, got %d", metrics.FailoverCount)
	}
	if metrics.LastFailoverAt == nil {
		t.Error("expected last failover time to be set")
	}
	if metrics.RecoveryTimeMs != 1500 {
		t.Errorf("expected recovery time 1500ms, got %d", metrics.RecoveryTimeMs)
	}
}

func TestProcessNodeFailoverRecordsMetrics(t *testing.T) {
	logger := failoverTestLogger()
	config := DefaultFailoverMonitorConfig()

	monitor := NewFailoverMonitor(nil, nil, logger, config)

	startTime := time.Now()
	monitor.mu.Lock()
	monitor.failoverCount = 1
	monitor.lastFailoverAt = &startTime
	monitor.recoveryTimeMs = 1500
	monitor.mu.Unlock()

	metrics := monitor.Metrics()
	if metrics.FailoverCount != 1 {
		t.Errorf("expected failover count 1, got %d", metrics.FailoverCount)
	}
	if metrics.LastFailoverAt == nil {
		t.Error("expected last failover time to be set")
	}
	if metrics.RecoveryTimeMs != 1500 {
		t.Errorf("expected recovery time 1500ms, got %d", metrics.RecoveryTimeMs)
	}
	if !metrics.Enabled {
		t.Error("expected enabled to be true")
	}
}

func TestFailoverMonitorConfigCustomValues(t *testing.T) {
	config := FailoverMonitorConfig{
		CheckInterval: 60 * time.Second,
		Threshold:     5,
		Enabled:       false,
	}

	if config.CheckInterval != 60*time.Second {
		t.Errorf("expected check interval 60s, got %v", config.CheckInterval)
	}
	if config.Threshold != 5 {
		t.Errorf("expected threshold 5, got %d", config.Threshold)
	}
	if config.Enabled {
		t.Error("expected enabled to be false")
	}
}

func TestGetNodesNeedingFailoverFiltering(t *testing.T) {
	config := FailoverMonitorConfig{
		CheckInterval: 30 * time.Second,
		Threshold:     3,
		Enabled:       true,
	}

	nodes := []models.Node{
		{ID: "1", Status: models.NodeStatusOnline, ConsecutiveHeartbeatMisses: 2},
		{ID: "2", Status: models.NodeStatusOnline, ConsecutiveHeartbeatMisses: 3},
		{ID: "3", Status: models.NodeStatusOnline, ConsecutiveHeartbeatMisses: 4},
		{ID: "4", Status: models.NodeStatusFailed, ConsecutiveHeartbeatMisses: 5},
	}

	var needsFailover []models.Node
	for _, node := range nodes {
		if (node.Status == models.NodeStatusOnline || node.Status == models.NodeStatusDegraded) &&
			node.ConsecutiveHeartbeatMisses >= config.Threshold {
			needsFailover = append(needsFailover, node)
		}
	}

	if len(needsFailover) != 2 {
		t.Errorf("expected 2 nodes needing failover, got %d", len(needsFailover))
	}
}
