package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBillingCheckpointRepo struct {
	recordCheckpointFunc func(ctx context.Context, vmID string, chargeHour time.Time, amount int64, transactionID *string) error
	existsForHourFunc    func(ctx context.Context, vmID string, chargeHour time.Time) (bool, error)
}

func (m *mockBillingCheckpointRepo) RecordCheckpoint(ctx context.Context, vmID string, chargeHour time.Time, amount int64, transactionID *string) error {
	return m.recordCheckpointFunc(ctx, vmID, chargeHour, amount, transactionID)
}

func (m *mockBillingCheckpointRepo) ExistsForHour(ctx context.Context, vmID string, chargeHour time.Time) (bool, error) {
	return m.existsForHourFunc(ctx, vmID, chargeHour)
}

type mockBillingVMRepo struct {
	listBillableVMsFunc func(ctx context.Context) ([]models.VM, error)
}

func (m *mockBillingVMRepo) ListBillableVMs(ctx context.Context) ([]models.VM, error) {
	return m.listBillableVMsFunc(ctx)
}

type mockBillingPlanRepo struct {
	getByIDFunc func(ctx context.Context, id string) (*models.Plan, error)
}

func (m *mockBillingPlanRepo) GetByID(ctx context.Context, id string) (*models.Plan, error) {
	return m.getByIDFunc(ctx, id)
}

type mockSchedulerDB struct {
	tryAdvisoryLockFunc    func(ctx context.Context, lockID int64) (bool, error)
	releaseAdvisoryLockFunc func(ctx context.Context, lockID int64) error
}

func (m *mockSchedulerDB) TryAdvisoryLock(ctx context.Context, lockID int64) (bool, error) {
	return m.tryAdvisoryLockFunc(ctx, lockID)
}

func (m *mockSchedulerDB) ReleaseAdvisoryLock(ctx context.Context, lockID int64) error {
	return m.releaseAdvisoryLockFunc(ctx, lockID)
}

func ptrInt64(v int64) *int64 { return &v }
func ptrStr(v string) *string { return &v }

func newTestScheduler(
	checkpointRepo *mockBillingCheckpointRepo,
	vmRepo *mockBillingVMRepo,
	planRepo *mockBillingPlanRepo,
	db *mockSchedulerDB,
) *BillingScheduler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	txRepo := &mockBillingTransactionRepo{
		debitAccountFunc: func(_ context.Context, _ string, amount int64, _ string, _, _, _ *string) (*models.BillingTransaction, error) {
			return &models.BillingTransaction{
				ID:           "tx-auto",
				Amount:       amount,
				BalanceAfter: 9000,
			}, nil
		},
		getBalanceFunc: func(_ context.Context, _ string) (int64, error) {
			return 9000, nil
		},
	}
	ledger := NewBillingLedgerService(BillingLedgerServiceConfig{
		TransactionRepo: txRepo,
		Logger:          logger,
	})
	return NewBillingScheduler(BillingSchedulerConfig{
		LedgerService:  ledger,
		CheckpointRepo: checkpointRepo,
		VMRepo:         vmRepo,
		PlanRepo:       planRepo,
		DB:             db,
		Logger:         logger,
	})
}

