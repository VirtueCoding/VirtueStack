package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

// mockPlanDB implements the DB interface for testing PlanRepository.
type mockPlanDB struct {
	execFunc     func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	queryFunc    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (m *mockPlanDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.queryFunc != nil {
		return m.queryFunc(ctx, sql, args...)
	}
	return nil, nil
}

func (m *mockPlanDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFunc != nil {
		return m.queryRowFunc(ctx, sql, args...)
	}
	return nil
}

func (m *mockPlanDB) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if m.execFunc != nil {
		return m.execFunc(ctx, sql, arguments...)
	}
	return pgconn.CommandTag{}, nil
}

func (m *mockPlanDB) Begin(ctx context.Context) (pgx.Tx, error) {
	return &mockPlanTx{mockPlanDB: m}, nil
}

// mockPlanTx implements pgx.Tx for testing.
type mockPlanTx struct {
	*mockPlanDB
}

func (m *mockPlanTx) Commit(ctx context.Context) error {
	return nil
}

func (m *mockPlanTx) Rollback(ctx context.Context) error {
	return nil
}

func (m *mockPlanTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, nil
}

func (m *mockPlanTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return nil
}

func (m *mockPlanTx) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}

func (m *mockPlanTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return nil, nil
}

func (m *mockPlanTx) Conn() *pgx.Conn {
	return nil
}

func (m *mockPlanTx) Begin(ctx context.Context) (pgx.Tx, error) {
	return m, nil
}

// mockPlanRow implements pgx.Row for testing QueryRow results.
type mockPlanRow struct {
	plan models.Plan
	err  error
}

func (m mockPlanRow) Scan(dest ...any) error {
	if m.err != nil {
		return m.err
	}
	// Handle scanPlan case with 20 column pointers
	if len(dest) >= 20 {
		if id, ok := dest[0].(*string); ok {
			*id = m.plan.ID
		}
		if name, ok := dest[1].(*string); ok {
			*name = m.plan.Name
		}
		if slug, ok := dest[2].(*string); ok {
			*slug = m.plan.Slug
		}
		if vcpu, ok := dest[3].(*int); ok {
			*vcpu = m.plan.VCPU
		}
		if memoryMB, ok := dest[4].(*int); ok {
			*memoryMB = m.plan.MemoryMB
		}
		if diskGB, ok := dest[5].(*int); ok {
			*diskGB = m.plan.DiskGB
		}
		if bandwidthLimitGB, ok := dest[6].(*int); ok {
			*bandwidthLimitGB = m.plan.BandwidthLimitGB
		}
		if portSpeedMbps, ok := dest[7].(*int); ok {
			*portSpeedMbps = m.plan.PortSpeedMbps
		}
		if priceMonthly, ok := dest[8].(**int64); ok {
			*priceMonthly = m.plan.PriceMonthly
		}
		if priceHourly, ok := dest[9].(**int64); ok {
			*priceHourly = m.plan.PriceHourly
		}
		if priceHourlyStopped, ok := dest[10].(**int64); ok {
			*priceHourlyStopped = m.plan.PriceHourlyStopped
		}
		if currency, ok := dest[11].(*string); ok {
			*currency = m.plan.Currency
		}
		if storageBackend, ok := dest[12].(*string); ok {
			*storageBackend = m.plan.StorageBackend
		}
		if isActive, ok := dest[13].(*bool); ok {
			*isActive = m.plan.IsActive
		}
		if sortOrder, ok := dest[14].(*int); ok {
			*sortOrder = m.plan.SortOrder
		}
		if createdAt, ok := dest[15].(*time.Time); ok {
			*createdAt = m.plan.CreatedAt
		}
		if updatedAt, ok := dest[16].(*time.Time); ok {
			*updatedAt = m.plan.UpdatedAt
		}
		if snapshotLimit, ok := dest[17].(*int); ok {
			*snapshotLimit = m.plan.SnapshotLimit
		}
		if backupLimit, ok := dest[18].(*int); ok {
			*backupLimit = m.plan.BackupLimit
		}
		if isoUploadLimit, ok := dest[19].(*int); ok {
			*isoUploadLimit = m.plan.ISOUploadLimit
		}
		return nil
	}
	return nil
}

// mockPlanRows implements pgx.Rows for testing Query results.
type mockPlanRows struct {
	plans  []models.Plan
	idx    int
	closed bool
}

func (m *mockPlanRows) Close() {
	m.closed = true
}

func (m *mockPlanRows) Err() error {
	return nil
}

func (m *mockPlanRows) Next() bool {
	m.idx++
	return m.idx <= len(m.plans)
}

