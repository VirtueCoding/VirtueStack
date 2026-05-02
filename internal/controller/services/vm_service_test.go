package services

import (
	"context"
	"database/sql"
	"encoding/json"
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
	if target, ok := dest.(*sql.NullString); ok {
		if val == nil {
			*target = sql.NullString{}
			return nil
		}
		str, ok := val.(string)
		if !ok {
			return fmt.Errorf("cannot assign %T to %T", val, dest)
		}
		*target = sql.NullString{String: str, Valid: true}
		return nil
	}

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

func (m *mockNodeAgentClient) GetNodeMetrics(_ context.Context, _ string) (*models.NodeHeartbeat, error) {
	return nil, nil
}
func (m *mockNodeAgentClient) PingNode(_ context.Context, _ string) error { return nil }
func (m *mockNodeAgentClient) EvacuateNode(_ context.Context, _ string) error {
	return nil
}
func (m *mockNodeAgentClient) StartVM(_ context.Context, _, _ string) error {
	return m.startErr
}
func (m *mockNodeAgentClient) StopVM(_ context.Context, _, _ string, _ int) error {
	return m.stopErr
}
func (m *mockNodeAgentClient) ForceStopVM(_ context.Context, _, _ string) error { return nil }
func (m *mockNodeAgentClient) DeleteVM(_ context.Context, _, _ string) error    { return nil }
func (m *mockNodeAgentClient) ResizeVM(_ context.Context, _, _ string, _, _, _ int) error {
	return nil
}
func (m *mockNodeAgentClient) GetVMMetrics(_ context.Context, _, _ string) (*models.VMMetrics, error) {
	return nil, nil
}
func (m *mockNodeAgentClient) GetVMStatus(_ context.Context, _, _ string) (string, error) {
	return "", nil
}
func (m *mockNodeAgentClient) AbortMigration(_ context.Context, _, _ string) error {
	return nil
}
func (m *mockNodeAgentClient) MigrateVM(_ context.Context, _, _, _ string, _ *tasks.MigrateVMOptions) error {
	return nil
}
func (m *mockNodeAgentClient) PostMigrateSetup(_ context.Context, _, _ string, _ int) error {
	return nil
}

type mockTaskPublisher struct{}

func (m *mockTaskPublisher) PublishTask(_ context.Context, _ string, _ map[string]any) (string, error) {
	return "task-1", nil
}

type countingTaskPublisher struct {
	calls int
}

func (m *countingTaskPublisher) PublishTask(_ context.Context, _ string, _ map[string]any) (string, error) {
	m.calls++
	return "task-1", nil
}

type recordingTaskPublisher struct {
	calls int
}

func (m *recordingTaskPublisher) PublishTask(_ context.Context, _ string, _ map[string]any) (string, error) {
	m.calls++
	return "task-1", nil
}

type failingTaskPublisher struct {
	calls int
}

func (m *failingTaskPublisher) PublishTask(_ context.Context, _ string, _ map[string]any) (string, error) {
	m.calls++
	return "", fmt.Errorf("publish failed")
}

type idempotencyRecordingTaskPublisher struct {
	calls               int
	publishedWithKey    bool
	capturedTaskType    string
	capturedPayload     map[string]any
	capturedIdempotency string
}

func (m *idempotencyRecordingTaskPublisher) PublishTask(_ context.Context, taskType string, payload map[string]any) (string, error) {
	m.calls++
	m.capturedTaskType = taskType
	m.capturedPayload = payload
	return "task-without-key", nil
}

func (m *idempotencyRecordingTaskPublisher) PublishTaskWithIdempotencyKey(_ context.Context, taskType string, payload map[string]any, idempotencyKey string) (string, error) {
	m.calls++
	m.publishedWithKey = true
	m.capturedTaskType = taskType
	m.capturedPayload = payload
	m.capturedIdempotency = idempotencyKey
	return "task-with-key", nil
}

type mockIPAM struct {
	getIPsByVMFunc func(ctx context.Context, vmID string) ([]models.IPAddress, error)
}

func (m *mockIPAM) AllocateIPv4(_ context.Context, _, _, _ string) (*models.IPAddress, error) {
	return nil, nil
}

func (m *mockIPAM) ReleaseIPsByVM(_ context.Context, _ string) error {
	return nil
}

func (m *mockIPAM) GetIPsByVM(ctx context.Context, vmID string) ([]models.IPAddress, error) {
	if m.getIPsByVMFunc != nil {
		return m.getIPsByVMFunc(ctx, vmID)
	}
	return nil, nil
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
				nodeRow(4, 8192, 4, 8192),
			},
			wantErr:    true,
			errContain: "no available nodes with sufficient capacity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeDB{
				queryFunc: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
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
		name        string
		planRow     []any
		planErr     error
		templateRow []any
		templateErr error
		req         *models.VMCreateRequest
		errContains string
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
			name:        "invalid template ID returns validation error",
			planRow:     planRow("plan-1", true, 40),
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
				queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
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
			name:        "delete vm already soft deleted returns retryable error",
			vmRow:       vmRow("vm-3", "customer-1", &nodeID, models.VMStatusDeleted, &now),
			method:      "delete",
			wantErr:     true,
			errContains: "already marked deleted",
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
				queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
					if strings.Contains(sql, "FROM vms WHERE id = $1") {
						return &fakeRow{values: tt.vmRow}
					}
					return &fakeRow{scanErr: pgx.ErrNoRows}
				},
				execFunc: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
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
				db.execFunc = func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
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