func TestBillingScheduler_chargeVM(t *testing.T) {
	chargeHour := time.Now().UTC().Truncate(time.Hour)

	tests := []struct {
		name           string
		vm             models.VM
		setupCheckpoint func() *mockBillingCheckpointRepo
		setupPlan      func() *mockBillingPlanRepo
		wantErr        bool
	}{
		{
			name: "running VM charges at hourly rate",
			vm: models.VM{
				ID:         "vm-1",
				CustomerID: "cust-1",
				PlanID:     "plan-1",
				Hostname:   "test-vm",
				Status:     models.VMStatusRunning,
			},
			setupCheckpoint: func() *mockBillingCheckpointRepo {
				return &mockBillingCheckpointRepo{
					existsForHourFunc: func(_ context.Context, _ string, _ time.Time) (bool, error) {
						return false, nil
					},
					recordCheckpointFunc: func(_ context.Context, _ string, _ time.Time, _ int64, _ *string) error {
						return nil
					},
				}
			},
			setupPlan: func() *mockBillingPlanRepo {
				return &mockBillingPlanRepo{
					getByIDFunc: func(_ context.Context, _ string) (*models.Plan, error) {
						return &models.Plan{
							ID:         "plan-1",
							Name:       "Basic",
							PriceHourly: ptrInt64(100),
						}, nil
					},
				}
			},
			wantErr: false,
		},
		{
			name: "stopped VM with PriceHourlyStopped",
			vm: models.VM{
				ID:         "vm-2",
				CustomerID: "cust-1",
				PlanID:     "plan-2",
				Hostname:   "stopped-vm",
				Status:     models.VMStatusStopped,
			},
			setupCheckpoint: func() *mockBillingCheckpointRepo {
				return &mockBillingCheckpointRepo{
					existsForHourFunc: func(_ context.Context, _ string, _ time.Time) (bool, error) {
						return false, nil
					},
					recordCheckpointFunc: func(_ context.Context, _ string, _ time.Time, _ int64, _ *string) error {
						return nil
					},
				}
			},
			setupPlan: func() *mockBillingPlanRepo {
				return &mockBillingPlanRepo{
					getByIDFunc: func(_ context.Context, _ string) (*models.Plan, error) {
						return &models.Plan{
							ID:                 "plan-2",
							Name:               "Basic",
							PriceHourly:        ptrInt64(100),
							PriceHourlyStopped: ptrInt64(25),
						}, nil
					},
				}
			},
			wantErr: false,
		},
		{
			name: "checkpoint exists skips idempotent",
			vm: models.VM{
				ID:         "vm-3",
				CustomerID: "cust-1",
				PlanID:     "plan-1",
				Hostname:   "already-charged",
				Status:     models.VMStatusRunning,
			},
			setupCheckpoint: func() *mockBillingCheckpointRepo {
				return &mockBillingCheckpointRepo{
					existsForHourFunc: func(_ context.Context, _ string, _ time.Time) (bool, error) {
						return true, nil
					},
				}
			},
			setupPlan: func() *mockBillingPlanRepo {
				return &mockBillingPlanRepo{}
			},
			wantErr: false,
		},
		{
			name: "VM in non-billable status skips",
			vm: models.VM{
				ID:         "vm-4",
				CustomerID: "cust-1",
				PlanID:     "plan-1",
				Hostname:   "migrating-vm",
				Status:     models.VMStatusMigrating,
			},
			setupCheckpoint: func() *mockBillingCheckpointRepo {
				return &mockBillingCheckpointRepo{
					existsForHourFunc: func(_ context.Context, _ string, _ time.Time) (bool, error) {
						return false, nil
					},
				}
			},
			setupPlan: func() *mockBillingPlanRepo {
				return &mockBillingPlanRepo{}
			},
			wantErr: false,
		},
		{
			name: "plan with nil PriceHourly skips",
			vm: models.VM{
				ID:         "vm-5",
				CustomerID: "cust-1",
				PlanID:     "plan-ext",
				Hostname:   "external-vm",
				Status:     models.VMStatusRunning,
			},
			setupCheckpoint: func() *mockBillingCheckpointRepo {
				return &mockBillingCheckpointRepo{
					existsForHourFunc: func(_ context.Context, _ string, _ time.Time) (bool, error) {
						return false, nil
					},
				}
			},
			setupPlan: func() *mockBillingPlanRepo {
				return &mockBillingPlanRepo{
					getByIDFunc: func(_ context.Context, _ string) (*models.Plan, error) {
						return &models.Plan{
							ID:          "plan-ext",
							Name:        "WHMCS Plan",
							PriceHourly: nil,
						}, nil
					},
				}
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheduler := newTestScheduler(
				tt.setupCheckpoint(),
				&mockBillingVMRepo{},
				tt.setupPlan(),
				&mockSchedulerDB{},
			)
			_, err := scheduler.chargeVM(context.Background(), tt.vm, chargeHour)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestBillingScheduler_RunBillingCycle(t *testing.T) {
	t.Run("advisory lock not acquired skips", func(t *testing.T) {
		lockAcquired := false
		scheduler := newTestScheduler(
			&mockBillingCheckpointRepo{},
			&mockBillingVMRepo{
				listBillableVMsFunc: func(_ context.Context) ([]models.VM, error) {
					lockAcquired = true
					return nil, errors.New("should not be called")
				},
			},
			&mockBillingPlanRepo{},
			&mockSchedulerDB{
				tryAdvisoryLockFunc: func(_ context.Context, _ int64) (bool, error) {
					return false, nil
				},
				releaseAdvisoryLockFunc: func(_ context.Context, _ int64) error {
					return nil
				},
			},
		)

		scheduler.RunBillingCycle(context.Background())
		assert.False(t, lockAcquired, "should not list VMs when lock not acquired")
	})

	t.Run("no billable VMs completes", func(t *testing.T) {
		released := false
		scheduler := newTestScheduler(
			&mockBillingCheckpointRepo{},
			&mockBillingVMRepo{
				listBillableVMsFunc: func(_ context.Context) ([]models.VM, error) {
					return []models.VM{}, nil
				},
			},
			&mockBillingPlanRepo{},
			&mockSchedulerDB{
				tryAdvisoryLockFunc: func(_ context.Context, _ int64) (bool, error) {
					return true, nil
				},
				releaseAdvisoryLockFunc: func(_ context.Context, _ int64) error {
					released = true
					return nil
				},
			},
		)

		scheduler.RunBillingCycle(context.Background())
		assert.True(t, released, "advisory lock should be released")
	})
}
