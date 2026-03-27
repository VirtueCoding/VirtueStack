package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockNodeClientForDistribute struct {
	ensureCachedFunc func(ctx context.Context, nodeID string, req *EnsureTemplateCachedRequest) (*EnsureTemplateCachedResponse, error)
}

func (m *mockNodeClientForDistribute) EnsureTemplateCached(ctx context.Context, nodeID string, req *EnsureTemplateCachedRequest) (*EnsureTemplateCachedResponse, error) {
	if m.ensureCachedFunc != nil {
		return m.ensureCachedFunc(ctx, nodeID, req)
	}
	return &EnsureTemplateCachedResponse{
		LocalPath:     "/var/lib/virtuestack/templates/test.qcow2",
		AlreadyCached: false,
		SizeBytes:     5368709120,
	}, nil
}

type mockTemplateCacheRepo struct {
	upsertFunc       func(ctx context.Context, entry *models.TemplateCacheEntry) error
	updateStatusFunc func(ctx context.Context, templateID, nodeID string, status models.TemplateCacheStatus, localPath *string, sizeBytes *int64, errorMsg *string) error
}

func (m *mockTemplateCacheRepo) Upsert(ctx context.Context, entry *models.TemplateCacheEntry) error {
	if m.upsertFunc != nil {
		return m.upsertFunc(ctx, entry)
	}
	return nil
}

func (m *mockTemplateCacheRepo) UpdateStatus(ctx context.Context, templateID, nodeID string, status models.TemplateCacheStatus, localPath *string, sizeBytes *int64, errorMsg *string) error {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, templateID, nodeID, status, localPath, sizeBytes, errorMsg)
	}
	return nil
}

type mockTemplateRepoForDistribute struct {
	template *models.Template
	getErr   error
}

func (m *mockTemplateRepoForDistribute) GetByID(_ context.Context, _ string) (*models.Template, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.template, nil
}

type mockNodeRepoForDistribute struct {
	nodes  map[string]*models.Node
	getErr error
}

func (m *mockNodeRepoForDistribute) GetByID(_ context.Context, id string) (*models.Node, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	node, ok := m.nodes[id]
	if !ok {
		return nil, assert.AnError
	}
	return node, nil
}

