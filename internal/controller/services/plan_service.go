// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// PlanRepository defines the interface for plan repository operations.
type PlanRepository interface {
	Create(ctx context.Context, plan *models.Plan) error
	GetByID(ctx context.Context, id string) (*models.Plan, error)
	GetBySlug(ctx context.Context, slug string) (*models.Plan, error)
	List(ctx context.Context, filter repository.PlanListFilter) ([]models.Plan, bool, string, error)
	ListActive(ctx context.Context) ([]models.Plan, error)
	Update(ctx context.Context, plan *models.Plan) error
	Delete(ctx context.Context, id string) error
	CountVMsByPlan(ctx context.Context, planID string) (int, error)
}

// PlanService provides business logic for managing service plans.
// Plans define the resource allocations (vCPU, memory, disk, bandwidth)
// and pricing for VPS offerings.
type PlanService struct {
	planRepo PlanRepository
	logger   *slog.Logger
}

// NewPlanService creates a new PlanService with the given dependencies.
func NewPlanService(planRepo PlanRepository, logger *slog.Logger) *PlanService {
	return &PlanService{
		planRepo: planRepo,
		logger:   logger.With("component", "plan-service"),
	}
}

// ListActive returns all active plans ordered by sort_order.
// This is the primary method for displaying plans to customers.
func (s *PlanService) ListActive(ctx context.Context) ([]models.Plan, error) {
	plans, err := s.planRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing active plans: %w", err)
	}
	return plans, nil
}

// List returns a paginated list of plans with optional filtering.
// Supports filtering by active status and pagination.
func (s *PlanService) List(ctx context.Context, filter repository.PlanListFilter) ([]models.Plan, bool, string, error) {
	plans, hasMore, lastID, err := s.planRepo.List(ctx, filter)
	if err != nil {
		return nil, false, "", fmt.Errorf("listing plans: %w", err)
	}
	return plans, hasMore, lastID, nil
}

// GetByID retrieves a plan by its UUID.
// Returns ErrNotFound if the plan doesn't exist.
func (s *PlanService) GetByID(ctx context.Context, id string) (*models.Plan, error) {
	plan, err := s.planRepo.GetByID(ctx, id)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return nil, fmt.Errorf("plan not found: %s: %w", id, sharederrors.ErrNotFound)
		}
		return nil, fmt.Errorf("getting plan: %w", err)
	}
	return plan, nil
}

// Create creates a new service plan.
// The plan's ID, CreatedAt, and UpdatedAt are populated by the database.
func (s *PlanService) Create(ctx context.Context, req *models.PlanCreateRequest) (*models.Plan, error) {
	// Check if slug already exists
	existing, err := s.planRepo.GetBySlug(ctx, req.Slug)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("plan with slug %s already exists", req.Slug)
	}

	plan := &models.Plan{
		Name:               req.Name,
		Slug:               req.Slug,
		VCPU:               req.VCPU,
		MemoryMB:           req.MemoryMB,
		DiskGB:             req.DiskGB,
		BandwidthLimitGB:   req.BandwidthLimitGB,
		PortSpeedMbps:      req.PortSpeedMbps,
		PriceMonthly:       req.PriceMonthly,
		PriceHourly:        req.PriceHourly,
		PriceHourlyStopped: req.PriceHourlyStopped,
		IsActive:           req.IsActive,
		SortOrder:          req.SortOrder,
		SnapshotLimit:      req.SnapshotLimit,
		BackupLimit:        req.BackupLimit,
		ISOUploadLimit:     req.ISOUploadLimit,
	}

	if req.Currency != "" {
		plan.Currency = req.Currency
	} else {
		plan.Currency = "USD"
	}

	if req.SnapshotLimit <= 0 {
		plan.SnapshotLimit = 2
	}
	if req.BackupLimit <= 0 {
		plan.BackupLimit = 2
	}
	if req.ISOUploadLimit <= 0 {
		plan.ISOUploadLimit = 2
	}

	if req.StorageBackend != "" {
		plan.StorageBackend = req.StorageBackend
	} else {
		plan.StorageBackend = models.DefaultStorageBackend
	}

	if err := s.planRepo.Create(ctx, plan); err != nil {
		return nil, fmt.Errorf("creating plan: %w", err)
	}

	s.logger.Info("plan created",
		"plan_id", plan.ID,
		"name", plan.Name,
		"slug", plan.Slug,
		"vcpu", plan.VCPU,
		"memory_mb", plan.MemoryMB,
		"disk_gb", plan.DiskGB)

	return plan, nil
}

// Update updates an existing plan's details.
// Only the provided fields in the update request are modified.
func (s *PlanService) Update(ctx context.Context, plan *models.Plan) error {
	// Verify plan exists
	existing, err := s.planRepo.GetByID(ctx, plan.ID)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("plan not found: %s: %w", plan.ID, sharederrors.ErrNotFound)
		}
		return fmt.Errorf("getting plan: %w", err)
	}

	// If slug is being changed, check for conflicts
	if plan.Slug != existing.Slug {
		conflict, err := s.planRepo.GetBySlug(ctx, plan.Slug)
		if err == nil && conflict != nil && conflict.ID != plan.ID {
			return fmt.Errorf("plan with slug %s already exists", plan.Slug)
		}
	}

	if err := s.planRepo.Update(ctx, plan); err != nil {
		return fmt.Errorf("updating plan: %w", err)
	}

	s.logger.Info("plan updated",
		"plan_id", plan.ID,
		"name", plan.Name)

	return nil
}

// Delete removes a plan from the system.
// Note: Plans with referencing VMs cannot be deleted due to FK constraints.
func (s *PlanService) Delete(ctx context.Context, id string) error {
	// Verify plan exists
	_, err := s.planRepo.GetByID(ctx, id)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return fmt.Errorf("plan not found: %s: %w", id, sharederrors.ErrNotFound)
		}
		return fmt.Errorf("getting plan: %w", err)
	}

	if err := s.planRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("deleting plan: %w", err)
	}

	s.logger.Info("plan deleted", "plan_id", id)
	return nil
}

// GetPlanUsage returns the count of VMs using a specific plan.
// This is useful for determining if a plan can be safely deleted.
func (s *PlanService) GetPlanUsage(ctx context.Context, id string) (int, error) {
	// Verify plan exists
	_, err := s.planRepo.GetByID(ctx, id)
	if err != nil {
		if sharederrors.Is(err, sharederrors.ErrNotFound) {
			return 0, fmt.Errorf("plan not found: %s: %w", id, sharederrors.ErrNotFound)
		}
		return 0, fmt.Errorf("getting plan: %w", err)
	}

	count, err := s.planRepo.CountVMsByPlan(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("getting plan usage: %w", err)
	}

	return count, nil
}
