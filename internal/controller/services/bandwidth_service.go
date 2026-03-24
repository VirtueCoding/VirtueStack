// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/AbuGosok/VirtueStack/internal/shared/util"
)

// BandwidthThrottler abstracts bandwidth throttling operations on node agents.
// This interface allows the BandwidthService to trigger throttling without
// depending directly on node agent communication details.
type BandwidthThrottler interface {
	// ApplyThrottle applies bandwidth throttling to a VM on a node.
	ApplyThrottle(ctx context.Context, nodeID, vmID string, rateKbps int) error
	// RemoveThrottle removes bandwidth throttling from a VM on a node.
	RemoveThrottle(ctx context.Context, nodeID, vmID string) error
}

// BandwidthService provides bandwidth accounting and throttling for VirtueStack.
// It tracks monthly bandwidth usage per VM and enforces throttling when limits
// are exceeded.
type BandwidthService struct {
	vmRepo        *repository.VMRepository
	bandwidthRepo *repository.BandwidthRepository
	throttler     BandwidthThrottler
	logger        *slog.Logger
}

// NewBandwidthService creates a new BandwidthService with the given dependencies.
func NewBandwidthService(
	vmRepo *repository.VMRepository,
	bandwidthRepo *repository.BandwidthRepository,
	throttler BandwidthThrottler,
	logger *slog.Logger,
) *BandwidthService {
	return &BandwidthService{
		vmRepo:        vmRepo,
		bandwidthRepo: bandwidthRepo,
		throttler:     throttler,
		logger:        logger.With("component", "bandwidth-service"),
	}
}

// RecordUsage records bandwidth usage for a VM.
// This is typically called by a background job that collects network stats
// from node agents and aggregates them into monthly totals.
func (s *BandwidthService) RecordUsage(ctx context.Context, vmID string, bytesIn, bytesOut uint64) error {
	logger := s.logger.With("vm_id", vmID, "bytes_in", bytesIn, "bytes_out", bytesOut)

	// Get VM to check current status
	vm, err := s.vmRepo.GetByID(ctx, vmID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			logger.Debug("VM not found, skipping bandwidth record")
			return nil
		}
		return fmt.Errorf("getting VM: %w", err)
	}

	// Calculate current billing period
	now := time.Now().UTC()
	year, month := now.Year(), int(now.Month())

	// Update bandwidth usage
	if err := s.bandwidthRepo.UpdateUsage(ctx, vmID, year, month, bytesIn, bytesOut); err != nil {
		return fmt.Errorf("updating bandwidth usage: %w", err)
	}

	logger.Info("recorded bandwidth usage", "year", year, "month", month)

	// Check if we need to throttle
	if vm.NodeID != nil && !vm.IsDeleted() {
		if _, err := s.checkAndThrottle(ctx, vm); err != nil {
			logger.Warn("failed to check throttle status", "error", err)
		}
	}

	return nil
}

// GetMonthlyUsage retrieves the bandwidth usage for a VM for a specific month.
// If year and month are 0, the current billing period is used.
func (s *BandwidthService) GetMonthlyUsage(ctx context.Context, vmID string, year, month int) (*models.BandwidthUsage, error) {
	// Default to current billing period
	if year == 0 || month == 0 {
		now := time.Now().UTC()
		year, month = now.Year(), int(now.Month())
	}

	// Get VM to get bandwidth limit
	vm, err := s.vmRepo.GetByID(ctx, vmID)
	if err != nil {
		return nil, fmt.Errorf("getting VM: %w", err)
	}

	// Calculate limit in bytes
	limitBytes := uint64(vm.BandwidthLimitGB) * 1024 * 1024 * 1024

	usage, err := s.bandwidthRepo.GetOrCreateUsage(ctx, vmID, year, month, limitBytes)
	if err != nil {
		return nil, fmt.Errorf("getting bandwidth usage: %w", err)
	}

	return usage, nil
}

// CheckLimit checks if a VM has exceeded its bandwidth limit.
// Returns: exceeded (bool), currentUsage (bytes), limit (bytes), error
func (s *BandwidthService) CheckLimit(ctx context.Context, vmID string) (bool, uint64, uint64, error) {
	usage, err := s.GetMonthlyUsage(ctx, vmID, 0, 0)
	if err != nil {
		return false, 0, 0, fmt.Errorf("getting bandwidth usage: %w", err)
	}

	return usage.Exceeded(), usage.TotalBytes(), usage.LimitBytes, nil
}

// TriggerThrottling applies bandwidth throttling to a VM.
// This is typically called automatically when a VM exceeds its bandwidth limit,
// but can also be called manually by admins.
func (s *BandwidthService) TriggerThrottling(ctx context.Context, vmID string) error {
	logger := s.logger.With("vm_id", vmID, "operation", "trigger_throttling")

	// Get VM
	vm, err := s.vmRepo.GetByID(ctx, vmID)
	if err != nil {
		return fmt.Errorf("getting VM: %w", err)
	}

	// Check if VM has a node assigned
	if vm.NodeID == nil {
		return fmt.Errorf("VM has no node assigned")
	}

	// Check if already throttled
	now := time.Now().UTC()
	usage, err := s.bandwidthRepo.GetOrCreateUsage(ctx, vmID, now.Year(), int(now.Month()), uint64(vm.BandwidthLimitGB)*1024*1024*1024)
	if err != nil {
		return fmt.Errorf("getting bandwidth usage: %w", err)
	}

	if usage.Throttled {
		logger.Info("VM already throttled")
		return nil
	}

	// Apply throttle on node agent
	throttleConfig := models.DefaultThrottleConfig()
	if err := s.throttler.ApplyThrottle(ctx, *vm.NodeID, vmID, throttleConfig.RateKbps); err != nil {
		return fmt.Errorf("applying throttle: %w", err)
	}

	// Update database
	if err := s.bandwidthRepo.SetThrottled(ctx, vmID, now.Year(), int(now.Month()), true); err != nil {
		logger.Error("failed to mark VM as throttled in database", "error", err)
		// Attempt to remove throttle from node
		_ = s.throttler.RemoveThrottle(ctx, *vm.NodeID, vmID)
		return fmt.Errorf("marking VM as throttled: %w", err)
	}

	logger.Info("VM throttled successfully", "rate_kbps", throttleConfig.RateKbps)
	return nil
}