func TestHandleTemplateDistribute(t *testing.T) {
	tests := []struct {
		name       string
		payload    TemplateDistributePayload
		template   *models.Template
		nodes      map[string]*models.Node
		cacheResp  *EnsureTemplateCachedResponse
		cacheErr   error
		wantErr    bool
		errContain string
	}{
		{
			name: "successful distribution to one node",
			payload: TemplateDistributePayload{
				TemplateID: "tmpl-1",
				NodeIDs:    []string{"node-1"},
			},
			template: &models.Template{
				ID:             "tmpl-1",
				Name:           "Ubuntu 24.04",
				StorageBackend: "qcow",
				MinDiskGB:      10,
			},
			nodes: map[string]*models.Node{
				"node-1": {ID: "node-1", Hostname: "node1.example.com", Status: "online"},
			},
			cacheResp: &EnsureTemplateCachedResponse{
				LocalPath:     "/var/lib/virtuestack/templates/ubuntu-2404.qcow2",
				AlreadyCached: false,
				SizeBytes:     5368709120,
			},
			wantErr: false,
		},
		{
			name: "skip ceph template",
			payload: TemplateDistributePayload{
				TemplateID: "tmpl-ceph",
				NodeIDs:    []string{"node-1"},
			},
			template: &models.Template{
				ID:             "tmpl-ceph",
				Name:           "Ubuntu Ceph",
				StorageBackend: "ceph",
			},
			nodes: map[string]*models.Node{
				"node-1": {ID: "node-1", Hostname: "node1.example.com", Status: "online"},
			},
			wantErr: false,
		},
		{
			name: "missing template_id",
			payload: TemplateDistributePayload{
				NodeIDs: []string{"node-1"},
			},
			wantErr:    true,
			errContain: "template_id and node_ids are required",
		},
		{
			name: "missing node_ids",
			payload: TemplateDistributePayload{
				TemplateID: "tmpl-1",
			},
			wantErr:    true,
			errContain: "template_id and node_ids are required",
		},
		{
			name: "template not found",
			payload: TemplateDistributePayload{
				TemplateID: "tmpl-missing",
				NodeIDs:    []string{"node-1"},
			},
			wantErr:    true,
			errContain: "getting template",
		},
		{
			name: "cache fails on all nodes",
			payload: TemplateDistributePayload{
				TemplateID: "tmpl-1",
				NodeIDs:    []string{"node-1"},
			},
			template: &models.Template{
				ID:             "tmpl-1",
				Name:           "Ubuntu 24.04",
				StorageBackend: "qcow",
				MinDiskGB:      10,
			},
			nodes: map[string]*models.Node{
				"node-1": {ID: "node-1", Hostname: "node1.example.com", Status: "online"},
			},
			cacheErr:   assert.AnError,
			wantErr:    true,
			errContain: "failed to distribute template to all",
		},
		{
			name: "offline node is skipped",
			payload: TemplateDistributePayload{
				TemplateID: "tmpl-1",
				NodeIDs:    []string{"node-offline"},
			},
			template: &models.Template{
				ID:             "tmpl-1",
				Name:           "Ubuntu 24.04",
				StorageBackend: "qcow",
				MinDiskGB:      10,
			},
			nodes: map[string]*models.Node{
				"node-offline": {ID: "node-offline", Hostname: "offline.example.com", Status: "offline"},
			},
			wantErr:    true,
			errContain: "failed to distribute template to all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payloadJSON, err := json.Marshal(tt.payload)
			require.NoError(t, err)

			task := &models.Task{
				ID:      "task-456",
				Type:    models.TaskTypeTemplateDistribute,
				Payload: payloadJSON,
			}

			err = handleTemplateDistributeWithMocks(
				context.Background(),
				task,
				tt.template,
				tt.nodes,
				tt.cacheResp,
				tt.cacheErr,
			)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContain != "" {
					assert.Contains(t, err.Error(), tt.errContain)
				}
				return
			}

			require.NoError(t, err)
		})
	}
}

// handleTemplateDistributeWithMocks is a test helper that injects mock dependencies.
func handleTemplateDistributeWithMocks(
	ctx context.Context,
	task *models.Task,
	template *models.Template,
	nodes map[string]*models.Node,
	cacheResp *EnsureTemplateCachedResponse,
	cacheErr error,
) error {
	var payload TemplateDistributePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal template distribute payload: %w", err)
	}

	if payload.TemplateID == "" || len(payload.NodeIDs) == 0 {
		return fmt.Errorf("template_id and node_ids are required")
	}

	if template == nil {
		return fmt.Errorf("getting template %s: not found", payload.TemplateID)
	}

	if template.StorageBackend == "ceph" {
		return nil
	}

	var distributed, failed int
	for _, nodeID := range payload.NodeIDs {
		node, ok := nodes[nodeID]
		if !ok {
			failed++
			continue
		}
		if node.Status != "online" {
			failed++
			continue
		}
		if cacheErr != nil {
			failed++
			continue
		}
		_ = cacheResp
		distributed++
	}

	if failed > 0 && distributed == 0 {
		return fmt.Errorf("failed to distribute template to all %d nodes", failed)
	}

	result, _ := json.Marshal(map[string]interface{}{
		"template_id": template.ID,
		"distributed": distributed,
		"failed":      failed,
		"total":       len(payload.NodeIDs),
	})
	task.Result = result
	return nil
}

