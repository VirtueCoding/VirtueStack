// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
)

func (s *BackupService) StartScheduler(ctx context.Context) {
	s.logger.Info("backup scheduler started")

	ticker := time.NewTicker(schedulerInterval)
	defer ticker.Stop()

	// Run immediately on start
	s.runSchedulerTick(ctx)

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("backup scheduler stopped", "reason", ctx.Err())
			return
		case <-ticker.C:
			s.runSchedulerTick(ctx)
		}
	}
}

func (s *BackupService) runSchedulerTick(ctx context.Context) {
	now := time.Now().UTC()
	currentDay := now.Day()
	currentYear, currentMonth, _ := now.Date()

	s.logger.Debug("running backup scheduler tick",
		"day", currentDay,
		"month", currentMonth,
		"year", currentYear)

	vms, err := s.vmRepo.ListAllActive(ctx)
	if err != nil {
		s.logger.Error("failed to list active VMs for backup scheduling", "error", err)
		return
	}

	stats := s.processVMsForBackup(ctx, vms, currentDay, currentYear, int(currentMonth))

	s.logger.Info("backup scheduler tick completed",
		"total_vms", len(vms),
		"backups_scheduled", stats.backupsScheduled,
		"skipped_has_backup", stats.skippedCount,
		"errors", stats.errorCount)
}

func (s *BackupService) processVMsForBackup(ctx context.Context, vms []models.VM, currentDay, currentYear, currentMonth int) schedulerStats {
	var stats schedulerStats

	for _, vm := range vms {
		assignedDay := s.getVMBackupDay(vm.ID)
		if assignedDay != currentDay {
			continue
		}

		shouldBackup, err := s.shouldBackupVM(ctx, vm.ID, currentYear, currentMonth)
		if err != nil {
			stats.errorCount++
			continue
		}
		if !shouldBackup {
			stats.skippedCount++
			continue
		}

		if s.scheduleBackupForVM(ctx, vm.ID, currentYear, currentMonth) {
			stats.backupsScheduled++
		}
	}

	return stats
}

func (s *BackupService) shouldBackupVM(ctx context.Context, vmID string, year, month int) (bool, error) {
	hasBackup, err := s.backupRepo.HasBackupInMonth(ctx, vmID, year, month)
	if err != nil {
		s.logger.Error("failed to check backup status for VM",
			"vm_id", vmID,
			"error", err)
		return false, err
	}

	if hasBackup {
		s.logger.Debug("VM already has backup this month, skipping",
			"vm_id", vmID)
		return false, nil
	}

	return true, nil
}

func (s *BackupService) scheduleBackupForVM(ctx context.Context, vmID string, year, month int) bool {
	if s.taskPublisher == nil {
		s.logger.Warn("task publisher not configured, cannot schedule backup",
			"vm_id", vmID)
		return false
	}

	taskID, err := s.taskPublisher.PublishTask(ctx, "backup.create", map[string]any{
		"vm_id":       vmID,
		"backup_name": fmt.Sprintf("monthly-%d-%02d", year, month),
		"source":      models.BackupSourceCustomerSchedule,
	})
	if err != nil {
		s.logger.Error("failed to publish backup task for VM",
			"vm_id", vmID,
			"error", err)
		return false
	}

	s.logger.Info("scheduled monthly backup for VM",
		"vm_id", vmID,
		"task_id", taskID,
		"assigned_day", s.getVMBackupDay(vmID))

	return true
}

