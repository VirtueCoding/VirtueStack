package services

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockPlanRepository is a mock implementation of PlanRepository for testing.
type MockPlanRepository struct {
	Plans map[string]*models.Plan
	Slugs map[string]string // slug -> id mapping
}

func NewMockPlanRepository() *MockPlanRepository {
	return &MockPlanRepository{
		Plans: make(map[string]*models.Plan),
		Slugs: make(map[string]string),
	}
}

func (m *MockPlanRepository) Create(ctx context.Context, plan *models.Plan) error {
	if _, exists := m.Plans[plan.ID]; exists {
		return errors.New("plan already exists")
	}
	// Generate UUID if not set (simulates database behavior)
	if plan.ID == "" {
		plan.ID = uuid.New().String()
	}
	m.Plans[plan.ID] = plan
	m.Slugs[plan.Slug] = plan.ID
	return nil
}

func (m *MockPlanRepository) GetByID(ctx context.Context, id string) (*models.Plan, error) {
	plan, exists := m.Plans[id]
	if !exists {
		return nil, sharederrors.ErrNotFound
	}
	return plan, nil
}

func (m *MockPlanRepository) GetBySlug(ctx context.Context, slug string) (*models.Plan, error) {
	id, exists := m.Slugs[slug]
	if !exists {
		return nil, sharederrors.ErrNotFound
	}
	return m.Plans[id], nil
}

func (m *MockPlanRepository) GetByName(ctx context.Context, name string) (*models.Plan, error) {
	for _, plan := range m.Plans {
		if plan.Name == name {
			return plan, nil
		}
	}
	return nil, sharederrors.ErrNotFound
}

func (m *MockPlanRepository) List(ctx context.Context, filter repository.PlanListFilter) ([]models.Plan, int, error) {
	var plans []models.Plan
	for _, plan := range m.Plans {
		if filter.IsActive != nil && plan.IsActive != *filter.IsActive {
			continue
		}
		plans = append(plans, *plan)
	}
	return plans, len(plans), nil
}

func (m *MockPlanRepository) ListActive(ctx context.Context) ([]models.Plan, error) {
	var plans []models.Plan
	for _, plan := range m.Plans {
		if plan.IsActive {
			plans = append(plans, *plan)
		}
	}
	return plans, nil
}

func (m *MockPlanRepository) Update(ctx context.Context, plan *models.Plan) error {
	if _, exists := m.Plans[plan.ID]; !exists {
		return sharederrors.ErrNotFound
	}
	// Update slug mapping if changed
	oldPlan := m.Plans[plan.ID]
	if oldPlan.Slug != plan.Slug {
		delete(m.Slugs, oldPlan.Slug)
		m.Slugs[plan.Slug] = plan.ID
	}
	m.Plans[plan.ID] = plan
	return nil
}

func (m *MockPlanRepository) UpdateActive(ctx context.Context, id string, isActive bool) error {
	plan, exists := m.Plans[id]
	if !exists {
		return sharederrors.ErrNotFound
	}
	plan.IsActive = isActive
	return nil
}

func (m *MockPlanRepository) UpdateSortOrder(ctx context.Context, id string, sortOrder int) error {
	plan, exists := m.Plans[id]
	if !exists {
		return sharederrors.ErrNotFound
	}
	plan.SortOrder = sortOrder
	return nil
}

func (m *MockPlanRepository) Delete(ctx context.Context, id string) error {
	plan, exists := m.Plans[id]
	if !exists {
		return sharederrors.ErrNotFound
	}
	delete(m.Slugs, plan.Slug)
	delete(m.Plans, id)
	return nil
}

func (m *MockPlanRepository) CountVMsByPlan(ctx context.Context, planID string) (int, error) {
	// For testing, return 0 (no VMs) - this mock doesn't track VMs
	return 0, nil
}

// Helper function to create a test logger.
func testPlanLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

// Helper function to create a test plan.
func testPlan(id, name, slug string) *models.Plan {
	return &models.Plan{
		ID:               id,
		Name:             name,
		Slug:             slug,
		VCPU:             2,
		MemoryMB:         2048,
		DiskGB:           40,
		BandwidthLimitGB: 1000,
		PortSpeedMbps:    1000,
		PriceMonthly:     1000,  // $10.00 in cents
		PriceHourly:      14,    // $0.14 in cents
		StorageBackend:   models.StorageBackendCeph,
		IsActive:         true,
		SortOrder:        1,
		SnapshotLimit:    2,
		BackupLimit:      2,
		ISOUploadLimit:   2,
	}
}