func (m *mockPlanRows) Scan(dest ...any) error {
	if m.idx < 1 || m.idx > len(m.plans) {
		return errors.New("no rows to scan")
	}
	plan := m.plans[m.idx-1]

	if len(dest) >= 20 {
		if id, ok := dest[0].(*string); ok {
			*id = plan.ID
		}
		if name, ok := dest[1].(*string); ok {
			*name = plan.Name
		}
		if slug, ok := dest[2].(*string); ok {
			*slug = plan.Slug
		}
		if vcpu, ok := dest[3].(*int); ok {
			*vcpu = plan.VCPU
		}
		if memoryMB, ok := dest[4].(*int); ok {
			*memoryMB = plan.MemoryMB
		}
		if diskGB, ok := dest[5].(*int); ok {
			*diskGB = plan.DiskGB
		}
		if bandwidthLimitGB, ok := dest[6].(*int); ok {
			*bandwidthLimitGB = plan.BandwidthLimitGB
		}
		if portSpeedMbps, ok := dest[7].(*int); ok {
			*portSpeedMbps = plan.PortSpeedMbps
		}
		if priceMonthly, ok := dest[8].(**int64); ok {
			*priceMonthly = plan.PriceMonthly
		}
		if priceHourly, ok := dest[9].(**int64); ok {
			*priceHourly = plan.PriceHourly
		}
		if priceHourlyStopped, ok := dest[10].(**int64); ok {
			*priceHourlyStopped = plan.PriceHourlyStopped
		}
		if currency, ok := dest[11].(*string); ok {
			*currency = plan.Currency
		}
		if storageBackend, ok := dest[12].(*string); ok {
			*storageBackend = plan.StorageBackend
		}
		if isActive, ok := dest[13].(*bool); ok {
			*isActive = plan.IsActive
		}
		if sortOrder, ok := dest[14].(*int); ok {
			*sortOrder = plan.SortOrder
		}
		if createdAt, ok := dest[15].(*time.Time); ok {
			*createdAt = plan.CreatedAt
		}
		if updatedAt, ok := dest[16].(*time.Time); ok {
			*updatedAt = plan.UpdatedAt
		}
		if snapshotLimit, ok := dest[17].(*int); ok {
			*snapshotLimit = plan.SnapshotLimit
		}
		if backupLimit, ok := dest[18].(*int); ok {
			*backupLimit = plan.BackupLimit
		}
		if isoUploadLimit, ok := dest[19].(*int); ok {
			*isoUploadLimit = plan.ISOUploadLimit
		}
	}
	return nil
}

func (m *mockPlanRows) Values() ([]any, error) {
	return nil, nil
}

func (m *mockPlanRows) RawValues() [][]byte {
	return nil
}

func (m *mockPlanRows) Conn() *pgx.Conn {
	return nil
}

func (m *mockPlanRows) CommandTag() pgconn.CommandTag {
	return pgconn.NewCommandTag("SELECT 0")
}