func TestVMServiceDeleteVMStatusTransitionFailureDoesNotPublishTask(t *testing.T) {
	nodeID := "node-1"
	publisher := &recordingTaskPublisher{}
	db := &fakeDB{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "FROM vms WHERE id = $1") {
				return &fakeRow{values: vmRow("vm-1", "customer-1", &nodeID, models.VMStatusRunning, nil)}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
		execFunc: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "WHERE id = $2 AND status = $3 AND deleted_at IS NULL") {
				return pgconn.NewCommandTag("UPDATE 0"), nil
			}
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}
	svc := NewVMService(VMServiceConfig{
		VMRepo:        repository.NewVMRepository(db),
		TaskPublisher: publisher,
		Logger:        testVMServiceLogger(),
	})

	taskID, err := svc.DeleteVM(context.Background(), "vm-1", "customer-1", false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "marking VM")
	assert.ErrorIs(t, err, sharederrors.ErrConflict)
	assert.Equal(t, "", taskID)
	assert.Equal(t, 0, publisher.calls)
}

func TestVMServiceDeleteVMPublishFailureRestoresPreviousStatus(t *testing.T) {
	nodeID := "node-1"
	publisher := &failingTaskPublisher{}
	restoreCalled := false
	db := &fakeDB{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "FROM vms WHERE id = $1") {
				return &fakeRow{values: vmRow("vm-1", "customer-1", &nodeID, models.VMStatusRunning, nil)}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
		execFunc: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if strings.Contains(sql, "WHERE id = $2 AND deleted_at IS NULL") &&
				!strings.Contains(sql, "status = $3") {
				restoreCalled = true
				return pgconn.NewCommandTag("UPDATE 1"), nil
			}
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
	}
	svc := NewVMService(VMServiceConfig{
		VMRepo:        repository.NewVMRepository(db),
		TaskPublisher: publisher,
		Logger:        testVMServiceLogger(),
	})

	taskID, err := svc.DeleteVM(context.Background(), "vm-1", "customer-1", false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "publishing delete task")
	assert.Equal(t, "", taskID)
	assert.Equal(t, 1, publisher.calls)
	assert.True(t, restoreCalled)
}

