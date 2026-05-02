package tasks

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/billing"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeVMDeleteRepo struct {
	vm            *models.VM
	getErr        error
	softDeleteErr error
	softDeleted   bool
}

func (r *fakeVMDeleteRepo) GetByIDIncludingDeleted(_ context.Context, _ string) (*models.VM, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.vm, nil
}

func (r *fakeVMDeleteRepo) SoftDelete(_ context.Context, _ string) error {
	if r.softDeleteErr != nil {
		return r.softDeleteErr
	}
	r.softDeleted = true
	return nil
}

type fakeVMDeleteTaskRepo struct {
	completed bool
	result    []byte
}

func (r *fakeVMDeleteTaskRepo) UpdateProgress(_ context.Context, _ string, _ int, _ string) error {
	return nil
}

func (r *fakeVMDeleteTaskRepo) SetCompleted(_ context.Context, _ string, result []byte) error {
	r.completed = true
	r.result = result
	return nil
}

type fakeVMDeleteNodeClient struct {
	deleteErr   error
	deleteCalls int
	stopCalls   int
	forceCalls  int
}

func (c *fakeVMDeleteNodeClient) StopVM(_ context.Context, _, _ string, _ int) error {
	c.stopCalls++
	return nil
}

func (c *fakeVMDeleteNodeClient) ForceStopVM(_ context.Context, _, _ string) error {
	c.forceCalls++
	return nil
}

func (c *fakeVMDeleteNodeClient) DeleteVM(_ context.Context, _, _ string) error {
	c.deleteCalls++
	return c.deleteErr
}

type fakeVMDeleteIPAM struct {
	releaseErr   error
	releaseCalls int
}

func (i *fakeVMDeleteIPAM) ReleaseIPsByVM(_ context.Context, _ string) error {
	i.releaseCalls++
	return i.releaseErr
}

type fakeVMDeleteCustomerRepo struct {
	customer *models.Customer
}

func (r *fakeVMDeleteCustomerRepo) GetByID(_ context.Context, _ string) (*models.Customer, error) {
	return r.customer, nil
}

type fakeVMDeleteBillingResolver struct {
	hook *fakeVMDeleteBillingHook
}

func (r fakeVMDeleteBillingResolver) ForCustomer(_ string) (billing.VMLifecycleHook, error) {
	return r.hook, nil
}

type fakeVMDeleteBillingHook struct {
	deletedCalls int
}

func (h *fakeVMDeleteBillingHook) OnVMCreated(context.Context, billing.VMRef) error {
	return nil
}

func (h *fakeVMDeleteBillingHook) OnVMDeleted(context.Context, billing.VMRef) error {
	h.deletedCalls++
	return nil
}

func (h *fakeVMDeleteBillingHook) OnVMResized(context.Context, billing.VMRef, string, string) error {
	return nil
}

func TestVMDeletionExecute(t *testing.T) {
	nodeID := "node-1"
	deletedAt := time.Now()
	customer := &models.Customer{ID: "customer-1"}

	tests := []struct {
		name              string
		vm                *models.VM
		getErr            error
		nodeDeleteErr     error
		ipReleaseErr      error
		wantErr           bool
		wantCompleted     bool
		wantSoftDeleted   bool
		wantNodeDelete    int
		wantIPRelease     int
		wantBillingDelete int
	}{
		{
			name: "deletes stopped VM after cleanup",
			vm: &models.VM{
				ID: "vm-1", CustomerID: customer.ID, PlanID: "plan-1",
				NodeID: &nodeID, Status: models.VMStatusDeleting,
			},
			wantCompleted: true, wantSoftDeleted: true, wantNodeDelete: 1,
			wantIPRelease: 1, wantBillingDelete: 1,
		},
		{
			name: "missing node resource is idempotent success",
			vm: &models.VM{
				ID: "vm-1", CustomerID: customer.ID, PlanID: "plan-1",
				NodeID: &nodeID, Status: models.VMStatusDeleting,
			},
			nodeDeleteErr:     status.Error(codes.NotFound, "domain not found"),
			wantCompleted:     true,
			wantSoftDeleted:   true,
			wantNodeDelete:    1,
			wantIPRelease:     1,
			wantBillingDelete: 1,
		},
		{
			name: "cleanup failure leaves VM deleting for retry",
			vm: &models.VM{
				ID: "vm-1", CustomerID: customer.ID, PlanID: "plan-1",
				NodeID: &nodeID, Status: models.VMStatusDeleting,
			},
			nodeDeleteErr:  errors.New("node offline"),
			wantErr:        true,
			wantNodeDelete: 1,
		},
		{
			name:          "missing VM record completes idempotently",
			getErr:        sharederrors.ErrNotFound,
			wantCompleted: true,
		},
		{
			name: "pre-soft-deleted VM still runs cleanup",
			vm: &models.VM{
				ID: "vm-1", CustomerID: customer.ID, PlanID: "plan-1",
				NodeID: &nodeID, Status: models.VMStatusDeleted,
				SoftDelete: models.SoftDelete{DeletedAt: &deletedAt},
			},
			wantCompleted:  true,
			wantNodeDelete: 1,
			wantIPRelease:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vmRepo := &fakeVMDeleteRepo{vm: tt.vm, getErr: tt.getErr}
			taskRepo := &fakeVMDeleteTaskRepo{}
			nodeClient := &fakeVMDeleteNodeClient{deleteErr: tt.nodeDeleteErr}
			ipam := &fakeVMDeleteIPAM{releaseErr: tt.ipReleaseErr}
			hook := &fakeVMDeleteBillingHook{}
			deletion := vmDeletion{
				vmRepo:       vmRepo,
				taskRepo:     taskRepo,
				customerRepo: &fakeVMDeleteCustomerRepo{customer: customer},
				ipam:         ipam,
				nodeClient:   nodeClient,
				billingHooks: fakeVMDeleteBillingResolver{hook: hook},
				logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
			}

			err := deletion.execute(context.Background(), "task-1", VMDeletePayload{VMID: "vm-1"})

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantCompleted, taskRepo.completed)
			assert.Equal(t, tt.wantSoftDeleted, vmRepo.softDeleted)
			assert.Equal(t, tt.wantNodeDelete, nodeClient.deleteCalls)
			assert.Equal(t, tt.wantIPRelease, ipam.releaseCalls)
			assert.Equal(t, tt.wantBillingDelete, hook.deletedCalls)
		})
	}
}
