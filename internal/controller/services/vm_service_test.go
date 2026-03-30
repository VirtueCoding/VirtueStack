package services

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/AbuGosok/VirtueStack/internal/controller/repository"
	"github.com/AbuGosok/VirtueStack/internal/controller/tasks"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeDB struct {
	queryRowFunc func(ctx context.Context, sql string, args ...any) pgx.Row
	queryFunc    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	execFunc     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	beginFunc    func(ctx context.Context) (pgx.Tx, error)
}

func (f *fakeDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if f.queryRowFunc != nil {
		return f.queryRowFunc(ctx, sql, args...)
	}
	return &fakeRow{scanErr: pgx.ErrNoRows}
}

func (f *fakeDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if f.queryFunc != nil {
		return f.queryFunc(ctx, sql, args...)
	}
	return &fakeRows{}, nil
}

func (f *fakeDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if f.execFunc != nil {
		return f.execFunc(ctx, sql, args...)
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (f *fakeDB) Begin(ctx context.Context) (pgx.Tx, error) {
	if f.beginFunc != nil {
		return f.beginFunc(ctx)
	}
	return nil, fmt.Errorf("not implemented")
}

type fakeRow struct {
	values  []any
	scanErr error
}

func (f *fakeRow) Scan(dest ...any) error {
	if f.scanErr != nil {
		return f.scanErr
	}
	if len(dest) != len(f.values) {
		return fmt.Errorf("scan destination count mismatch: got %d want %d", len(dest), len(f.values))
	}
	for i := range dest {
		if err := assignValue(dest[i], f.values[i]); err != nil {
			return err
		}
	}
	return nil
}

type fakeRows struct {
	rows [][]any
	idx  int
	err  error
}

func (f *fakeRows) Close() {}

func (f *fakeRows) Err() error { return f.err }

func (f *fakeRows) CommandTag() pgconn.CommandTag { return pgconn.NewCommandTag("SELECT 0") }

func (f *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (f *fakeRows) Next() bool {
	if f.idx >= len(f.rows) {
		return false
	}
	f.idx++
	return true
}

func (f *fakeRows) Scan(dest ...any) error {
	if f.idx == 0 || f.idx > len(f.rows) {
		return fmt.Errorf("scan called before Next")
	}
	current := f.rows[f.idx-1]
	if len(dest) != len(current) {
		return fmt.Errorf("scan destination count mismatch: got %d want %d", len(dest), len(current))
	}
	for i := range dest {
		if err := assignValue(dest[i], current[i]); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeRows) Values() ([]any, error) {
	if f.idx == 0 || f.idx > len(f.rows) {
		return nil, fmt.Errorf("values called before Next")
	}
	return f.rows[f.idx-1], nil
}

func (f *fakeRows) RawValues() [][]byte { return nil }

func (f *fakeRows) Conn() *pgx.Conn { return nil }

func assignValue(dest any, val any) error {
	dv := reflect.ValueOf(dest)
	if dv.Kind() != reflect.Ptr || dv.IsNil() {
		return fmt.Errorf("destination is not a pointer")
	}
	if val == nil {
		dv.Elem().Set(reflect.Zero(dv.Elem().Type()))
		return nil
	}

	v := reflect.ValueOf(val)
	if v.Type().AssignableTo(dv.Elem().Type()) {
		dv.Elem().Set(v)
		return nil
	}
	if v.Type().ConvertibleTo(dv.Elem().Type()) {
		dv.Elem().Set(v.Convert(dv.Elem().Type()))
		return nil
	}
	return fmt.Errorf("cannot assign %T to %T", val, dest)
}

type mockNodeAgentClient struct {
	startErr error
	stopErr  error
}

func (m *mockNodeAgentClient) GetNodeMetrics(ctx context.Context, nodeID string) (*models.NodeHeartbeat, error) {
	return nil, nil
}
func (m *mockNodeAgentClient) PingNode(ctx context.Context, nodeID string) error { return nil }
func (m *mockNodeAgentClient) EvacuateNode(ctx context.Context, nodeID string) error {
	return nil
}
func (m *mockNodeAgentClient) StartVM(ctx context.Context, nodeID, vmID string) error { return m.startErr }
func (m *mockNodeAgentClient) StopVM(ctx context.Context, nodeID, vmID string, timeoutSeconds int) error {
	return m.stopErr
}
func (m *mockNodeAgentClient) ForceStopVM(ctx context.Context, nodeID, vmID string) error { return nil }
func (m *mockNodeAgentClient) DeleteVM(ctx context.Context, nodeID, vmID string) error     { return nil }
func (m *mockNodeAgentClient) ResizeVM(ctx context.Context, nodeID, vmID string, vcpu, memoryMB, diskGB int) error {
	return nil
}
func (m *mockNodeAgentClient) GetVMMetrics(ctx context.Context, nodeID, vmID string) (*models.VMMetrics, error) {
	return nil, nil
}
func (m *mockNodeAgentClient) GetVMStatus(ctx context.Context, nodeID, vmID string) (string, error) {
	return "", nil
}
func (m *mockNodeAgentClient) AbortMigration(ctx context.Context, nodeID, vmID string) error { return nil }
func (m *mockNodeAgentClient) MigrateVM(ctx context.Context, sourceNodeID, targetNodeID, vmID string, opts *tasks.MigrateVMOptions) error {
	return nil
}
func (m *mockNodeAgentClient) PostMigrateSetup(ctx context.Context, nodeID, vmID string, bandwidthMbps int) error {
	return nil
}

type mockTaskPublisher struct{}

func (m *mockTaskPublisher) PublishTask(ctx context.Context, taskType string, payload map[string]any) (string, error) {
	return "task-1", nil
}

func testVMServiceLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestVMService_SelectNodeForVM(t *testing.T) {
	tests := []struct {
		name       string
		nodeRows   [][]any
		wantErr    bool
		errContain string
	}{
		{
			name:       "no available nodes",
			nodeRows:   [][]any{},
			wantErr:    true,
			errContain: "no available nodes",
		},
		{
			name: "all nodes at capacity",
			nodeRows: [][]any{
				nodeRow("node-1", models.NodeStatusOnline, 4, 8192, 4, 8192),
			},
			wantErr:    true,
			errContain: "no available nodes with sufficient capacity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeDB{
				queryFunc: func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
					if strings.Contains(sql, "FROM nodes WHERE status = $1") {
						return &fakeRows{rows: tt.nodeRows}, nil
					}
					return &fakeRows{}, nil
				},
			}
			svc := NewVMService(VMServiceConfig{
				NodeRepo: repository.NewNodeRepository(db),
				Logger:   testVMServiceLogger(),
			})

			node, err := svc.selectNodeForVM(context.Background(), "", 2, 2048, "")
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContain)
				assert.Nil(t, node)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, node)
		})
	}
}