func TestVMServiceDeleteVMSoftDeletedWithoutDurableTaskDoesNotReturnSuccess(t *testing.T) {
	now := time.Now().UTC()
	nodeID := "node-1"
	publisher := &recordingTaskPublisher{}
	db := &fakeDB{
		queryRowFunc: func(_ context.Context, sql string, _ ...any) pgx.Row {
			if strings.Contains(sql, "FROM vms WHERE id = $1") {
				return &fakeRow{values: vmRow("vm-1", "customer-1", &nodeID, models.VMStatusDeleted, &now)}
			}
			return &fakeRow{scanErr: pgx.ErrNoRows}
		},
	}
	svc := NewVMService(VMServiceConfig{
		VMRepo:        repository.NewVMRepository(db),
		TaskPublisher: publisher,
		Logger:        testVMServiceLogger(),
	})

	taskID, err := svc.DeleteVM(context.Background(), "vm-1", "customer-1", false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "already marked deleted")
	assert.ErrorIs(t, err, sharederrors.ErrConflict)
	assert.Equal(t, "", taskID)
	assert.Equal(t, 0, publisher.calls)
}

func TestVMService_CreateVMRejectsExistingExternalServiceForSameCustomerWithoutTask(t *testing.T) {
	tests := []struct {
		name              string
		externalServiceID int
		customerID        string
	}{
		{
			name:              "same customer duplicate external service has no durable task",
			externalServiceID: 456,
			customerID:        "customer-owner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeDB{
				queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
					if strings.Contains(sql, "FROM vms WHERE external_service_id = $1") {
						require.Equal(t, tt.externalServiceID, args[0])
						return &fakeRow{values: vmRow("vm-existing", tt.customerID, nil, models.VMStatusRunning, nil)}
					}
					return &fakeRow{scanErr: pgx.ErrNoRows}
				},
			}
			taskPublisher := &countingTaskPublisher{}
			svc := NewVMService(VMServiceConfig{
				VMRepo:        repository.NewVMRepository(db),
				TaskPublisher: taskPublisher,
				Logger:        testVMServiceLogger(),
			})

			vm, taskID, err := svc.CreateVM(context.Background(), &models.VMCreateRequest{
				ExternalServiceID: &tt.externalServiceID,
			}, tt.customerID)

			require.Error(t, err)
			assert.ErrorIs(t, err, sharederrors.ErrConflict)
			assert.Nil(t, vm)
			assert.Empty(t, taskID)
			assert.Equal(t, 0, taskPublisher.calls)
		})
	}
}

func TestVMService_CreateVMPublishesTaskWithRequestIdempotencyKey(t *testing.T) {
	tests := []struct {
		name           string
		idempotencyKey string
		wantTaskID     string
	}{
		{
			name:           "vm create task carries request idempotency key",
			idempotencyKey: "provisioning-create-123",
			wantTaskID:     "task-with-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publisher := &idempotencyRecordingTaskPublisher{}
			svc := NewVMService(VMServiceConfig{
				TaskPublisher: publisher,
				Logger:        testVMServiceLogger(),
			})
			vm := &models.VM{
				ID:               "vm-1",
				Hostname:         "vm-host",
				VCPU:             2,
				MemoryMB:         2048,
				DiskGB:           40,
				PortSpeedMbps:    1000,
				BandwidthLimitGB: 1000,
				MACAddress:       "52:54:00:12:34:56",
			}
			deps := vmCreateDeps{
				plan:     &models.Plan{StorageBackend: models.StorageBackendCeph},
				template: &models.Template{ID: "template-1", RBDImage: "ubuntu", RBDSnapshot: "base"},
				node:     &models.Node{ID: "node-1", CephPool: "vs-vms"},
			}

			taskID, err := svc.publishVMCreateTask(context.Background(), &models.VMCreateRequest{
				IdempotencyKey: tt.idempotencyKey,
			}, vm, deps, nil)

			require.NoError(t, err)
			assert.Equal(t, tt.wantTaskID, taskID)
			assert.True(t, publisher.publishedWithKey)
			assert.Equal(t, 1, publisher.calls)
			assert.Equal(t, models.TaskTypeVMCreate, publisher.capturedTaskType)
			assert.Equal(t, tt.idempotencyKey, publisher.capturedIdempotency)
			assert.Equal(t, "vm-1", publisher.capturedPayload["vm_id"])
		})
	}
}