// Helper function to create a plan creation request.
func testPlanCreateRequest(slug string) *models.PlanCreateRequest {
	return &models.PlanCreateRequest{
		Name:             "Test Plan",
		Slug:             slug,
		VCPU:             2,
		MemoryMB:         2048,
		DiskGB:           40,
		BandwidthLimitGB: 1000,
		PortSpeedMbps:    1000,
		PriceMonthly:     1000,
		PriceHourly:      14,
		IsActive:         true,
		SortOrder:        1,
	}
}

func TestPlanService_ListActive(t *testing.T) {
	tests := []struct {
		name       string
		setupMock  func(*MockPlanRepository)
		wantCount  int
		wantErr    bool
		errContain string
	}{
		{
			name: "returns active plans only",
			setupMock: func(m *MockPlanRepository) {
				m.Plans["plan-1"] = testPlan("plan-1", "Active Plan", "active-plan")
				m.Plans["plan-1"].IsActive = true
				m.Plans["plan-2"] = testPlan("plan-2", "Inactive Plan", "inactive-plan")
				m.Plans["plan-2"].IsActive = false
			},
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "returns empty list when no active plans",
			setupMock: func(m *MockPlanRepository) {
				m.Plans["plan-1"] = testPlan("plan-1", "Inactive Plan", "inactive-plan")
				m.Plans["plan-1"].IsActive = false
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:       "returns empty list when no plans exist",
			setupMock:  func(m *MockPlanRepository) {},
			wantCount:  0,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockPlanRepository()
			tt.setupMock(mockRepo)
			svc := NewPlanService(mockRepo, testPlanLogger())

			plans, err := svc.ListActive(context.Background())

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContain != "" {
					assert.Contains(t, err.Error(), tt.errContain)
				}
				return
			}

			require.NoError(t, err)
			assert.Len(t, plans, tt.wantCount)

			// Verify all returned plans are active
			for _, plan := range plans {
				assert.True(t, plan.IsActive)
			}
		})
	}
}

func TestPlanService_List(t *testing.T) {
	active := true
	inactive := false

	tests := []struct {
		name       string
		filter     repository.PlanListFilter
		setupMock  func(*MockPlanRepository)
		wantCount  int
		wantTotal  int
		wantErr    bool
		errContain string
	}{
		{
			name:   "returns all plans without filter",
			filter: repository.PlanListFilter{},
			setupMock: func(m *MockPlanRepository) {
				m.Plans["plan-1"] = testPlan("plan-1", "Plan 1", "plan-1")
				m.Plans["plan-2"] = testPlan("plan-2", "Plan 2", "plan-2")
			},
			wantCount: 2,
			wantTotal: 2,
			wantErr:   false,
		},
		{
			name: "filters by active status true",
			filter: repository.PlanListFilter{
				IsActive: &active,
			},
			setupMock: func(m *MockPlanRepository) {
				m.Plans["plan-1"] = testPlan("plan-1", "Active", "active")
				m.Plans["plan-1"].IsActive = true
				m.Plans["plan-2"] = testPlan("plan-2", "Inactive", "inactive")
				m.Plans["plan-2"].IsActive = false
			},
			wantCount: 1,
			wantTotal: 1,
			wantErr:   false,
		},
		{
			name: "filters by active status false",
			filter: repository.PlanListFilter{
				IsActive: &inactive,
			},
			setupMock: func(m *MockPlanRepository) {
				m.Plans["plan-1"] = testPlan("plan-1", "Active", "active")
				m.Plans["plan-1"].IsActive = true
				m.Plans["plan-2"] = testPlan("plan-2", "Inactive", "inactive")
				m.Plans["plan-2"].IsActive = false
			},
			wantCount: 1,
			wantTotal: 1,
			wantErr:   false,
		},
		{
			name:   "returns empty when no plans match filter",
			filter: repository.PlanListFilter{IsActive: &active},
			setupMock: func(m *MockPlanRepository) {
				m.Plans["plan-1"] = testPlan("plan-1", "Inactive", "inactive")
				m.Plans["plan-1"].IsActive = false
			},
			wantCount: 0,
			wantTotal: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockPlanRepository()
			tt.setupMock(mockRepo)
			svc := NewPlanService(mockRepo, testPlanLogger())

			plans, total, err := svc.List(context.Background(), tt.filter)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContain != "" {
					assert.Contains(t, err.Error(), tt.errContain)
				}
				return
			}

			require.NoError(t, err)
			assert.Len(t, plans, tt.wantCount)
			assert.Equal(t, tt.wantTotal, total)
		})
	}
}

