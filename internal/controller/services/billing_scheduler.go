package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// BillingSchedulerConfig holds dependencies for the hourly billing scheduler.
type BillingSchedulerConfig struct {
	LedgerService  *BillingLedgerService
	CheckpointRepo BillingCheckpointRepo
	VMRepo         BillingVMRepo
	PlanRepo       BillingPlanRepo
	DB             SchedulerDB
	Logger         *slog.Logger
}

// BillingCheckpointRepo defines checkpoint persistence for the scheduler.
type BillingCheckpointRepo interface {
	RecordCheckpoint(ctx context.Context, vmID string, chargeHour time.Time,
		amount int64, transactionID *string) error
	ExistsForHour(ctx context.Context, vmID string, chargeHour time.Time) (bool, error)
}

// BillingVMRepo defines VM queries for the scheduler.
type BillingVMRepo interface {
	ListBillableVMs(ctx context.Context) ([]models.VM, error)
}

// BillingPlanRepo defines plan queries for the scheduler.
type BillingPlanRepo interface {
	GetByID(ctx context.Context, id string) (*models.Plan, error)
}

// SchedulerDB exposes the advisory lock capability.
type SchedulerDB interface {
	TryAdvisoryLock(ctx context.Context, lockID int64) (bool, error)
	ReleaseAdvisoryLock(ctx context.Context, lockID int64) error
}

// Advisory lock ID for the hourly billing scheduler.
const billingSchedulerLockID int64 = 0x5649525455455354

// BillingScheduler runs hourly billing deductions for native-billing VMs.
type BillingScheduler struct {
	ledger         *BillingLedgerService
	checkpointRepo BillingCheckpointRepo
	vmRepo         BillingVMRepo
	planRepo       BillingPlanRepo
	db             SchedulerDB
	logger         *slog.Logger
}

// NewBillingScheduler creates a new BillingScheduler.
func NewBillingScheduler(cfg BillingSchedulerConfig) *BillingScheduler {
	return &BillingScheduler{
		ledger:         cfg.LedgerService,
		checkpointRepo: cfg.CheckpointRepo,
		vmRepo:         cfg.VMRepo,
		planRepo:       cfg.PlanRepo,
		db:             cfg.DB,
		logger:         cfg.Logger.With("component", "billing-scheduler"),
	}
}

// Start runs the billing scheduler on a 1-hour interval.
// Only one instance executes per interval using pg_try_advisory_lock.
func (s *BillingScheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	s.logger.Info("billing scheduler started")

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("billing scheduler stopped")
			return
		case <-ticker.C:
			s.RunBillingCycle(ctx)
		}
	}
}

// RunBillingCycle processes one hourly billing cycle. Exported for testing.
func (s *BillingScheduler) RunBillingCycle(ctx context.Context) {
	acquired, err := s.db.TryAdvisoryLock(ctx, billingSchedulerLockID)
	if err != nil {
		s.logger.Error("failed to acquire billing lock", "error", err)
		return
	}
	if !acquired {
		s.logger.Debug("billing lock held by another instance, skipping")
		return
	}
	defer func() {
		if releaseErr := s.db.ReleaseAdvisoryLock(ctx, billingSchedulerLockID); releaseErr != nil {
			s.logger.Warn("failed to release billing lock", "error", releaseErr)
		}
	}()

	chargeHour := time.Now().UTC().Truncate(time.Hour)
	s.logger.Info("running billing cycle", "charge_hour", chargeHour)

	vms, err := s.vmRepo.ListBillableVMs(ctx)
	if err != nil {
		s.logger.Error("failed to list billable VMs", "error", err)
		return
	}

	var charged, skipped, failed int
	for i := range vms {
		didCharge, chargeErr := s.chargeVM(ctx, vms[i], chargeHour)
		if chargeErr != nil {
			s.logger.Error("failed to charge VM",
				"vm_id", vms[i].ID, "error", chargeErr)
			failed++
			continue
		}
		if didCharge {
			charged++
		} else {
			skipped++
		}
	}

	s.checkLowBalances(ctx, vms)

	s.logger.Info("billing cycle completed",
		"charge_hour", chargeHour,
		"charged", charged,
		"skipped", skipped,
		"failed", failed,
		"total", len(vms),
	)
}

func (s *BillingScheduler) chargeVM(
	ctx context.Context, vm models.VM, chargeHour time.Time,
) (bool, error) {
	exists, err := s.checkpointRepo.ExistsForHour(ctx, vm.ID, chargeHour)
	if err != nil {
		return false, fmt.Errorf("check checkpoint: %w", err)
	}
	if exists {
		return false, nil
	}

	if vm.Status != models.VMStatusRunning && vm.Status != models.VMStatusStopped {
		return false, nil
	}

	plan, err := s.planRepo.GetByID(ctx, vm.PlanID)
	if err != nil {
		return false, fmt.Errorf("get plan %s: %w", vm.PlanID, err)
	}

	amount := plan.EffectiveHourlyRate(vm.Status)
	if amount == 0 {
		return false, nil
	}

	refType := models.BillingRefTypeVMUsage
	idempotencyKey := fmt.Sprintf("hourly:%s:%s", vm.ID,
		chargeHour.Format("2006-01-02T15"))

	bt, err := s.ledger.DebitAccount(
		ctx, vm.CustomerID, amount,
		fmt.Sprintf("Hourly charge: %s (%s)", vm.Hostname, plan.Name),
		&refType, &vm.ID, &idempotencyKey,
	)
	if err != nil {
		return false, fmt.Errorf("debit account: %w", err)
	}

	if err := s.checkpointRepo.RecordCheckpoint(
		ctx, vm.ID, chargeHour, amount, &bt.ID,
	); err != nil {
		return false, fmt.Errorf("record checkpoint: %w", err)
	}

	return true, nil
}

// checkLowBalances emits structured log warnings for customers with low balance.
func (s *BillingScheduler) checkLowBalances(ctx context.Context, vms []models.VM) {
	checked := make(map[string]bool)
	for i := range vms {
		cid := vms[i].CustomerID
		if checked[cid] {
			continue
		}
		checked[cid] = true

		balance, err := s.ledger.GetBalance(ctx, cid)
		if err != nil {
			s.logger.Warn("failed to check balance for low-balance alert",
				"customer_id", cid, "error", err)
			continue
		}
		if balance <= 0 {
			s.logger.Warn("customer balance at or below zero",
				"customer_id", cid, "balance", balance)
		}
	}
}