func TestDefaultTaskPublisher_PublishTaskWithIdempotencyKeyPersistsKey(t *testing.T) {
	tests := []struct {
		name           string
		idempotencyKey string
	}{
		{
			name:           "create task stores idempotency key",
			idempotencyKey: "provisioning-create-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeDB{
				queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
					require.Contains(t, sql, "INSERT INTO tasks")
					require.Equal(t, tt.idempotencyKey, args[6])
					taskID, ok := args[0].(string)
					require.True(t, ok)
					taskType, ok := args[1].(string)
					require.True(t, ok)
					payload, ok := args[3].(json.RawMessage)
					require.True(t, ok)
					return &fakeRow{values: taskRow(taskID, taskType, tt.idempotencyKey, payload)}
				},
			}
			publisher := NewDefaultTaskPublisher(repository.NewTaskRepository(db), testVMServiceLogger())

			taskID, err := publisher.PublishTaskWithIdempotencyKey(context.Background(), models.TaskTypeVMCreate, map[string]any{
				"vm_id": "vm-1",
			}, tt.idempotencyKey)

			require.NoError(t, err)
			assert.NotEmpty(t, taskID)
		})
	}
}

func TestVMService_CreateVMReplaysIdempotencyBeforeExternalServiceConflict(t *testing.T) {
	tests := []struct {
		name              string
		idempotencyKey    string
		externalServiceID int
		customerID        string
	}{
		{
			name:              "same keyed task and external service returns existing VM task",
			idempotencyKey:    "provisioning-create-456",
			externalServiceID: 456,
			customerID:        "customer-owner",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := json.Marshal(map[string]any{"vm_id": "vm-existing"})
			require.NoError(t, err)
			db := &fakeDB{
				queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
					switch {
					case strings.Contains(sql, "FROM tasks WHERE idempotency_key = $1"):
						require.Equal(t, tt.idempotencyKey, args[0])
						return &fakeRow{values: taskRow("task-existing", models.TaskTypeVMCreate, tt.idempotencyKey, payload)}
					case strings.Contains(sql, "FROM vms WHERE id = $1"):
						require.Equal(t, "vm-existing", args[0])
						row := vmRow("vm-existing", tt.customerID, nil, models.VMStatusRunning, nil)
						row[17] = &tt.externalServiceID
						return &fakeRow{values: row}
					case strings.Contains(sql, "FROM vms WHERE external_service_id = $1"):
						require.Equal(t, tt.externalServiceID, args[0])
						row := vmRow("vm-existing", tt.customerID, nil, models.VMStatusRunning, nil)
						row[17] = &tt.externalServiceID
						return &fakeRow{values: row}
					default:
						return &fakeRow{scanErr: pgx.ErrNoRows}
					}
				},
			}
			taskPublisher := &countingTaskPublisher{}
			svc := NewVMService(VMServiceConfig{
				VMRepo:        repository.NewVMRepository(db),
				TaskRepo:      repository.NewTaskRepository(db),
				TaskPublisher: taskPublisher,
				Logger:        testVMServiceLogger(),
			})

			vm, taskID, err := svc.CreateVM(context.Background(), &models.VMCreateRequest{
				ExternalServiceID: &tt.externalServiceID,
				IdempotencyKey:    tt.idempotencyKey,
			}, tt.customerID)

			require.NoError(t, err)
			require.NotNil(t, vm)
			assert.Equal(t, "vm-existing", vm.ID)
			assert.Equal(t, "task-existing", taskID)
			assert.Equal(t, 0, taskPublisher.calls)
		})
	}
}