func TestVMService_CreateVMValidationErrors(t *testing.T) {
	tests := []struct {
		name         string
		planRow      []any
		planErr      error
		templateRow  []any
		templateErr  error
		req          *models.VMCreateRequest
		errContains  string
	}{
		{
			name:        "invalid plan ID returns validation error",
			planErr:     pgx.ErrNoRows,
			templateErr: pgx.ErrNoRows,
			req: &models.VMCreateRequest{
				PlanID:     "missing-plan",
				TemplateID: "template-1",
			},
			errContains: "plan not found",
		},
		{
			name: "invalid template ID returns validation error",
			planRow: planRow("plan-1", true, 40),
			templateErr: pgx.ErrNoRows,
			req: &models.VMCreateRequest{
				PlanID:     "plan-1",
				TemplateID: "missing-template",
			},
			errContains: "template not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeDB{
				queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
					switch {
					case strings.Contains(sql, "FROM plans WHERE id = $1"):
						return &fakeRow{values: tt.planRow, scanErr: tt.planErr}
					case strings.Contains(sql, "FROM templates WHERE id = $1"):
						return &fakeRow{values: tt.templateRow, scanErr: tt.templateErr}
					default:
						return &fakeRow{scanErr: pgx.ErrNoRows}
					}
				},
			}

			svc := NewVMService(VMServiceConfig{
				PlanRepo:     repository.NewPlanRepository(db),
				TemplateRepo: repository.NewTemplateRepository(db),
				Logger:       testVMServiceLogger(),
			})

			vm, taskID, err := svc.CreateVM(context.Background(), tt.req, "customer-1")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
			assert.Nil(t, vm)
			assert.Equal(t, "", taskID)
		})
	}
}