func TestPlanService_GetByID(t *testing.T) {
	tests := []struct {
		name       string
		planID     string
		setupMock  func(*MockPlanRepository)
		wantErr    bool
		errContain string
	}{
		{
			name:   "returns plan when found",
			planID: "plan-123",
			setupMock: func(m *MockPlanRepository) {
				m.Plans["plan-123"] = testPlan("plan-123", "Test Plan", "test-plan")
			},
			wantErr: false,
		},
		{
			name:      "returns error when plan not found",
			planID:    "nonexistent",
			setupMock: func(m *MockPlanRepository) {},
			wantErr:   true,
			errContain: "plan not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockPlanRepository()
			tt.setupMock(mockRepo)
			svc := NewPlanService(mockRepo, testPlanLogger())

			plan, err := svc.GetByID(context.Background(), tt.planID)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContain)
				assert.Nil(t, plan)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, plan)
			assert.Equal(t, tt.planID, plan.ID)
		})
	}
}

func TestPlanService_Create(t *testing.T) {
	tests := []struct {
		name       string
		req        *models.PlanCreateRequest
		setupMock  func(*MockPlanRepository)
		wantErr    bool
		errContain string
		checkPlan  func(t *testing.T, plan *models.Plan)
	}{
		{
			name: "creates plan with all fields",
			req:  testPlanCreateRequest("new-plan"),
			setupMock: func(m *MockPlanRepository) {
				// No existing plans
			},
			wantErr: false,
			checkPlan: func(t *testing.T, plan *models.Plan) {
				assert.Equal(t, "Test Plan", plan.Name)
				assert.Equal(t, "new-plan", plan.Slug)
				assert.Equal(t, 2, plan.VCPU)
				assert.Equal(t, 2048, plan.MemoryMB)
				assert.Equal(t, 40, plan.DiskGB)
				assert.Equal(t, models.StorageBackendCeph, plan.StorageBackend)
				assert.True(t, plan.IsActive)
			},
		},
		{
			name: "creates plan with qcow storage backend",
			req: &models.PlanCreateRequest{
				Name:           "QCOW Plan",
				Slug:           "qcow-plan",
				VCPU:           1,
				MemoryMB:       1024,
				DiskGB:         20,
				PortSpeedMbps:  100,
				StorageBackend: models.StorageBackendQcow,
				IsActive:       true,
			},
			setupMock: func(m *MockPlanRepository) {},
			wantErr:   false,
			checkPlan: func(t *testing.T, plan *models.Plan) {
				assert.Equal(t, models.StorageBackendQcow, plan.StorageBackend)
			},
		},
		{
			name: "sets default storage backend when empty",
			req: &models.PlanCreateRequest{
				Name:          "Default Backend Plan",
				Slug:          "default-backend",
				VCPU:          1,
				MemoryMB:      1024,
				DiskGB:        20,
				PortSpeedMbps: 100,
				IsActive:      true,
			},
			setupMock: func(m *MockPlanRepository) {},
			wantErr:   false,
			checkPlan: func(t *testing.T, plan *models.Plan) {
				assert.Equal(t, models.DefaultStorageBackend, plan.StorageBackend)
			},
		},
		{
			name: "sets default snapshot/backup/iso limits when zero",
			req: &models.PlanCreateRequest{
				Name:           "Default Limits Plan",
				Slug:           "default-limits",
				VCPU:           1,
				MemoryMB:       1024,
				DiskGB:         20,
				PortSpeedMbps:  100,
				SnapshotLimit:  0,
				BackupLimit:    0,
				ISOUploadLimit: 0,
				IsActive:       true,
			},
			setupMock: func(m *MockPlanRepository) {},
			wantErr:   false,
			checkPlan: func(t *testing.T, plan *models.Plan) {
				assert.Equal(t, 2, plan.SnapshotLimit)
				assert.Equal(t, 2, plan.BackupLimit)
				assert.Equal(t, 2, plan.ISOUploadLimit)
			},
		},
		{
			name: "preserves explicit snapshot/backup/iso limits",
			req: &models.PlanCreateRequest{
				Name:           "Explicit Limits Plan",
				Slug:           "explicit-limits",
				VCPU:           1,
				MemoryMB:       1024,
				DiskGB:         20,
				PortSpeedMbps:  100,
				SnapshotLimit:  5,
				BackupLimit:    10,
				ISOUploadLimit: 3,
				IsActive:       true,
			},
			setupMock: func(m *MockPlanRepository) {},
			wantErr:   false,
			checkPlan: func(t *testing.T, plan *models.Plan) {
				assert.Equal(t, 5, plan.SnapshotLimit)
				assert.Equal(t, 10, plan.BackupLimit)
				assert.Equal(t, 3, plan.ISOUploadLimit)
			},
		},
		{
			name: "returns error when slug already exists",
			req:  testPlanCreateRequest("existing-slug"),
			setupMock: func(m *MockPlanRepository) {
				existing := testPlan("plan-existing", "Existing Plan", "existing-slug")
				m.Plans["plan-existing"] = existing
				m.Slugs["existing-slug"] = "plan-existing"
			},
			wantErr:    true,
			errContain: "already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockPlanRepository()
			tt.setupMock(mockRepo)
			svc := NewPlanService(mockRepo, testPlanLogger())

			plan, err := svc.Create(context.Background(), tt.req)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContain)
				assert.Nil(t, plan)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, plan)
			assert.NotEmpty(t, plan.ID)

			if tt.checkPlan != nil {
				tt.checkPlan(t, plan)
			}
		})
	}
}