func TestVMService_CreateVMRejectsExternalServiceOwnedByAnotherCustomer(t *testing.T) {
	tests := []struct {
		name              string
		externalServiceID int
		existingCustomer  string
		requestCustomer   string
	}{
		{
			name:              "external service belongs to another customer",
			externalServiceID: 123,
			existingCustomer:  "customer-owner",
			requestCustomer:   "customer-requester",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &fakeDB{
				queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
					if strings.Contains(sql, "FROM vms WHERE external_service_id = $1") {
						require.Equal(t, tt.externalServiceID, args[0])
						return &fakeRow{values: vmRow("vm-existing", tt.existingCustomer, nil, models.VMStatusRunning, nil)}
					}
					return &fakeRow{scanErr: pgx.ErrNoRows}
				},
			}
			svc := NewVMService(VMServiceConfig{
				VMRepo: repository.NewVMRepository(db),
				Logger: testVMServiceLogger(),
			})

			vm, taskID, err := svc.CreateVM(context.Background(), &models.VMCreateRequest{
				ExternalServiceID: &tt.externalServiceID,
			}, tt.requestCustomer)

			require.Error(t, err)
			assert.ErrorIs(t, err, sharederrors.ErrConflict)
			assert.Nil(t, vm)
			assert.Empty(t, taskID)
		})
	}
}

func TestVMService_CreateVMRejectsExistingExternalServiceAfterCreateConflictWithoutTask(t *testing.T) {
	tests := []struct {
		name              string
		externalServiceID int
		existingCustomer  string
		requestCustomer   string
	}{
		{
			name:              "concurrent duplicate create for same customer has no durable task",
			externalServiceID: 789,
			existingCustomer:  "customer-owner",
			requestCustomer:   "customer-owner",
		},
		{
			name:              "concurrent duplicate create for another customer returns conflict",
			externalServiceID: 790,
			existingCustomer:  "customer-owner",
			requestCustomer:   "customer-requester",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskPublisher := &countingTaskPublisher{}
			externalLookups := 0
			db := &fakeDB{
				queryRowFunc: func(_ context.Context, sql string, args ...any) pgx.Row {
					switch {
					case strings.Contains(sql, "FROM vms WHERE external_service_id = $1"):
						require.Equal(t, tt.externalServiceID, args[0])
						externalLookups++
						if externalLookups == 1 {
							return &fakeRow{scanErr: pgx.ErrNoRows}
						}
						return &fakeRow{values: vmRow("vm-existing", tt.existingCustomer, nil, models.VMStatusRunning, nil)}
					case strings.Contains(sql, "FROM plans WHERE id = $1"):
						return &fakeRow{values: planRow("plan-1", true, 40)}
					case strings.Contains(sql, "FROM templates WHERE id = $1"):
						return &fakeRow{values: templateRow("template-1", true, 10)}
					case strings.Contains(sql, "INSERT INTO vms"):
						return &fakeRow{scanErr: &pgconn.PgError{
							Code:           "23505",
							ConstraintName: "idx_vms_external_service_id_active_unique",
						}}
					default:
						return &fakeRow{scanErr: pgx.ErrNoRows}
					}
				},
				queryFunc: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
					if strings.Contains(sql, "FROM nodes WHERE status = $1") {
						return &fakeRows{rows: [][]any{nodeRow(8, 16384, 0, 0)}}, nil
					}
					return &fakeRows{}, nil
				},
			}
			svc := NewVMService(VMServiceConfig{
				VMRepo:        repository.NewVMRepository(db),
				NodeRepo:      repository.NewNodeRepository(db),
				PlanRepo:      repository.NewPlanRepository(db),
				TemplateRepo:  repository.NewTemplateRepository(db),
				TaskPublisher: taskPublisher,
				EncryptionKey: strings.Repeat("0", 64),
				Logger:        testVMServiceLogger(),
			})

			vm, taskID, err := svc.CreateVM(context.Background(), &models.VMCreateRequest{
				PlanID:            "plan-1",
				TemplateID:        "template-1",
				Hostname:          "vm-host",
				Password:          "correct horse battery staple",
				ExternalServiceID: &tt.externalServiceID,
			}, tt.requestCustomer)

			require.Error(t, err)
			assert.ErrorIs(t, err, sharederrors.ErrConflict)
			assert.Nil(t, vm)
			assert.Empty(t, taskID)
			assert.Equal(t, 2, externalLookups)
			assert.Equal(t, 0, taskPublisher.calls)
		})
	}
}

