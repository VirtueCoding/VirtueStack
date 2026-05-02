// Package tasks provides the VM deletion task handler.
package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/AbuGosok/VirtueStack/internal/controller/billing"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type vmDeletionVMRepo interface {
	GetByIDIncludingDeleted(ctx context.Context, id string) (*models.VM, error)
	SoftDelete(ctx context.Context, id string) error
}

type vmDeletionTaskRepo interface {
	UpdateProgress(ctx context.Context, id string, progress int, message string) error
	SetCompleted(ctx context.Context, id string, result []byte) error
}

type vmDeletionCustomerRepo interface {
	GetByID(ctx context.Context, id string) (*models.Customer, error)
}

type vmDeletionNodeClient interface {
	StopVM(ctx context.Context, nodeID, vmID string, timeoutSec int) error
	ForceStopVM(ctx context.Context, nodeID, vmID string) error
	DeleteVM(ctx context.Context, nodeID, vmID string) error
}

type vmDeletionIPAM interface {
	ReleaseIPsByVM(ctx context.Context, vmID string) error
}

type vmDeletion struct {
	vmRepo       vmDeletionVMRepo
	taskRepo     vmDeletionTaskRepo
	customerRepo vmDeletionCustomerRepo
	ipam         vmDeletionIPAM
	nodeClient   vmDeletionNodeClient
	billingHooks BillingHookResolver
	logger       *slog.Logger
}

func handleVMDelete(ctx context.Context, task *models.Task, deps *HandlerDeps) error {
	var payload VMDeletePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("parsing vm.delete payload: %w", err)
	}
	deletion := vmDeletion{
		vmRepo:       deps.VMRepo,
		taskRepo:     deps.TaskRepo,
		customerRepo: deps.CustomerRepo,
		ipam:         deps.IPAMService,
		nodeClient:   deps.NodeClient,
		billingHooks: deps.BillingHooks,
		logger:       taskLogger(deps.Logger, task),
	}
	return deletion.execute(ctx, task.ID, payload)
}

func (d vmDeletion) execute(ctx context.Context, taskID string, payload VMDeletePayload) error {
	d.progress(ctx, taskID, 5, "Starting VM deletion...")
	vm, err := d.vmRepo.GetByIDIncludingDeleted(ctx, payload.VMID)
	if err != nil {
		return d.completeIfRecordMissing(ctx, taskID, payload.VMID, err)
	}
	wasDeleted := vm.IsDeleted()
	if err := d.cleanupNode(ctx, taskID, vm); err != nil {
		return err
	}
	d.progress(ctx, taskID, 70, "Releasing IP addresses...")
	if err := d.releaseIPs(ctx, vm.ID); err != nil {
		return err
	}
	d.progress(ctx, taskID, 90, "Removing VM record...")
	if !wasDeleted {
		if err := d.vmRepo.SoftDelete(ctx, vm.ID); err != nil {
			return fmt.Errorf("soft deleting VM %s: %w", vm.ID, err)
		}
		d.notifyBilling(ctx, vm)
	}
	d.progress(ctx, taskID, 100, "VM deleted successfully")
	return d.complete(ctx, taskID, vm.ID)
}

func (d vmDeletion) cleanupNode(ctx context.Context, taskID string, vm *models.VM) error {
	if vm.NodeID == nil {
		return nil
	}
	nodeID := *vm.NodeID
	d.progress(ctx, taskID, 15, "Stopping virtual machine...")
	if vm.Status == models.VMStatusRunning {
		if err := d.stopVM(ctx, nodeID, vm.ID); err != nil {
			d.logger.Warn("failed to stop VM before deletion, continuing", "error", err)
		}
	}
	d.progress(ctx, taskID, 40, "Deleting virtual machine from node...")
	if err := d.nodeClient.DeleteVM(ctx, nodeID, vm.ID); err != nil && !isAlreadyAbsent(err) {
		return fmt.Errorf("deleting VM %s from node %s: %w", vm.ID, nodeID, err)
	}
	return nil
}

func (d vmDeletion) stopVM(ctx context.Context, nodeID, vmID string) error {
	if err := d.nodeClient.StopVM(ctx, nodeID, vmID, 60); err != nil {
		d.logger.Warn("graceful stop failed, attempting force stop", "vm_id", vmID, "error", err)
		if forceErr := d.nodeClient.ForceStopVM(ctx, nodeID, vmID); forceErr != nil {
			return fmt.Errorf("force stopping VM %s: %w", vmID, forceErr)
		}
	}
	return nil
}

func (d vmDeletion) releaseIPs(ctx context.Context, vmID string) error {
	if d.ipam == nil {
		return nil
	}
	if err := d.ipam.ReleaseIPsByVM(ctx, vmID); err != nil && !isAlreadyAbsent(err) {
		return fmt.Errorf("releasing IPs for VM %s: %w", vmID, err)
	}
	return nil
}

func (d vmDeletion) completeIfRecordMissing(ctx context.Context, taskID, vmID string, err error) error {
	if !isAlreadyAbsent(err) {
		return fmt.Errorf("getting VM %s for deletion: %w", vmID, err)
	}
	d.logger.Info("VM already absent during deletion")
	return d.complete(ctx, taskID, vmID)
}

func (d vmDeletion) complete(ctx context.Context, taskID, vmID string) error {
	result, err := json.Marshal(map[string]any{"vm_id": vmID, "status": "deleted"})
	if err != nil {
		return fmt.Errorf("marshaling vm.delete result: %w", err)
	}
	if err := d.taskRepo.SetCompleted(ctx, taskID, result); err != nil {
		return fmt.Errorf("completing vm.delete task: %w", err)
	}
	return nil
}

func (d vmDeletion) progress(ctx context.Context, taskID string, progress int, message string) {
	if err := d.taskRepo.UpdateProgress(ctx, taskID, progress, message); err != nil {
		d.logger.Warn("failed to update task progress", "error", err)
	}
}

func (d vmDeletion) notifyBilling(ctx context.Context, vm *models.VM) {
	if d.billingHooks == nil || d.customerRepo == nil {
		return
	}
	hook, err := d.billingHook(ctx, vm.CustomerID)
	if err != nil {
		d.logger.Warn("billing hook: provider not found", "customer_id", vm.CustomerID, "error", err)
		return
	}
	if err := hook.OnVMDeleted(ctx, billing.VMRef{
		ID: vm.ID, CustomerID: vm.CustomerID, PlanID: vm.PlanID, Hostname: vm.Hostname,
	}); err != nil {
		d.logger.Warn("billing hook: callback failed", "customer_id", vm.CustomerID, "error", err)
	}
}

func (d vmDeletion) billingHook(ctx context.Context, customerID string) (billing.VMLifecycleHook, error) {
	customer, err := d.customerRepo.GetByID(ctx, customerID)
	if err != nil {
		return nil, fmt.Errorf("getting customer %s: %w", customerID, err)
	}
	provider := ""
	if customer.BillingProvider != nil {
		provider = *customer.BillingProvider
	}
	return d.billingHooks.ForCustomer(provider)
}

func isAlreadyAbsent(err error) bool {
	if err == nil {
		return false
	}
	if sharederrors.Is(err, sharederrors.ErrNotFound) || errors.Is(err, sharederrors.ErrNotFound) {
		return true
	}
	if status.Code(err) == codes.NotFound {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}