func (m *mockPlanRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func int64Ptr(v int64) *int64 { return &v }

// newTestPlan creates a Plan with sensible test defaults.
func newTestPlan() models.Plan {
	now := time.Now()
	return models.Plan{
		ID:               "550e8400-e29b-41d4-a716-446655440000",
		Name:             "Test Plan",
		Slug:             "test-plan",
		VCPU:             2,
		MemoryMB:         2048,
		DiskGB:           40,
		BandwidthLimitGB: 1000,
		PortSpeedMbps:    1000,
		PriceMonthly:     int64Ptr(999),
		PriceHourly:      int64Ptr(1),
		Currency:         "USD",
		StorageBackend:   "ceph",
		IsActive:         true,
		SortOrder:        1,
		SnapshotLimit:    5,
		BackupLimit:      3,
		ISOUploadLimit:   2,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

func TestPlanRepository_Create(t *testing.T) {
	testPlan := newTestPlan()

	tests := []struct {
		name        string
		plan        *models.Plan
		queryRowErr error
		wantErr     bool
		errContains string
	}{
		{
			name:        "successful create",
			plan:        &testPlan,
			queryRowErr: nil,
			wantErr:     false,
		},
		{
			name: "create with minimal fields",
			plan: &models.Plan{
				Name:           "Minimal Plan",
				Slug:           "minimal-plan",
				VCPU:           1,
				MemoryMB:       512,
				DiskGB:         10,
				StorageBackend: "qcow",
			},
			queryRowErr: nil,
			wantErr:     false,
		},
		{
			name:        "database error",
			plan:        &testPlan,
			queryRowErr: errors.New("connection refused"),
			wantErr:     true,
			errContains: "connection refused",
		},
		{
			name:        "constraint violation",
			plan:        &testPlan,
			queryRowErr: errors.New("duplicate key value violates unique constraint"),
			wantErr:     true,
			errContains: "duplicate key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockPlanDB{
				queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
					if tt.queryRowErr != nil {
						return mockPlanRow{err: tt.queryRowErr}
					}
					created := *tt.plan
					created.ID = "550e8400-e29b-41d4-a716-446655440001"
					created.CreatedAt = time.Now()
					created.UpdatedAt = time.Now()
					return mockPlanRow{plan: created}
				},
			}

			repo := NewPlanRepository(mock)
			err := repo.Create(context.Background(), tt.plan)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPlanRepository_GetByID(t *testing.T) {
	testPlan := newTestPlan()

	tests := []struct {
		name        string
		id          string
		queryRowErr error
		wantErr     bool
		errContains string
	}{
		{
			name:        "successful get",
			id:          testPlan.ID,
			queryRowErr: nil,
			wantErr:     false,
		},
		{
			name:        "plan not found",
			id:          "550e8400-e29b-41d4-a716-446655440001",
			queryRowErr: pgx.ErrNoRows,
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "database error",
			id:          testPlan.ID,
			queryRowErr: errors.New("connection refused"),
			wantErr:     true,
			errContains: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockPlanDB{
				queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
					if tt.queryRowErr != nil {
						return mockPlanRow{err: tt.queryRowErr}
					}
					return mockPlanRow{plan: testPlan}
				},
			}

			repo := NewPlanRepository(mock)
			result, err := repo.GetByID(context.Background(), tt.id)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result == nil {
					t.Fatal("expected result, got nil")
				}
				if result.ID != testPlan.ID {
					t.Fatalf("expected ID %s, got %s", testPlan.ID, result.ID)
				}
			}
		})
	}
}

func TestPlanRepository_GetByName(t *testing.T) {
	testPlan := newTestPlan()

	tests := []struct {
		name        string
		planName    string
		queryRowErr error
		wantErr     bool
		errContains string
	}{
		{
			name:        "successful get",
			planName:    testPlan.Name,
			queryRowErr: nil,
			wantErr:     false,
		},
		{
			name:        "plan not found",
			planName:    "Nonexistent Plan",
			queryRowErr: pgx.ErrNoRows,
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "database error",
			planName:    testPlan.Name,
			queryRowErr: errors.New("connection refused"),
			wantErr:     true,
			errContains: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockPlanDB{
				queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
					if tt.queryRowErr != nil {
						return mockPlanRow{err: tt.queryRowErr}
					}
					return mockPlanRow{plan: testPlan}
				},
			}

			repo := NewPlanRepository(mock)
			result, err := repo.GetByName(context.Background(), tt.planName)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result == nil {
					t.Fatal("expected result, got nil")
				}
				if result.Name != testPlan.Name {
					t.Fatalf("expected Name %s, got %s", testPlan.Name, result.Name)
				}
			}
		})
	}
}

func TestPlanRepository_GetBySlug(t *testing.T) {
	testPlan := newTestPlan()

	tests := []struct {
		name        string
		slug        string
		queryRowErr error
		wantErr     bool
		errContains string
	}{
		{
			name:        "successful get",
			slug:        testPlan.Slug,
			queryRowErr: nil,
			wantErr:     false,
		},
		{
			name:        "plan not found",
			slug:        "nonexistent-slug",
			queryRowErr: pgx.ErrNoRows,
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "database error",
			slug:        testPlan.Slug,
			queryRowErr: errors.New("connection refused"),
			wantErr:     true,
			errContains: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockPlanDB{
				queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
					if tt.queryRowErr != nil {
						return mockPlanRow{err: tt.queryRowErr}
					}
					return mockPlanRow{plan: testPlan}
				},
			}

			repo := NewPlanRepository(mock)
			result, err := repo.GetBySlug(context.Background(), tt.slug)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result == nil {
					t.Fatal("expected result, got nil")
				}
				if result.Slug != testPlan.Slug {
					t.Fatalf("expected Slug %s, got %s", testPlan.Slug, result.Slug)
				}
			}
		})
	}
}