func TestPlanService_Update(t *testing.T) {
	tests := []struct {
		name       string
		plan       *models.Plan
		setupMock  func(*MockPlanRepository)
		wantErr    bool
		errContain string
	}{
		{
			name: "updates existing plan",
			plan: &models.Plan{
				ID:           "plan-123",
				Name:         "Updated Plan",
				Slug:         "updated-plan",
				VCPU:         4,
				MemoryMB:     4096,
				DiskGB:       80,
				IsActive:     true,
				StorageBackend: models.StorageBackendCeph,
			},
			setupMock: func(m *MockPlanRepository) {
				m.Plans["plan-123"] = testPlan("plan-123", "Original Plan", "original-plan")
				m.Slugs["original-plan"] = "plan-123"
			},
			wantErr: false,
		},
		{
			name: "allows changing slug to new unique value",
			plan: &models.Plan{
				ID:           "plan-123",
				Name:         "Plan",
				Slug:         "new-unique-slug",
				VCPU:         2,
				MemoryMB:     2048,
				DiskGB:       40,
				IsActive:     true,
				StorageBackend: models.StorageBackendCeph,
			},
			setupMock: func(m *MockPlanRepository) {
				m.Plans["plan-123"] = testPlan("plan-123", "Plan", "old-slug")
				m.Slugs["old-slug"] = "plan-123"
			},
			wantErr: false,
		},
		{
			name: "returns error when plan not found",
			plan: &models.Plan{
				ID:           "nonexistent",
				Name:         "Plan",
				Slug:         "plan",
				VCPU:         2,
				MemoryMB:     2048,
				DiskGB:       40,
				IsActive:     true,
				StorageBackend: models.StorageBackendCeph,
			},
			setupMock:  func(m *MockPlanRepository) {},
			wantErr:    true,
			errContain: "plan not found",
		},
		{
			name: "returns error when new slug conflicts with another plan",
			plan: &models.Plan{
				ID:           "plan-1",
				Name:         "Plan 1",
				Slug:         "plan-2-slug", // Trying to use another plan's slug
				VCPU:         2,
				MemoryMB:     2048,
				DiskGB:       40,
				IsActive:     true,
				StorageBackend: models.StorageBackendCeph,
			},
			setupMock: func(m *MockPlanRepository) {
				m.Plans["plan-1"] = testPlan("plan-1", "Plan 1", "plan-1-slug")
				m.Slugs["plan-1-slug"] = "plan-1"
				m.Plans["plan-2"] = testPlan("plan-2", "Plan 2", "plan-2-slug")
				m.Slugs["plan-2-slug"] = "plan-2"
			},
			wantErr:    true,
			errContain: "already exists",
		},
		{
			name: "allows keeping same slug",
			plan: &models.Plan{
				ID:           "plan-123",
				Name:         "Updated Name",
				Slug:         "same-slug",
				VCPU:         4,
				MemoryMB:     4096,
				DiskGB:       80,
				IsActive:     true,
				StorageBackend: models.StorageBackendCeph,
			},
			setupMock: func(m *MockPlanRepository) {
				m.Plans["plan-123"] = testPlan("plan-123", "Original Name", "same-slug")
				m.Slugs["same-slug"] = "plan-123"
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockPlanRepository()
			tt.setupMock(mockRepo)
			svc := NewPlanService(mockRepo, testPlanLogger())

			err := svc.Update(context.Background(), tt.plan)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContain)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestPlanService_Delete(t *testing.T) {
	tests := []struct {
		name       string
		planID     string
		setupMock  func(*MockPlanRepository)
		wantErr    bool
		errContain string
	}{
		{
			name:   "deletes existing plan",
			planID: "plan-123",
			setupMock: func(m *MockPlanRepository) {
				m.Plans["plan-123"] = testPlan("plan-123", "Plan to Delete", "delete-me")
				m.Slugs["delete-me"] = "plan-123"
			},
			wantErr: false,
		},
		{
			name:       "returns error when plan not found",
			planID:     "nonexistent",
			setupMock:  func(m *MockPlanRepository) {},
			wantErr:    true,
			errContain: "plan not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := NewMockPlanRepository()
			tt.setupMock(mockRepo)
			svc := NewPlanService(mockRepo, testPlanLogger())

			err := svc.Delete(context.Background(), tt.planID)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContain)
				return
			}

			require.NoError(t, err)

			// Verify plan was deleted
			_, exists := mockRepo.Plans[tt.planID]
			assert.False(t, exists, "plan should be deleted from repository")
		})
	}
}