// RemoveThrottling removes bandwidth throttling from a VM.
// This is typically called at the start of a new billing period,
// but can also be called manually by admins.
func (s *BandwidthService) RemoveThrottling(ctx context.Context, vmID string) error {
	logger := s.logger.With("vm_id", vmID, "operation", "remove_throttling")

	// Get VM
	vm, err := s.vmRepo.GetByID(ctx, vmID)
	if err != nil {
		return fmt.Errorf("getting VM: %w", err)
	}

	// Check if VM has a node assigned
	if vm.NodeID == nil {
		logger.Debug("VM has no node assigned, updating database only")
	} else {
		// Remove throttle on node agent
		if err := s.throttler.RemoveThrottle(ctx, *vm.NodeID, vmID); err != nil {
			logger.Warn("failed to remove throttle from node agent", "error", err)
			// Continue to update database anyway
		}
	}

	// Update database
	now := time.Now().UTC()
	if err := s.bandwidthRepo.SetThrottled(ctx, vmID, now.Year(), int(now.Month()), false); err != nil {
		return fmt.Errorf("marking VM as unthrottled: %w", err)
	}

	logger.Info("VM throttling removed successfully")
	return nil
}

// ResetMonthlyCounters resets bandwidth counters for all VMs at month end.
// This should be called by a cron job at the start of each month (00:00 UTC on 1st).
func (s *BandwidthService) ResetMonthlyCounters(ctx context.Context) error {
	logger := s.logger.With("operation", "reset_monthly_counters")

	now := time.Now().UTC()
	year, month := now.Year(), int(now.Month())

	logger.Info("starting monthly bandwidth reset", "year", year, "month", month)

	// Get all throttled VMs and unthrottle them
	throttled, err := s.bandwidthRepo.ListThrottled(ctx)
	if err != nil {
		return fmt.Errorf("listing throttled VMs: %w", err)
	}

	for _, usage := range throttled {
		if err := s.RemoveThrottling(ctx, usage.VMID); err != nil {
			logger.Warn("failed to remove throttling during reset", "vm_id", usage.VMID, "error", err)
		}
	}

	logger.Info("monthly bandwidth reset completed", "unthrottled_count", len(throttled))
	return nil
}

// GetThrottledVMs returns a list of all currently throttled VM IDs.
func (s *BandwidthService) GetThrottledVMs(ctx context.Context) ([]string, error) {
	usages, err := s.bandwidthRepo.ListThrottled(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing throttled VMs: %w", err)
	}

	vmIDs := make([]string, 0, len(usages))
	for _, usage := range usages {
		vmIDs = append(vmIDs, usage.VMID)
	}
	return vmIDs, nil
}

// CheckAllVMs checks bandwidth limits for all active VMs and applies throttling
// where necessary. This should be called periodically (e.g., daily).
// Uses batch processing to avoid loading all VMs into memory at once.
func (s *BandwidthService) CheckAllVMs(ctx context.Context) (int, error) {
	logger := s.logger.With("operation", "check_all_vms")

	const batchSize = 100
	throttledCount := 0
	totalChecked := 0

	for page := 1; ; page++ {
		filter := models.VMListFilter{
			Status: util.StringPtr(models.VMStatusRunning),
			PaginationParams: models.PaginationParams{
				Page:    page,
				PerPage: batchSize,
			},
		}

		vms, total, err := s.vmRepo.List(ctx, filter)
		if err != nil {
			return throttledCount, fmt.Errorf("listing running VMs (page %d): %w", page, err)
		}

		if len(vms) == 0 {
			break
		}

		for i := range vms {
			// checkAndThrottle returns whether throttling was applied (F-174).
			// Avoid calling GetMonthlyUsage again to prevent redundant queries.
			throttled, err := s.checkAndThrottle(ctx, &vms[i])
			if err != nil {
				logger.Warn("failed to check VM bandwidth", "vm_id", vms[i].ID, "error", err)
				continue
			}
			if throttled {
				throttledCount++
			}
		}

		totalChecked += len(vms)

		if totalChecked >= total || len(vms) < batchSize {
			break
		}
	}

	logger.Info("bandwidth check completed", "vms_checked", totalChecked, "throttled", throttledCount)
	return throttledCount, nil
}

// checkAndThrottle checks if a VM has exceeded its bandwidth limit and applies
// throttling if necessary. Returns (throttled bool, err error) so that callers
// do not need a second GetMonthlyUsage call to determine throttle status (F-174).
func (s *BandwidthService) checkAndThrottle(ctx context.Context, vm *models.VM) (bool, error) {
	logger := s.logger.With("vm_id", vm.ID, "operation", "check_and_throttle")

	// Skip if no bandwidth limit (unlimited)
	if vm.BandwidthLimitGB == 0 {
		return false, nil
	}

	// Check limit
	exceeded, _, _, err := s.CheckLimit(ctx, vm.ID)
	if err != nil {
		return false, fmt.Errorf("checking bandwidth limit: %w", err)
	}

	if exceeded {
		logger.Info("VM exceeded bandwidth limit, applying throttle")
		if err := s.TriggerThrottling(ctx, vm.ID); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}