func TestPlanRepository_List(t *testing.T) {
	testPlan1 := newTestPlan()
	testPlan2 := newTestPlan()
	testPlan2.ID = "550e8400-e29b-41d4-a716-446655440002"
	testPlan2.Name = "Test Plan 2"
	testPlan2.Slug = "test-plan-2"
	testPlan2.IsActive = false

	tests := []struct {
		name        string
		filter      PlanListFilter
		plans       []models.Plan
		queryErr    error
		wantErr     bool
		errContains string
		wantCount   int
	}{
		{
			name:      "list all plans",
			filter:    PlanListFilter{PaginationParams: models.PaginationParams{PerPage: 20}},
			plans:     []models.Plan{testPlan1, testPlan2},
			wantCount: 2,
		},
		{
			name:      "list active plans only",
			filter:    PlanListFilter{IsActive: boolPtr(true), PaginationParams: models.PaginationParams{PerPage: 20}},
			plans:     []models.Plan{testPlan1},
			wantCount: 1,
		},
		{
			name:      "list inactive plans only",
			filter:    PlanListFilter{IsActive: boolPtr(false), PaginationParams: models.PaginationParams{PerPage: 20}},
			plans:     []models.Plan{testPlan2},
			wantCount: 1,
		},
		{
			name:      "empty result",
			filter:    PlanListFilter{PaginationParams: models.PaginationParams{PerPage: 20}},
			plans:     []models.Plan{},
			wantCount: 0,
		},
		{
			name:      "with pagination",
			filter:    PlanListFilter{PaginationParams: models.PaginationParams{PerPage: 10}},
			plans:     []models.Plan{testPlan1},
			wantCount: 1,
		},
		{
			name:        "query error",
			filter:      PlanListFilter{PaginationParams: models.PaginationParams{PerPage: 20}},
			queryErr:    errors.New("connection refused"),
			wantErr:     true,
			errContains: "listing plans",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockPlanDB{
				queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
					return mockPlanRow{plan: testPlan1}
				},
				queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
					if tt.queryErr != nil {
						return nil, tt.queryErr
					}
					return &mockPlanRows{plans: tt.plans, idx: 0}, nil
				},
			}

			repo := NewPlanRepository(mock)
			result, hasMore, lastID, err := repo.List(context.Background(), tt.filter)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(result) != tt.wantCount {
					t.Fatalf("expected %d plans, got %d", tt.wantCount, len(result))
				}
				if tt.wantCount > 0 {
					assert.NotEmpty(t, lastID)
				}
				_ = hasMore
			}
		})
	}
}

func TestPlanRepository_ListActive(t *testing.T) {
	testPlan1 := newTestPlan()
	testPlan2 := newTestPlan()
	testPlan2.ID = "550e8400-e29b-41d4-a716-446655440002"
	testPlan2.Name = "Test Plan 2"
	testPlan2.Slug = "test-plan-2"

	tests := []struct {
		name        string
		plans       []models.Plan
		queryErr    error
		wantErr     bool
		errContains string
		wantCount   int
	}{
		{
			name:      "list active plans",
			plans:     []models.Plan{testPlan1, testPlan2},
			wantCount: 2,
		},
		{
			name:      "empty result",
			plans:     []models.Plan{},
			wantCount: 0,
		},
		{
			name:        "database error",
			queryErr:    errors.New("connection refused"),
			wantErr:     true,
			errContains: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockPlanDB{
				queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
					if tt.queryErr != nil {
						return nil, tt.queryErr
					}
					return &mockPlanRows{plans: tt.plans, idx: 0}, nil
				},
			}

			repo := NewPlanRepository(mock)
			result, err := repo.ListActive(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(result) != tt.wantCount {
					t.Fatalf("expected %d plans, got %d", tt.wantCount, len(result))
				}
			}
		})
	}
}

