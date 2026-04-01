package provisioning

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/services"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubPlanRepository struct {
	plans map[string]*models.Plan
}

func (s *stubPlanRepository) Create(_ context.Context, _ *models.Plan) error { return nil }
func (s *stubPlanRepository) GetByID(_ context.Context, id string) (*models.Plan, error) {
	plan, ok := s.plans[id]
	if !ok {
		return nil, sharederrors.ErrNotFound
	}
	return plan, nil
}
func (s *stubPlanRepository) GetBySlug(context.Context, string) (*models.Plan, error) {
	return nil, sharederrors.ErrNotFound
}
func (s *stubPlanRepository) List(_ context.Context, filter repository.PlanListFilter) ([]models.Plan, bool, string, error) {
	plans := make([]models.Plan, 0)
	for _, plan := range s.plans {
		if filter.IsActive != nil && plan.IsActive != *filter.IsActive {
			continue
		}
		plans = append(plans, *plan)
	}
	hasMore := len(plans) > filter.PerPage
	if hasMore {
		plans = plans[:filter.PerPage]
	}
	lastID := ""
	if len(plans) > 0 {
		lastID = plans[len(plans)-1].ID
	}
	return plans, hasMore, lastID, nil
}
func (s *stubPlanRepository) ListActive(_ context.Context) ([]models.Plan, error) {
	plans := make([]models.Plan, 0)
	for _, plan := range s.plans {
		if plan.IsActive {
			plans = append(plans, *plan)
		}
	}
	return plans, nil
}
func (s *stubPlanRepository) Update(context.Context, *models.Plan) error { return nil }
func (s *stubPlanRepository) Delete(context.Context, string) error       { return nil }
func (s *stubPlanRepository) CountVMsByPlan(context.Context, string) (int, error) {
	return 0, nil
}

func newProvisioningPlanHandler(t *testing.T, plans map[string]*models.Plan) *ProvisioningHandler {
	t.Helper()
	return &ProvisioningHandler{
		logger:      testProvisioningLogger(),
		planService: services.NewPlanService(&stubPlanRepository{plans: plans}, testProvisioningLogger()),
	}
}

func TestGetPlan_ReturnsNotFoundForMissingPlan(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := newProvisioningPlanHandler(t, map[string]*models.Plan{})
	router := gin.New()
	router.GET("/plans/:id", handler.GetPlan)

	req := httptest.NewRequest(http.MethodGet, "/plans/550e8400-e29b-41d4-a716-446655440000", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp models.ErrorResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "PLAN_NOT_FOUND", resp.Error.Code)
}

func TestListPlans_UsesStandardPaginatedResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := newProvisioningPlanHandler(t, map[string]*models.Plan{
		"plan-1": {ID: "plan-1", Name: "Plan One", IsActive: true},
		"plan-2": {ID: "plan-2", Name: "Plan Two", IsActive: true},
	})
	router := gin.New()
	router.GET("/plans", handler.ListPlans)

	req := httptest.NewRequest(http.MethodGet, "/plans?per_page=1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Data []models.Plan         `json:"data"`
		Meta models.PaginationMeta `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Data, 1)
	assert.Equal(t, 1, resp.Meta.PerPage)
	assert.True(t, resp.Meta.HasMore)
	assert.NotEmpty(t, resp.Meta.NextCursor)
}