func (s *BackupService) CreateSchedule(ctx context.Context, schedule *models.BackupSchedule) (string, error) {
	if schedule == nil {
		return "", fmt.Errorf("schedule is required")
	}
	if schedule.VMID == "" || schedule.CustomerID == "" {
		return "", fmt.Errorf("vm_id and customer_id are required")
	}

	frequency := strings.ToLower(strings.TrimSpace(schedule.Frequency))
	if frequency != "daily" && frequency != "weekly" && frequency != "monthly" {
		return "", fmt.Errorf("invalid frequency: %s", schedule.Frequency)
	}

	if schedule.RetentionCount <= 0 {
		schedule.RetentionCount = 30
	}

	schedule.Frequency = frequency
	schedule.NextRunAt = computeNextRun(time.Now().UTC(), frequency)
	schedule.Active = true

	if err := s.backupRepo.CreateBackupSchedule(ctx, schedule); err != nil {
		return "", fmt.Errorf("creating schedule: %w", err)
	}

	s.logger.Info("backup schedule created",
		"schedule_id", schedule.ID,
		"vm_id", schedule.VMID,
		"customer_id", schedule.CustomerID,
		"frequency", schedule.Frequency)

	return schedule.ID, nil
}

func (s *BackupService) ListSchedules(ctx context.Context, vmID string) ([]*models.BackupSchedule, error) {
	filter := repository.BackupScheduleListFilter{
		PaginationParams: models.PaginationParams{
			PerPage: 100, // Default limit for non-paginated list
		},
	}
	if vmID != "" {
		filter.VMID = &vmID
	}

	schedules, _, _, err := s.backupRepo.ListBackupSchedules(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("listing schedules: %w", err)
	}

	result := make([]*models.BackupSchedule, 0, len(schedules))
	for i := range schedules {
		sched := schedules[i]
		result = append(result, &sched)
	}

	return result, nil
}

func (s *BackupService) ListSchedulesPaginated(ctx context.Context, vmID string, pagination models.PaginationParams) ([]*models.BackupSchedule, bool, string, error) {
	filter := repository.BackupScheduleListFilter{
		PaginationParams: pagination,
	}
	if vmID != "" {
		filter.VMID = &vmID
	}

	schedules, hasMore, lastID, err := s.backupRepo.ListBackupSchedules(ctx, filter)
	if err != nil {
		return nil, false, "", fmt.Errorf("listing schedules: %w", err)
	}

	result := make([]*models.BackupSchedule, 0, len(schedules))
	for i := range schedules {
		sched := schedules[i]
		result = append(result, &sched)
	}

	return result, hasMore, lastID, nil
}

func (s *BackupService) UpdateSchedule(ctx context.Context, scheduleID string, enabled bool) error {
	if err := s.backupRepo.UpdateBackupScheduleActive(ctx, scheduleID, enabled); err != nil {
		return fmt.Errorf("updating schedule: %w", err)
	}
	return nil
}

func (s *BackupService) DeleteSchedule(ctx context.Context, scheduleID string) error {
	if err := s.backupRepo.DeleteBackupSchedule(ctx, scheduleID); err != nil {
		return fmt.Errorf("deleting schedule: %w", err)
	}
	return nil
}

func (s *BackupService) ApplyRetentionPolicy(ctx context.Context, vmID string, retention int) error {
	if retention < 0 {
		return fmt.Errorf("retention must be >= 0")
	}

	backups, err := s.backupRepo.ListBackupsByVM(ctx, vmID)
	if err != nil {
		return fmt.Errorf("listing backups: %w", err)
	}

	if retention >= len(backups) {
		return nil
	}

	for i := retention; i < len(backups); i++ {
		if err := s.backupRepo.DeleteBackup(ctx, backups[i].ID); err != nil {
			return fmt.Errorf("deleting backup %s: %w", backups[i].ID, err)
		}
	}

	return nil
}

func (s *BackupService) ProcessExpiredBackups(ctx context.Context) (int, error) {
	expired, err := s.backupRepo.ListExpiredBackups(ctx, time.Now().UTC())
	if err != nil {
		return 0, fmt.Errorf("listing expired backups: %w", err)
	}

	deleted := 0
	for _, b := range expired {
		if err := s.backupRepo.DeleteBackup(ctx, b.ID); err != nil {
			s.logger.Warn("failed to delete expired backup", "backup_id", b.ID, "error", err)
			continue
		}
		deleted++
	}

	return deleted, nil
}
