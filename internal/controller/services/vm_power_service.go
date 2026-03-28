// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

func (s *VMService) StartVM(ctx context.Context, vmID, customerID string, isAdmin bool) error {
	// Get VM and verify ownership
	vm, err := s.getVMAndVerifyOwnership(ctx, vmID, customerID, isAdmin)
	if err != nil {
		return fmt.Errorf("verifying ownership for start: %w", err)
	}

	// Verify status allows starting
	if vm.Status != models.VMStatusStopped && vm.Status != models.VMStatusSuspended {
		return fmt.Errorf("cannot start VM in status %s", vm.Status)
	}

	// Check if node is assigned
	if vm.NodeID == nil {
		return fmt.Errorf("VM has no node assigned")
	}

	// Call node agent to start VM
	if err := s.nodeAgent.StartVM(ctx, *vm.NodeID, vm.ID); err != nil {
		return fmt.Errorf("starting VM on node agent: %w", err)
	}

	// Update status
	if err := s.vmRepo.TransitionStatus(ctx, vm.ID, vm.Status, models.VMStatusRunning); err != nil {
		if errors.Is(err, sharederrors.ErrConflict) {
			return fmt.Errorf("transitioning VM %s from %s to running: %w", vm.ID, vm.Status, err)
		}
		s.logger.Error("failed to transition VM status after start - status mismatch may occur",
			"vm_id", vm.ID, "from_status", vm.Status, "error", err)
	}

	s.logger.Info("VM started", "vm_id", vm.ID, "customer_id", customerID)
	return nil
}

func (s *VMService) StopVM(ctx context.Context, vmID, customerID string, force bool, isAdmin bool) error {
	// Get VM and verify ownership
	vm, err := s.getVMAndVerifyOwnership(ctx, vmID, customerID, isAdmin)
	if err != nil {
		return fmt.Errorf("verifying ownership for stop: %w", err)
	}

	// Verify status allows stopping
	if vm.Status != models.VMStatusRunning {
		return fmt.Errorf("cannot stop VM in status %s", vm.Status)
	}

	// Check if node is assigned
	if vm.NodeID == nil {
		return fmt.Errorf("VM has no node assigned")
	}

	// Call appropriate stop method
	if force {
		if err := s.nodeAgent.ForceStopVM(ctx, *vm.NodeID, vm.ID); err != nil {
			return fmt.Errorf("force stopping VM on node agent: %w", err)
		}
	} else {
		// Graceful shutdown with 120 second timeout
		if err := s.nodeAgent.StopVM(ctx, *vm.NodeID, vm.ID, 120); err != nil {
			return fmt.Errorf("stopping VM on node agent: %w", err)
		}
	}

	// Update status
	if err := s.vmRepo.TransitionStatus(ctx, vm.ID, models.VMStatusRunning, models.VMStatusStopped); err != nil {
		if errors.Is(err, sharederrors.ErrConflict) {
			return fmt.Errorf("transitioning VM %s from running to stopped: %w", vm.ID, err)
		}
		s.logger.Error("failed to transition VM status after stop - status mismatch may occur",
			"vm_id", vm.ID, "error", err)
	}

	s.logger.Info("VM stopped", "vm_id", vm.ID, "force", force, "customer_id", customerID)
	return nil
}

func (s *VMService) RestartVM(ctx context.Context, vmID, customerID string, isAdmin bool) error {
	// Get VM and verify ownership
	vm, err := s.getVMAndVerifyOwnership(ctx, vmID, customerID, isAdmin)
	if err != nil {
		return fmt.Errorf("verifying ownership for restart: %w", err)
	}

	// Verify status allows restart
	if vm.Status != models.VMStatusRunning {
		return fmt.Errorf("cannot restart VM in status %s", vm.Status)
	}

	// Check if node is assigned
	if vm.NodeID == nil {
		return fmt.Errorf("VM has no node assigned")
	}

	// Graceful shutdown with 60 second timeout
	if err := s.nodeAgent.StopVM(ctx, *vm.NodeID, vm.ID, 60); err != nil {
		s.logger.Warn("graceful stop failed during restart, attempting force stop", "vm_id", vm.ID, "error", err)
		if err := s.nodeAgent.ForceStopVM(ctx, *vm.NodeID, vm.ID); err != nil {
			return fmt.Errorf("force stopping VM during restart: %w", err)
		}
	}
	if err := s.vmRepo.TransitionStatus(ctx, vm.ID, models.VMStatusRunning, models.VMStatusStopped); err != nil {
		if errors.Is(err, sharederrors.ErrConflict) {
			return fmt.Errorf("transitioning VM %s from running to stopped during restart: %w", vm.ID, err)
		}
		s.logger.Warn("failed to transition VM status to stopped during restart", "vm_id", vm.ID, "error", err)
	}

	// Start the VM
	if err := s.nodeAgent.StartVM(ctx, *vm.NodeID, vm.ID); err != nil {
		return fmt.Errorf("starting VM during restart: %w", err)
	}

	// Update status to running
	if err := s.vmRepo.TransitionStatus(ctx, vm.ID, models.VMStatusStopped, models.VMStatusRunning); err != nil {
		if errors.Is(err, sharederrors.ErrConflict) {
			return fmt.Errorf("transitioning VM %s from stopped to running after restart: %w", vm.ID, err)
		}
		s.logger.Error("failed to transition VM status after restart - status mismatch may occur",
			"vm_id", vm.ID, "error", err)
	}

	s.logger.Info("VM restarted", "vm_id", vm.ID, "customer_id", customerID)
	return nil
}
