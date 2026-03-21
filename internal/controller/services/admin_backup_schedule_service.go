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
	nextRunAt := calculateNextRunTime(schedule.Frequency, now)
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
func (s *AdminBackupScheduleService) findTargetVMs(ctx context.Context, schedule models.AdminBackupSchedule) ([]models.VM, error) {
	// If targeting all VMs, get all active VMs
	if schedule.TargetAll {
		vms, err := s.vmRepo.ListAllActive(ctx)
		if err != nil {
			return nil, err
		}
		return vms, nil
	}

	// Note: The VM repository doesn't support filtering by multiple plan IDs directly.
	// We fetch VMs matching node/customer criteria and filter plan IDs in-memory.

	var allVMs []models.VM
	seenVMs := make(map[string]bool)

	// Build a map of plan IDs for quick lookup
	planIDSet := make(map[string]bool)
	for _, planID := range schedule.TargetPlanIDs {
		planIDSet[planID] = true
	}

	// Fetch VMs by node IDs
	for _, nodeID := range schedule.TargetNodeIDs {
		nodeFilter := models.VMListFilter{
			PaginationParams: models.PaginationParams{Page: 1, PerPage: models.MaxPerPage},
			NodeID:           &nodeID,
		}
		vms, _, err := s.vmRepo.List(ctx, nodeFilter)
		if err != nil {
			s.logger.Warn("failed to list VMs by node ID", "node_id", nodeID, "error", err)
			continue
		}
		for _, vm := range vms {
			if !seenVMs[vm.ID] {
				seenVMs[vm.ID] = true
				allVMs = append(allVMs, vm)
			}
		}
	}

	// Fetch VMs by customer IDs
	for _, customerID := range schedule.TargetCustomerIDs {
		customerFilter := models.VMListFilter{
			PaginationParams: models.PaginationParams{Page: 1, PerPage: models.MaxPerPage},
			CustomerID:       &customerID,
		}
		vms, _, err := s.vmRepo.List(ctx, customerFilter)
		if err != nil {
			s.logger.Warn("failed to list VMs by customer ID", "customer_id", customerID, "error", err)
			continue
		}
		for _, vm := range vms {
			if !seenVMs[vm.ID] {
				seenVMs[vm.ID] = true
				allVMs = append(allVMs, vm)
			}
		}
	}

	// If plan IDs are specified and we have no VMs from node/customer targeting,
	// we need to query all VMs and filter by plan ID in-memory
	if len(planIDSet) > 0 && len(allVMs) == 0 {
		// Query all active VMs and filter by plan ID
		allActiveVMs, err := s.vmRepo.ListAllActive(ctx)
		if err != nil {
			s.logger.Warn("failed to list all active VMs for plan filtering", "error", err)
		} else {
			for _, vm := range allActiveVMs {
				if planIDSet[vm.PlanID] && !seenVMs[vm.ID] {
					seenVMs[vm.ID] = true
					allVMs = append(allVMs, vm)
				}
			}
		}
	} else if len(planIDSet) > 0 {
		// Filter existing VMs by plan ID as well (intersection with node/customer targeting)
		filtered := allVMs[:0]
		for _, vm := range allVMs {
			if planIDSet[vm.PlanID] {
				filtered = append(filtered, vm)
			}
		}
		allVMs = filtered
	}

	return allVMs, nil
}

// generateAdminBackupName generates a backup name for an admin schedule.
func generateAdminBackupName(scheduleName string, now time.Time) string {
	return scheduleName + "-" + now.Format("20060102-150405")
}

// calculateNextRunTime calculates the next run time based on frequency.
func calculateNextRunTime(frequency string, from time.Time) time.Time {
	switch frequency {
	case models.AdminBackupScheduleFrequencyDaily:
		return from.Add(24 * time.Hour)
	case models.AdminBackupScheduleFrequencyWeekly:
		return from.Add(7 * 24 * time.Hour)
	case models.AdminBackupScheduleFrequencyMonthly:
		return from.AddDate(0, 1, 0)
	default:
		return from.Add(24 * time.Hour)
	}
}