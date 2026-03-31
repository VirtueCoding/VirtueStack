package controller

import (
	"context"
	"time"

	controllermetrics "github.com/AbuGosok/VirtueStack/internal/controller/metrics"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	nodeagentpb "github.com/AbuGosok/VirtueStack/internal/shared/proto/virtuestack"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
)

// StartSchedulers starts background schedulers (e.g., backup scheduler).
// Each scheduler runs in its own goroutine and stops when the context is cancelled.
func (s *Server) StartSchedulers(ctx context.Context) {
	if s.backupService != nil {
		s.logger.Info("starting backup scheduler")
		go s.backupService.StartScheduler(ctx)
	}

	if s.adminBackupScheduleService != nil {
		s.logger.Info("starting admin backup schedule scheduler")
		go s.adminBackupScheduleService.StartScheduler(ctx)
	}

	if s.failoverMonitor != nil {
		s.logger.Info("starting failover monitor")
		go s.failoverMonitor.Start(ctx)
	}

	if s.heartbeatChecker != nil {
		s.logger.Info("starting heartbeat checker")
		go s.heartbeatChecker.Start(ctx)
	}

	if s.taskWorker != nil {
		s.logger.Info("starting stuck task scanner")
		go s.taskWorker.StartStuckTaskScanner(ctx, 5*time.Minute, 30*time.Minute)
	}

	s.startMetricsCollector(ctx)

	if s.bandwidthRepo != nil && s.nodeClient != nil {
		go s.startBandwidthCollector(ctx)
	}

	go s.startSessionCleanup(ctx)

	if s.inAppNotifService != nil {
		s.logger.Info("starting notification cleanup scheduler")
		go s.inAppNotifService.StartCleanupScheduler(ctx, 6*time.Hour, 90*24*time.Hour)
	}

	if s.billingScheduler != nil {
		s.logger.Info("starting billing scheduler")
		go s.billingScheduler.Start(ctx)
	}
}

func (s *Server) startMetricsCollector(ctx context.Context) {
	if s.dbPool == nil {
		return
	}

	vmRepo := repository.NewVMRepository(s.dbPool)
	nodeRepo := repository.NewNodeRepository(s.dbPool)

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.collectControllerMetrics(ctx, vmRepo, nodeRepo)
			}
		}
	}()
}

func (s *Server) collectControllerMetrics(ctx context.Context, vmRepo *repository.VMRepository, nodeRepo *repository.NodeRepository) {
	vmStatuses := []string{models.VMStatusRunning, models.VMStatusStopped, models.VMStatusProvisioning, models.VMStatusSuspended, models.VMStatusMigrating, models.VMStatusError}
	for _, status := range vmStatuses {
		vms, _, _, err := vmRepo.List(ctx, models.VMListFilter{
			Status:           util.StringPtr(status),
			PaginationParams: models.PaginationParams{PerPage: models.MaxPerPage},
		})
		count := 0
		if err == nil {
			count = len(vms)
		}
		controllermetrics.VMsTotal.WithLabelValues(status).Set(float64(count))
	}

	nodeStatuses := []string{models.NodeStatusOnline, models.NodeStatusOffline, models.NodeStatusDraining, models.NodeStatusDegraded, models.NodeStatusFailed}
	for _, status := range nodeStatuses {
		nodes, _, _, err := nodeRepo.List(ctx, models.NodeListFilter{
			Status:           &status,
			PaginationParams: models.PaginationParams{PerPage: models.MaxPerPage},
		})
		count := 0
		if err == nil {
			count = len(nodes)
		}
		controllermetrics.NodesTotal.WithLabelValues(status).Set(float64(count))
	}

	onlineNodes, _, _, err := nodeRepo.List(ctx, models.NodeListFilter{
		Status:           util.StringPtr(models.NodeStatusOnline),
		PaginationParams: models.PaginationParams{PerPage: models.MaxPerPage},
	})
	if err != nil {
		return
	}

	now := time.Now()
	for _, node := range onlineNodes {
		var age float64
		if node.LastHeartbeatAt != nil {
			age = now.Sub(*node.LastHeartbeatAt).Seconds()
		} else {
			age = 9999
		}
		controllermetrics.NodeHeartbeatAge.WithLabelValues(node.ID).Set(age)
	}
}

func (s *Server) startBandwidthCollector(ctx context.Context) {
	s.logger.Info("starting bandwidth collector")

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("bandwidth collector stopped")
			return
		case <-ticker.C:
			s.collectBandwidth(ctx)
		}
	}
}

func (s *Server) startSessionCleanup(ctx context.Context) {
	if s.dbPool == nil {
		return
	}

	s.logger.Info("starting session cleanup scheduler")

	customerRepo := repository.NewCustomerRepository(s.dbPool)
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("session cleanup scheduler stopped")
			return
		case <-ticker.C:
			s.cleanupExpiredSessions(ctx, customerRepo)
		}
	}
}

func (s *Server) cleanupExpiredSessions(ctx context.Context, customerRepo *repository.CustomerRepository) {
	if err := customerRepo.DeleteExpiredSessions(ctx); err != nil {
		s.logger.Warn("failed to delete expired sessions", "error", err)
		return
	}
	s.logger.Debug("expired sessions cleaned up")
}

func (s *Server) collectBandwidth(ctx context.Context) {
	vmRepo := repository.NewVMRepository(s.dbPool)

	vms, _, _, err := vmRepo.List(ctx, models.VMListFilter{
		Status:           util.StringPtr(models.VMStatusRunning),
		PaginationParams: models.PaginationParams{PerPage: models.MaxPerPage},
	})
	if err != nil {
		s.logger.Warn("failed to list running VMs for bandwidth collection", "error", err)
		return
	}

	for _, vm := range vms {
		if vm.NodeID == nil {
			continue
		}

		node, err := repository.NewNodeRepository(s.dbPool).GetByID(ctx, *vm.NodeID)
		if err != nil {
			continue
		}

		conn, err := s.nodeClient.GetConnection(ctx, *vm.NodeID, node.GRPCAddress)
		if err != nil {
			s.logger.Debug("failed to get connection for bandwidth", "vm_id", vm.ID, "node_id", *vm.NodeID, "error", err)
			continue
		}

		pbClient := nodeagentpb.NewNodeAgentServiceClient(conn)
		bwResp, err := pbClient.GetBandwidthUsage(ctx, &nodeagentpb.VMIdentifier{VmId: vm.ID})
		if err != nil {
			s.logger.Debug("failed to get bandwidth for VM", "vm_id", vm.ID, "error", err)
			continue
		}

		if bwResp.RxBytes > 0 || bwResp.TxBytes > 0 {
			controllermetrics.BandwidthBytesTotal.WithLabelValues(vm.ID, "rx").Set(float64(bwResp.RxBytes))
			controllermetrics.BandwidthBytesTotal.WithLabelValues(vm.ID, "tx").Set(float64(bwResp.TxBytes))
		}
	}
}