func TestPlanRepository_Update(t *testing.T) {
	testPlan := newTestPlan()

	tests := []struct {
		name        string
		plan        *models.Plan
		queryRowErr error
		wantErr     bool
		errContains string
	}{
		{
			name: "successful update",
			plan: &models.Plan{
				ID:               testPlan.ID,
				Name:             "Updated Plan",
				Slug:             "updated-plan",
				VCPU:             4,
				MemoryMB:         4096,
				DiskGB:           80,
				BandwidthLimitGB: 2000,
				PortSpeedMbps:    2000,
				PriceMonthly:     int64Ptr(1999),
				PriceHourly:      int64Ptr(2),
				Currency:         "USD",
				StorageBackend:   "ceph",
				IsActive:         true,
				SortOrder:        2,
				SnapshotLimit:    10,
				BackupLimit:      5,
				ISOUploadLimit:   3,
			},
			queryRowErr: nil,
			wantErr:     false,
		},
		{
			name: "update not found",
			plan: &models.Plan{
				ID:   "550e8400-e29b-41d4-a716-446655440001",
				Name: "Nonexistent Plan",
				Slug: "nonexistent-plan",
			},
			queryRowErr: pgx.ErrNoRows,
			wantErr:     true,
			errContains: "updating plan",
		},
		{
			name: "database error",
			plan: &models.Plan{
				ID:   testPlan.ID,
				Name: "Updated Plan",
				Slug: "updated-plan",
			},
			queryRowErr: errors.New("connection refused"),
			wantErr:     true,
			errContains: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockPlanDB{
				queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
					if tt.queryRowErr != nil {
						return mockPlanRow{err: tt.queryRowErr}
					}
					updated := *tt.plan
					updated.UpdatedAt = time.Now()
					return mockPlanRow{plan: updated}
				},
			}

			repo := NewPlanRepository(mock)
			err := repo.Update(context.Background(), tt.plan)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPlanRepository_UpdateActive(t *testing.T) {
	testPlan := newTestPlan()

	tests := []struct {
		name         string
		id           string
		isActive     bool
		rowsAffected int64
		execErr      error
		wantErr      bool
		errContains  string
	}{
		{
			name:         "activate plan",
			id:           testPlan.ID,
			isActive:     true,
			rowsAffected: 1,
			wantErr:      false,
		},
		{
			name:         "deactivate plan",
			id:           testPlan.ID,
			isActive:     false,
			rowsAffected: 1,
			wantErr:      false,
		},
		{
			name:         "plan not found",
			id:           "550e8400-e29b-41d4-a716-446655440001",
			isActive:     true,
			rowsAffected: 0,
			wantErr:      true,
			errContains:  ErrNoRowsAffected.Error(),
		},
		{
			name:        "database error",
			id:          testPlan.ID,
			isActive:    true,
			execErr:     errors.New("connection refused"),
			wantErr:     true,
			errContains: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockPlanDB{
				execFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
					if tt.execErr != nil {
						return pgconn.CommandTag{}, tt.execErr
					}
					return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", tt.rowsAffected)), nil
				},
			}

			repo := NewPlanRepository(mock)
			err := repo.UpdateActive(context.Background(), tt.id, tt.isActive)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPlanRepository_UpdateSortOrder(t *testing.T) {
	testPlan := newTestPlan()

	tests := []struct {
		name         string
		id           string
		sortOrder    int
		rowsAffected int64
		execErr      error
		wantErr      bool
		errContains  string
	}{
		{
			name:         "update sort order",
			id:           testPlan.ID,
			sortOrder:    5,
			rowsAffected: 1,
			wantErr:      false,
		},
		{
			name:         "sort order zero",
			id:           testPlan.ID,
			sortOrder:    0,
			rowsAffected: 1,
			wantErr:      false,
		},
		{
			name:         "plan not found",
			id:           "550e8400-e29b-41d4-a716-446655440001",
			sortOrder:    1,
			rowsAffected: 0,
			wantErr:      true,
			errContains:  ErrNoRowsAffected.Error(),
		},
		{
			name:        "database error",
			id:          testPlan.ID,
			sortOrder:   1,
			execErr:     errors.New("connection refused"),
			wantErr:     true,
			errContains: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockPlanDB{
				execFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
					if tt.execErr != nil {
						return pgconn.CommandTag{}, tt.execErr
					}
					return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", tt.rowsAffected)), nil
				},
			}

			repo := NewPlanRepository(mock)
			err := repo.UpdateSortOrder(context.Background(), tt.id, tt.sortOrder)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPlanRepository_Delete(t *testing.T) {
	testPlan := newTestPlan()

	tests := []struct {
		name         string
		id           string
		rowsAffected int64
		execErr      error
		wantErr      bool
		errContains  string
	}{
		{
			name:         "successful delete",
			id:           testPlan.ID,
			rowsAffected: 1,
			wantErr:      false,
		},
		{
			name:         "plan not found",
			id:           "550e8400-e29b-41d4-a716-446655440001",
			rowsAffected: 0,
			wantErr:      true,
			errContains:  ErrNoRowsAffected.Error(),
		},
		{
			name:        "database error",
			id:          testPlan.ID,
			execErr:     errors.New("connection refused"),
			wantErr:     true,
			errContains: "connection refused",
		},
		{
			name:        "foreign key constraint violation",
			id:          testPlan.ID,
			execErr:     errors.New("violates foreign key constraint"),
			wantErr:     true,
			errContains: "foreign key constraint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockPlanDB{
				execFunc: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
					if tt.execErr != nil {
						return pgconn.CommandTag{}, tt.execErr
					}
					return pgconn.NewCommandTag(fmt.Sprintf("UPDATE %d", tt.rowsAffected)), nil
				},
			}

			repo := NewPlanRepository(mock)
			err := repo.Delete(context.Background(), tt.id)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}