func TestPlanService_NewPlanService(t *testing.T) {
	mockRepo := NewMockPlanRepository()
	logger := testPlanLogger()

	svc := NewPlanService(mockRepo, logger)

	require.NotNil(t, svc)
	assert.NotNil(t, svc.planRepo)
	assert.NotNil(t, svc.logger)
}

func TestPlanService_ErrorWrapping(t *testing.T) {
	t.Run("GetByID wraps not found error correctly", func(t *testing.T) {
		mockRepo := NewMockPlanRepository()
		svc := NewPlanService(mockRepo, testPlanLogger())

		_, err := svc.GetByID(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plan not found")
	})

	t.Run("Update wraps not found error correctly", func(t *testing.T) {
		mockRepo := NewMockPlanRepository()
		svc := NewPlanService(mockRepo, testPlanLogger())

		plan := &models.Plan{ID: "nonexistent", Slug: "test", StorageBackend: models.StorageBackendCeph}
		err := svc.Update(context.Background(), plan)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plan not found")
	})

	t.Run("Delete wraps not found error correctly", func(t *testing.T) {
		mockRepo := NewMockPlanRepository()
		svc := NewPlanService(mockRepo, testPlanLogger())

		err := svc.Delete(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plan not found")
	})
}

func TestPlanService_StorageBackendDefaults(t *testing.T) {
	t.Run("empty storage backend defaults to ceph", func(t *testing.T) {
		mockRepo := NewMockPlanRepository()
		svc := NewPlanService(mockRepo, testPlanLogger())

		req := &models.PlanCreateRequest{
			Name:          "Test Plan",
			Slug:          "test-storage-default",
			VCPU:          1,
			MemoryMB:      1024,
			DiskGB:        20,
			PortSpeedMbps: 100,
			IsActive:      true,
			// StorageBackend not set
		}

		plan, err := svc.Create(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, models.StorageBackendCeph, plan.StorageBackend)
	})
}