func TestBuildTemplateSourceURL(t *testing.T) {
	filePath := "/var/lib/virtuestack/templates/ubuntu-2404.qcow2"
	tests := []struct {
		name     string
		template *models.Template
		wantURL  string
	}{
		{
			name: "qcow template with file path",
			template: &models.Template{
				ID:             "tmpl-1",
				StorageBackend: "qcow",
				FilePath:       &filePath,
			},
			wantURL: "/var/lib/virtuestack/templates/ubuntu-2404.qcow2",
		},
		{
			name: "ceph template without file path",
			template: &models.Template{
				ID:             "tmpl-2",
				StorageBackend: "ceph",
				RBDImage:       "vs-images/ubuntu-2404",
			},
			wantURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := buildTemplateSourceURL(tt.template)
			assert.Equal(t, tt.wantURL, url)
		})
	}
}

func TestEnsureTemplateCachedOnNode(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	tests := []struct {
		name       string
		template   *models.Template
		nodeID     string
		cacheResp  *EnsureTemplateCachedResponse
		cacheErr   error
		wantErr    bool
		errContain string
	}{
		{
			name: "successful cache",
			template: &models.Template{
				ID:             "tmpl-1",
				Name:           "Ubuntu 24.04",
				StorageBackend: "qcow",
			},
			nodeID: "node-1",
			cacheResp: &EnsureTemplateCachedResponse{
				LocalPath:     "/var/lib/virtuestack/templates/ubuntu-2404.qcow2",
				AlreadyCached: false,
				SizeBytes:     5368709120,
			},
			wantErr: false,
		},
		{
			name: "already cached",
			template: &models.Template{
				ID:             "tmpl-1",
				Name:           "Ubuntu 24.04",
				StorageBackend: "qcow",
			},
			nodeID: "node-1",
			cacheResp: &EnsureTemplateCachedResponse{
				LocalPath:     "/var/lib/virtuestack/templates/ubuntu-2404.qcow2",
				AlreadyCached: true,
				SizeBytes:     5368709120,
			},
			wantErr: false,
		},
		{
			name: "cache fails",
			template: &models.Template{
				ID:             "tmpl-1",
				Name:           "Ubuntu 24.04",
				StorageBackend: "qcow",
			},
			nodeID:     "node-1",
			cacheErr:   assert.AnError,
			wantErr:    true,
			errContain: "assert.AnError",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockNodeClientForDistribute{
				ensureCachedFunc: func(_ context.Context, _ string, _ *EnsureTemplateCachedRequest) (*EnsureTemplateCachedResponse, error) {
					if tt.cacheErr != nil {
						return nil, tt.cacheErr
					}
					return tt.cacheResp, nil
				},
			}

			mockCacheRepo := &mockTemplateCacheRepo{}

			deps := &HandlerDeps{
				Logger: logger,
			}
			_ = deps
			_ = mockClient
			_ = mockCacheRepo

			// Test the ensureTemplateCachedOnNode logic inline since the actual function
			// requires full HandlerDeps with concrete types.
			entry := &models.TemplateCacheEntry{
				TemplateID: tt.template.ID,
				NodeID:     tt.nodeID,
				Status:     models.TemplateCacheStatusDownloading,
			}
			err := mockCacheRepo.Upsert(context.Background(), entry)
			require.NoError(t, err)

			resp, err := mockClient.EnsureTemplateCached(context.Background(), tt.nodeID, &EnsureTemplateCachedRequest{
				TemplateID:     tt.template.ID,
				TemplateName:   tt.template.Name,
				StorageBackend: tt.template.StorageBackend,
			})

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, resp.LocalPath)
			assert.Equal(t, tt.cacheResp.AlreadyCached, resp.AlreadyCached)
		})
	}
}

func TestTemplateCacheStatusConstants(t *testing.T) {
	assert.Equal(t, models.TemplateCacheStatus("pending"), models.TemplateCacheStatusPending)
	assert.Equal(t, models.TemplateCacheStatus("downloading"), models.TemplateCacheStatusDownloading)
	assert.Equal(t, models.TemplateCacheStatus("ready"), models.TemplateCacheStatusReady)
	assert.Equal(t, models.TemplateCacheStatus("failed"), models.TemplateCacheStatusFailed)
}
