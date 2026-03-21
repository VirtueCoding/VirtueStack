// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
)

// AdminBackupScheduleServiceConfig holds configuration for AdminBackupScheduleService.
type AdminBackupScheduleServiceConfig struct {
	AdminBackupScheduleRepo *repository.AdminBackupScheduleRepository
	VMRepo                  *repository.VMRepository
	BackupRepo              *repository.BackupRepository
	TaskPublisher           TaskPublisher
	Logger                  *slog.Logger
}

// AdminBackupScheduleService handles execution of admin backup schedules.
type AdminBackupScheduleService struct {
	adminScheduleRepo *repository.AdminBackupScheduleRepository
	vmRepo            *repository.VMRepository
	backupRepo        *repository.BackupRepository
	taskPublisher     TaskPublisher
	logger            *slog.Logger
}

// NewAdminBackupScheduleService creates a new AdminBackupScheduleService.
func NewAdminBackupScheduleService(cfg AdminBackupScheduleServiceConfig) *AdminBackupScheduleService {
	return &AdminBackupScheduleService{
		adminScheduleRepo: cfg.AdminBackupScheduleRepo,
		vmRepo:            cfg.VMRepo,
		backupRepo:        cfg.BackupRepo,
		taskPublisher:     cfg.TaskPublisher,
		logger:            cfg.Logger.With("service", "admin-backup-schedule"),
	}
}

// adminSchedulerInterval is how often the scheduler checks for due schedules.
const adminSchedulerInterval = 5 * time.Minute

// StartScheduler starts the admin backup schedule execution loop.
// It runs until the context is cancelled.
func (s *AdminBackupScheduleService) StartScheduler(ctx context.Context) {
	s.logger.Info("admin backup schedule scheduler started")

	ticker := time.NewTicker(adminSchedulerInterval)
	defer ticker.Stop()

	// Run immediately on start
	s.runSchedulerTick(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("admin backup schedule scheduler stopped", "reason", ctx.Err())
			return
		case <-ticker.C:
			s.runSchedulerTick(ctx)
		}
	}
}

// runSchedulerTick performs a single iteration of the admin backup scheduler.
func (s *AdminBackupScheduleService) runSchedulerTick(ctx context.Context) {
	now := time.Now().UTC()

	s.logger.Debug("running admin backup schedule scheduler tick")

	// Get all due schedules
	schedules, err := s.adminScheduleRepo.ListDueSchedules(ctx, now)
	if err != nil {
		s.logger.Error("failed to list due admin backup schedules", "error", err)
		return
	}

	if len(schedules) == 0 {
		s.logger.Debug("no admin backup schedules due")
		return
	}

	s.logger.Info("processing due admin backup schedules", "count", len(schedules))

	stats := adminSchedulerStats{}
	for _, schedule := range schedules {
		scheduleStats := s.executeSchedule(ctx, schedule, now)
		stats.backupsScheduled += scheduleStats.backupsScheduled
		stats.vmsTargeted += scheduleStats.vmsTargeted
		stats.errors += scheduleStats.errors
	}

	s.logger.Info("admin backup schedule scheduler tick completed",
		"schedules_processed", len(schedules),
		"vms_targeted", stats.vmsTargeted,
		"backups_scheduled", stats.backupsScheduled,
		"errors", stats.errors)
}

type adminSchedulerStats struct {
	backupsScheduled int
	vmsTargeted      int
	errors           int
}

// executeSchedule executes a single admin backup schedule.
func (s *AdminBackupScheduleService) executeSchedule(ctx context.Context, schedule models.AdminBackupSchedule, now time.Time) adminSchedulerStats {
	stats := adminSchedulerStats{}

	// Find target VMs
	targetVMs, err := s.findTargetVMs(ctx, schedule)
	if err != nil {
		s.logger.Error("failed to find target VMs for admin backup schedule",
			"schedule_id", schedule.ID,
			"error", err)
		stats.errors++
		return stats
	}

	stats.vmsTargeted = len(targetVMs)

	s.logger.Info("executing admin backup schedule",
		"schedule_id", schedule.ID,
		"schedule_name", schedule.Name,
		"target_vm_count", len(targetVMs))

	// Create backup tasks for each VM
	for _, vm := range targetVMs {
		// Check if VM is in a state that allows backups
		if vm.Status != models.VMStatusRunning && vm.Status != models.VMStatusStopped {
			s.logger.Debug("skipping VM - invalid status for backup",
				"vm_id", vm.ID,
				"vm_status", vm.Status)
			continue
		}

		// Create backup task payload
		payload := map[string]any{
			"vm_id":       vm.ID,
			"backup_name": generateAdminBackupName(schedule.Name, now),
			"source":      models.BackupSourceAdminSchedule,
		}
		if schedule.ID != "" {
			payload["admin_schedule_id"] = schedule.ID
		}

		taskID, err := s.taskPublisher.PublishTask(ctx, "backup.create", payload)
		if err != nil {
			s.logger.Error("failed to create backup task for VM",
				"vm_id", vm.ID,
				"schedule_id", schedule.ID,
				"error", err)
			stats.errors++
			continue
		}

		stats.backupsScheduled++
		s.logger.Debug("backup task created",
			"vm_id", vm.ID,
			"task_id", taskID,
			"schedule_id", schedule.ID)
	}

	// Update schedule's next run time
	nextRunAt := models.CalculateNextRunTime(schedule.Frequency, now)
	if err := s.adminScheduleRepo.UpdateNextRunAt(ctx, schedule.ID, nextRunAt, now); err != nil {
		s.logger.Error("failed to update admin backup schedule next run time",
			"schedule_id", schedule.ID,
			"error", err)
		// Don't count as error since backups were scheduled
	}

	s.logger.Info("admin backup schedule execution completed",
		"schedule_id", schedule.ID,
		"backups_scheduled", stats.backupsScheduled,
		"next_run_at", nextRunAt)

	return stats
}

// findTargetVMs finds all VMs that match the schedule's targeting criteria.
// Uses efficient batch queries to avoid N+1 database calls.
func (s *AdminBackupScheduleService) findTargetVMs(ctx context.Context, schedule models.AdminBackupSchedule) ([]models.VM, error) {
	// If targeting all VMs, get all active VMs
	if schedule.TargetAll {
		vms, err := s.vmRepo.ListAllActive(ctx)
		if err != nil {
			return nil, err
		}
		return vms, nil
	}

	// Use the efficient batch query method that applies all filters in a single query
	filter := repository.AdminBackupTargetFilter{
		NodeIDs:     schedule.TargetNodeIDs,
		CustomerIDs: schedule.TargetCustomerIDs,
		PlanIDs:     schedule.TargetPlanIDs,
	}

	vms, err := s.vmRepo.ListForAdminBackupTarget(ctx, filter)
	if err != nil {
		s.logger.Error("failed to list VMs for admin backup target",
			"schedule_id", schedule.ID,
			"node_ids", schedule.TargetNodeIDs,
			"customer_ids", schedule.TargetCustomerIDs,
			"plan_ids", schedule.TargetPlanIDs,
			"error", err)
		return nil, err
	}

	return vms, nil
}

// generateAdminBackupName generates a backup name for an admin schedule.
func generateAdminBackupName(scheduleName string, now time.Time) string {
	return scheduleName + "-" + now.Format("20060102-150405")
}