func TestVMService_PowerAndDeleteConflictPaths(t *testing.T) {
	now := time.Now().UTC()
	nodeID := "node-1"
	tests := []struct {
		name        string
		vmRow       []any
		method      string
		execTag     pgconn.CommandTag
		wantErr     bool
		errContains string
	}{
		{
			name:        "start vm on already-running vm returns conflict",
			vmRow:       vmRow("vm-1", "customer-1", &nodeID, models.VMStatusRunning, nil),
			method:      "start",
			wantErr:     true,
			errContains: "cannot start VM in status running",
		},
		{
			name:        "stop vm on already-stopped vm returns conflict",
			vmRow:       vmRow("vm-2", "customer-1", &nodeID, models.VMStatusStopped, nil),
			method:      "stop",
			wantErr:     true,
			errContains: "cannot stop VM in status stopped",
		},
		{
			name:        "delete vm already deleted returns error path",
			vmRow:       vmRow("vm-3", "customer-1", &nodeID, models.VMStatusDeleted, &now),
			method:      "delete",
			wantErr:     true,
			errContains: "VM has been deleted",
		},
		{
			name:        "state machine conflict on start transition returns conflict error",
			vmRow:       vmRow("vm-4", "customer-1", &nodeID, models.VMStatusStopped, nil),
			method:      "start",
			execTag:     pgconn.NewCommandTag("UPDATE 0"),
			wantErr:     true,
			errContains: "transitioning VM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeDB{
				queryRowFunc: func(ctx context.Context, sql string, args ...any) pgx.Row {
					if strings.Contains(sql, "FROM vms WHERE id = $1 AND deleted_at IS NULL") {
						return &fakeRow{values: tt.vmRow}
					}
					return &fakeRow{scanErr: pgx.ErrNoRows}
				},
				execFunc: func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
					if tt.execTag.RowsAffected() != 0 {
						return tt.execTag, nil
					}
					if strings.Contains(sql, "WHERE id = $2 AND status = $3 AND deleted_at IS NULL") {
						return pgconn.NewCommandTag("UPDATE 1"), nil
					}
					return pgconn.NewCommandTag("UPDATE 1"), nil
				},
			}

			if tt.name == "state machine conflict on start transition returns conflict error" {
				db.execFunc = func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
					if strings.Contains(sql, "WHERE id = $2 AND status = $3 AND deleted_at IS NULL") {
						return pgconn.NewCommandTag("UPDATE 0"), nil
					}
					return pgconn.NewCommandTag("UPDATE 1"), nil
				}
			}

			svc := NewVMService(VMServiceConfig{
				VMRepo:        repository.NewVMRepository(db),
				TaskPublisher: &mockTaskPublisher{},
				NodeAgent:     &mockNodeAgentClient{},
				Logger:        testVMServiceLogger(),
			})

			var err error
			switch tt.method {
			case "start":
				err = svc.StartVM(context.Background(), "vm-id", "customer-1", false)
			case "stop":
				err = svc.StopVM(context.Background(), "vm-id", "customer-1", false, false)
			case "delete":
				_, err = svc.DeleteVM(context.Background(), "vm-id", "customer-1", false)
			default:
				t.Fatalf("unknown test method %q", tt.method)
			}

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}
			require.NoError(t, err)
		})
	}
}

func planRow(id string, isActive bool, diskGB int) []any {
	now := time.Now().UTC()
	return []any{
		id, "test-plan", "test-plan", 2, 2048,
		diskGB, 1000, 1000,
		1000, 14, models.StorageTypeCeph, isActive,
		1, now, now, 2, 2, 2,
	}
}

func nodeRow(id, status string, totalVCPU, totalMemoryMB, allocatedVCPU, allocatedMemoryMB int) []any {
	now := time.Now().UTC()
	return []any{
		id, "node-" + id, "10.0.0.1:50051", "10.0.0.1",
		nil, status, totalVCPU, totalMemoryMB,
		allocatedVCPU, allocatedMemoryMB, "vs-vms",
		nil, nil, nil,
		&now, 0, now,
		models.StorageTypeCeph, "", nil, nil,
	}
}

func vmRow(id, customerID string, nodeID *string, status string, deletedAt *time.Time) []any {
	now := time.Now().UTC()
	return []any{
		id, customerID, nodeID, "plan-1",
		"vm-host", status, 2, 2048,
		40, 1000, 1000,
		int64(0), now,
		"52:54:00:12:34:56", nil, nil,
		nil, nil, nil,
		now, now, deletedAt,
		models.StorageBackendCeph, nil, nil, nil,
	}
}

func init() {
	_ = sharederrors.ErrConflict
}