func TestVMService_ListVMsEnrichesDisplayNameAndPrimaryIPv4(t *testing.T) {
	db := &fakeDB{
		queryFunc: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			if strings.Contains(sql, "FROM vms") {
				return &fakeRows{
					rows: [][]any{vmRow("vm-1", "customer-1", nil, models.VMStatusRunning, nil)},
				}, nil
			}
			return &fakeRows{}, nil
		},
	}

	svc := NewVMService(VMServiceConfig{
		VMRepo: repository.NewVMRepository(db),
		IPAMService: &mockIPAM{
			getIPsByVMFunc: func(_ context.Context, _ string) ([]models.IPAddress, error) {
				return []models.IPAddress{
					{ID: "ip-1", Address: "192.0.2.10", IPVersion: 4, IsPrimary: true},
					{ID: "ip-2", Address: "2001:db8::10", IPVersion: 6, IsPrimary: true},
				}, nil
			},
		},
		Logger: testVMServiceLogger(),
	})

	vms, hasMore, lastID, err := svc.ListVMs(context.Background(), models.VMListFilter{
		PaginationParams: models.PaginationParams{PerPage: 20},
	}, "customer-1", false)
	require.NoError(t, err)
	require.Len(t, vms, 1)
	assert.False(t, hasMore)
	assert.Equal(t, "vm-1", lastID)
	assert.Equal(t, "vm-host", vms[0].Name)
	assert.Equal(t, "192.0.2.10", vms[0].IPv4)
}

func planRow(id string, isActive bool, diskGB int) []any {
	now := time.Now().UTC()
	pm := int64(1000)
	ph := int64(14)
	return []any{
		id, "test-plan", "test-plan", 2, 2048,
		diskGB, 1000, 1000,
		&pm, &ph, (*int64)(nil), "USD",
		models.StorageTypeCeph, isActive,
		1, now, now, 2, 2, 2,
	}
}

func nodeRow(totalVCPU, totalMemoryMB, allocatedVCPU, allocatedMemoryMB int) []any {
	now := time.Now().UTC()
	return []any{
		"node-1", "node-node-1", "10.0.0.1:50051", "10.0.0.1",
		nil, models.NodeStatusOnline, totalVCPU, totalMemoryMB,
		allocatedVCPU, allocatedMemoryMB, "vs-vms",
		nil, nil, nil,
		&now, 0, now,
		models.StorageTypeCeph, "", nil, nil,
	}
}

func templateRow(id string, isActive bool, minDiskGB int) []any {
	now := time.Now().UTC()
	return []any{
		id, "ubuntu", "linux", "22.04",
		"ubuntu-22.04", "base", minDiskGB,
		true, isActive, 1, 1, "",
		models.StorageBackendCeph, nil,
		now, now,
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

func taskRow(id, taskType, idempotencyKey string, payload json.RawMessage) []any {
	now := time.Now().UTC()
	return []any{
		id, taskType, models.TaskStatusPending, payload,
		[]byte(nil), []byte(nil), 0, 0,
		&idempotencyKey, (*string)(nil),
		now, (*time.Time)(nil), (*time.Time)(nil),
	}
}

func init() {
	_ = sharederrors.ErrConflict
}
