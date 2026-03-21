// Package services provides business logic services for VirtueStack Controller.
package services

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// mockAdminBackupScheduleRepo is a mock implementation for testing.
// It implements the methods needed by AdminBackupScheduleService.
type mockAdminBackupScheduleRepo struct {
	schedules []models.AdminBackupSchedule
	byID      map[string]*models.AdminBackupSchedule
	err       error
	lastID    string
}

func (m *mockAdminBackupScheduleRepo) Create(ctx context.Context, schedule *models.AdminBackupSchedule) error {
	if m.err != nil {
		return m.err
	}
	schedule.ID = "test-schedule-id"
	m.lastID = schedule.ID
	return nil
}

func (m *mockAdminBackupScheduleRepo) GetByID(ctx context.Context, id string) (*models.AdminBackupSchedule, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.byID != nil {
		if s, ok := m.byID[id]; ok {
			return s, nil
		}
	}
	return nil, pgx.ErrNoRows
}

func (m *mockAdminBackupScheduleRepo) List(ctx context.Context, filter interface{}) ([]models.AdminBackupSchedule, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	return m.schedules, len(m.schedules), nil
}

func (m *mockAdminBackupScheduleRepo) ListDueSchedules(ctx context.Context, now time.Time) ([]models.AdminBackupSchedule, error) {
	if m.err != nil {
		return nil, m.err
	}
	var due []models.AdminBackupSchedule
	for _, s := range m.schedules {
		if s.Active && s.NextRunAt.Before(now) {
			due = append(due, s)
		}
	}
	return due, nil
}

func (m *mockAdminBackupScheduleRepo) Update(ctx context.Context, schedule *models.AdminBackupSchedule) error {
	if m.err != nil {
		return m.err
	}
	return nil
}

func (m *mockAdminBackupScheduleRepo) UpdateNextRunAt(ctx context.Context, id string, nextRunAt, lastRunAt time.Time) error {
	if m.err != nil {
		return m.err
	}
	return nil
}

func (m *mockAdminBackupScheduleRepo) Delete(ctx context.Context, id string) error {
	if m.err != nil {
		return m.err
	}
	return nil
}

// mockVMRepoForSchedule is a mock VM repository for schedule testing.
type mockVMRepoForSchedule struct {
	vms []models.VM
	err error
}

func (m *mockVMRepoForSchedule) ListAllActive(ctx context.Context) ([]models.VM, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.vms, nil
}

func (m *mockVMRepoForSchedule) List(ctx context.Context, filter models.VMListFilter) ([]models.VM, int, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	return m.vms, len(m.vms), nil
}

// mockTaskPublisherForSchedule is a mock task publisher.
type mockTaskPublisherForSchedule struct {
	published []map[string]any
	err       error
}

func (m *mockTaskPublisherForSchedule) PublishTask(ctx context.Context, taskType string, payload map[string]any) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	m.published = append(m.published, payload)
	return "task-id", nil
}

func testScheduleLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

// TestCalculateNextRunTime tests the calculateNextRunTime helper function.
func TestCalculateNextRunTime(t *testing.T) {
	now := time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		frequency string
		expected  time.Time
	}{
		{
			name:      "daily",
			frequency: models.AdminBackupScheduleFrequencyDaily,
			expected:  time.Date(2024, 3, 16, 10, 0, 0, 0, time.UTC),
		},
		{
			name:      "weekly",
			frequency: models.AdminBackupScheduleFrequencyWeekly,
			expected:  time.Date(2024, 3, 22, 10, 0, 0, 0, time.UTC),
		},
		{
			name:      "monthly",
			frequency: models.AdminBackupScheduleFrequencyMonthly,
			expected:  time.Date(2024, 4, 15, 10, 0, 0, 0, time.UTC),
		},
		{
			name:      "unknown_defaults_to_daily",
			frequency: "unknown",
			expected:  time.Date(2024, 3, 16, 10, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateNextRunTime(tt.frequency, now)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGenerateAdminBackupName tests the generateAdminBackupName helper function.
func TestGenerateAdminBackupName(t *testing.T) {
	scheduleName := "Daily Backups"
	now := time.Date(2024, 3, 15, 10, 30, 45, 0, time.UTC)

	result := generateAdminBackupName(scheduleName, now)

	expected := "Daily Backups-20240315-103045"
	assert.Equal(t, expected, result)
}

// TestMockRepoListDueSchedules tests the mock repository behavior.
func TestMockRepoListDueSchedules(t *testing.T) {
	now := time.Date(2024, 3, 15, 12, 0, 0, 0, time.UTC)

	pastTime := time.Date(2024, 3, 15, 10, 0, 0, 0, time.UTC)
	futureTime := time.Date(2024, 3, 16, 10, 0, 0, 0, time.UTC)

	mockRepo := &mockAdminBackupScheduleRepo{
		schedules: []models.AdminBackupSchedule{
			{ID: "due-1", Active: true, NextRunAt: pastTime},
			{ID: "not-due", Active: true, NextRunAt: futureTime},
			{ID: "due-2", Active: true, NextRunAt: pastTime},
			{ID: "inactive", Active: false, NextRunAt: pastTime},
		},
	}

	due, err := mockRepo.ListDueSchedules(context.Background(), now)
	require.NoError(t, err)

	// Should only return active schedules with next_run_at before now
	assert.Len(t, due, 2)
	assert.ElementsMatch(t, []string{"due-1", "due-2"}, []string{due[0].ID, due[1].ID})
}

// TestMockVMRepoList tests the mock VM repository behavior.
func TestMockVMRepoList(t *testing.T) {
	vms := []models.VM{
		{ID: "vm-1", Status: models.VMStatusRunning, PlanID: "plan-1"},
		{ID: "vm-2", Status: models.VMStatusStopped, PlanID: "plan-2"},
	}

	mockRepo := &mockVMRepoForSchedule{vms: vms}

	result, err := mockRepo.ListAllActive(context.Background())
	require.NoError(t, err)
	assert.Len(t, result, 2)
}

// TestMockTaskPublisher tests the mock task publisher behavior.
func TestMockTaskPublisher(t *testing.T) {
	mockPublisher := &mockTaskPublisherForSchedule{}

	taskID, err := mockPublisher.PublishTask(context.Background(), "backup.create", map[string]any{"vm_id": "vm-1"})
	require.NoError(t, err)
	assert.Equal(t, "task-id", taskID)
	assert.Len(t, mockPublisher.published, 1)
	assert.Equal(t, "vm-1", mockPublisher.published[0]["vm_id"])

	// Test error case
	mockPublisher.err = assert.AnError
	_, err = mockPublisher.PublishTask(context.Background(), "backup.create", nil)
	assert.Error(t, err)
}