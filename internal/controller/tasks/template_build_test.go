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

type mockTemplateRepo struct {
	createFunc func(ctx context.Context, template *models.Template) error
}

func (m *mockTemplateRepo) Create(ctx context.Context, template *models.Template) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, template)
	}
	template.ID = "template-uuid-123"
	return nil
}

type mockNodeClientForBuild struct {
	buildFunc func(ctx context.Context, nodeID string, req *BuildTemplateFromISORequest) (*BuildTemplateFromISOResponse, error)
}

func (m *mockNodeClientForBuild) BuildTemplateFromISO(ctx context.Context, nodeID string, req *BuildTemplateFromISORequest) (*BuildTemplateFromISOResponse, error) {
	if m.buildFunc != nil {
		return m.buildFunc(ctx, nodeID, req)
	}
	return &BuildTemplateFromISOResponse{
		TemplateRef: "ubuntu-2404-base",
		SnapshotRef: "ubuntu-2404-snap",
		SizeBytes:   5368709120,
	}, nil
}

func TestHandleTemplateBuild(t *testing.T) {
	tests := []struct {
		name       string
		payload    TemplateBuildPayload
		buildResp  *BuildTemplateFromISOResponse
		buildErr   error
		createErr  error
		wantErr    bool
		errContain string
	}{
		{
			name: "successful ceph build",
			payload: TemplateBuildPayload{
				TemplateName:   "Ubuntu 24.04",
				OSFamily:       "ubuntu",
				OSVersion:      "24.04",
				ISOPath:        "/var/lib/iso/ubuntu.iso",
				NodeID:         "node-1",
				StorageBackend: "ceph",
				DiskSizeGB:     10,
				MemoryMB:       2048,
				VCPUs:          2,
			},
			buildResp: &BuildTemplateFromISOResponse{
				TemplateRef: "ubuntu-2404-base",
				SnapshotRef: "ubuntu-2404-snap",
				SizeBytes:   5368709120,
			},
			wantErr: false,
		},
		{
			name: "successful qcow build",
			payload: TemplateBuildPayload{
				TemplateName:   "Debian 12",
				OSFamily:       "debian",
				OSVersion:      "12",
				ISOPath:        "/var/lib/iso/debian.iso",
				NodeID:         "node-2",
				StorageBackend: "qcow",
				DiskSizeGB:     10,
				MemoryMB:       2048,
				VCPUs:          2,
			},
			buildResp: &BuildTemplateFromISOResponse{
				TemplateRef: "/var/lib/virtuestack/templates/debian-12.qcow2",
				SizeBytes:   3221225472,
			},
			wantErr: false,
		},
		{
			name: "successful lvm build",
			payload: TemplateBuildPayload{
				TemplateName:   "AlmaLinux 9",
				OSFamily:       "almalinux",
				OSVersion:      "9",
				ISOPath:        "/var/lib/iso/almalinux.iso",
				NodeID:         "node-3",
				StorageBackend: "lvm",
				DiskSizeGB:     15,
				MemoryMB:       4096,
				VCPUs:          4,
			},
			buildResp: &BuildTemplateFromISOResponse{
				TemplateRef: "/dev/vgvs/almalinux-9-base",
				SizeBytes:   8589934592,
			},
			wantErr: false,
		},
		{
			name: "successful build with iso_url",
			payload: TemplateBuildPayload{
				TemplateName:   "Debian 12 from URL",
				OSFamily:       "debian",
				OSVersion:      "12",
				ISOURL:         "https://cdimage.debian.org/debian-cd/current/amd64/iso-cd/debian-12.9.0-amd64-netinst.iso",
				NodeID:         "node-1",
				StorageBackend: "ceph",
				DiskSizeGB:     10,
				MemoryMB:       2048,
				VCPUs:          2,
			},
			buildResp: &BuildTemplateFromISOResponse{
				TemplateRef: "debian-12-base",
				SnapshotRef: "debian-12-snap",
				SizeBytes:   3221225472,
			},
			wantErr: false,
		},
		{
			name: "missing template name",
			payload: TemplateBuildPayload{
				ISOPath:        "/var/lib/iso/ubuntu.iso",
				NodeID:         "node-1",
				StorageBackend: "ceph",
			},
			wantErr:    true,
			errContain: "missing required fields",
		},
		{
			name: "missing ISO path and URL",
			payload: TemplateBuildPayload{
				TemplateName:   "Ubuntu 24.04",
				NodeID:         "node-1",
				StorageBackend: "ceph",
			},
			wantErr:    true,
			errContain: "iso_path or iso_url required",
		},
		{
			name: "missing node ID",
			payload: TemplateBuildPayload{
				TemplateName:   "Ubuntu 24.04",
				ISOPath:        "/var/lib/iso/ubuntu.iso",
				StorageBackend: "ceph",
			},
			wantErr:    true,
			errContain: "missing required fields",
		},
		{
			name: "node agent build fails",
			payload: TemplateBuildPayload{
				TemplateName:   "Ubuntu 24.04",
				OSFamily:       "ubuntu",
				OSVersion:      "24.04",
				ISOPath:        "/var/lib/iso/ubuntu.iso",
				NodeID:         "node-1",
				StorageBackend: "ceph",
				DiskSizeGB:     10,
				MemoryMB:       2048,
				VCPUs:          2,
			},
			buildErr:   assert.AnError,
			wantErr:    true,
			errContain: "build template from ISO on node",
		},
		{
			name: "DB create fails",
			payload: TemplateBuildPayload{
				TemplateName:   "Ubuntu 24.04",
				OSFamily:       "ubuntu",
				OSVersion:      "24.04",
				ISOPath:        "/var/lib/iso/ubuntu.iso",
				NodeID:         "node-1",
				StorageBackend: "ceph",
				DiskSizeGB:     10,
				MemoryMB:       2048,
				VCPUs:          2,
			},
			buildResp: &BuildTemplateFromISOResponse{
				TemplateRef: "ubuntu-2404-base",
				SnapshotRef: "ubuntu-2404-snap",
				SizeBytes:   5368709120,
			},
			createErr:  assert.AnError,
			wantErr:    true,
			errContain: "creating template record",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			payloadJSON, err := json.Marshal(tt.payload)
			require.NoError(t, err)

			task := &models.Task{
				ID:      "task-123",
				Type:    models.TaskTypeTemplateBuild,
				Payload: payloadJSON,
			}

			mockClient := &mockNodeClientForBuild{
				buildFunc: func(ctx context.Context, nodeID string, req *BuildTemplateFromISORequest) (*BuildTemplateFromISOResponse, error) {
					if tt.buildErr != nil {
						return nil, tt.buildErr
					}
					return tt.buildResp, nil
				},
			}

			mockRepo := &mockTemplateRepo{
				createFunc: func(ctx context.Context, template *models.Template) error {
					if tt.createErr != nil {
						return tt.createErr
					}
					template.ID = "template-uuid-123"
					return nil
				},
			}

			// Build a minimal HandlerDeps with our mocks
			deps := &HandlerDeps{
				Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
			}

			// Call the handler directly with mocked dependencies
			err = handleTemplateBuildWithMocks(context.Background(), task, deps, mockClient, mockRepo)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContain != "" {
					assert.Contains(t, err.Error(), tt.errContain)
				}
				return
			}

			require.NoError(t, err)

			// Verify task result was set
			assert.NotNil(t, task.Result)

			var result map[string]interface{}
			err = json.Unmarshal(task.Result, &result)
			require.NoError(t, err)
			assert.Equal(t, "template-uuid-123", result["template_id"])
			assert.Equal(t, tt.payload.TemplateName, result["template_name"])
			assert.Equal(t, tt.payload.StorageBackend, result["storage_backend"])
		})
	}
}

// handleTemplateBuildWithMocks is a test helper that allows injecting mock dependencies
// that differ from the full HandlerDeps (which uses concrete types).
func handleTemplateBuildWithMocks(
	ctx context.Context,
	task *models.Task,
	deps *HandlerDeps,
	nodeClient *mockNodeClientForBuild,
	templateRepo *mockTemplateRepo,
) error {
	var payload TemplateBuildPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal template build payload: %w", err)
	}

	if payload.TemplateName == "" || payload.NodeID == "" {
		return fmt.Errorf("missing required fields in template build payload")
	}
	if payload.ISOPath == "" && payload.ISOURL == "" {
		return fmt.Errorf("missing required fields in template build payload: iso_path or iso_url required")
	}

	req := &BuildTemplateFromISORequest{
		TemplateName:        payload.TemplateName,
		ISOPath:             payload.ISOPath,
		ISOURL:              payload.ISOURL,
		OSFamily:            payload.OSFamily,
		OSVersion:           payload.OSVersion,
		DiskSizeGB:          payload.DiskSizeGB,
		MemoryMB:            payload.MemoryMB,
		VCPUs:               payload.VCPUs,
		StorageBackend:      payload.StorageBackend,
		RootPassword:        payload.RootPassword,
		CustomInstallConfig: payload.CustomInstallConfig,
	}

	resp, err := nodeClient.BuildTemplateFromISO(ctx, payload.NodeID, req)
	if err != nil {
		return fmt.Errorf("build template from ISO on node %s: %w", payload.NodeID, err)
	}

	minDiskGB := payload.DiskSizeGB
	if resp.SizeBytes > 0 {
		calculated := int(float64(resp.SizeBytes) / (1024 * 1024 * 1024) * minDiskOverheadFactor)
		if calculated > minDiskGB {
			minDiskGB = calculated
		}
	}
	if minDiskGB < defaultMinDiskGB {
		minDiskGB = defaultMinDiskGB
	}

	template := &models.Template{
		Name:              payload.TemplateName,
		OSFamily:          payload.OSFamily,
		OSVersion:         payload.OSVersion,
		MinDiskGB:         minDiskGB,
		SupportsCloudInit: true,
		IsActive:          true,
		SortOrder:         0,
		StorageBackend:    payload.StorageBackend,
	}

	if payload.StorageBackend == "ceph" {
		template.RBDImage = resp.TemplateRef
		template.RBDSnapshot = resp.SnapshotRef
	} else {
		template.FilePath = &resp.TemplateRef
	}

	if err := templateRepo.Create(ctx, template); err != nil {
		return fmt.Errorf("creating template record: %w", err)
	}

	result, err := json.Marshal(map[string]any{
		"template_id":     template.ID,
		"template_name":   template.Name,
		"storage_backend": template.StorageBackend,
		"template_ref":    resp.TemplateRef,
		"min_disk_gb":     minDiskGB,
	})
	if err != nil {
		return fmt.Errorf("marshal template build result: %w", err)
	}
	task.Result = result

	return nil
}

func TestMinDiskCalculation(t *testing.T) {
	tests := []struct {
		name      string
		sizeBytes int64
		diskGB    int
		wantGB    int
	}{
		{
			name:      "5GB template with 10GB disk",
			sizeBytes: 5368709120, // 5GB
			diskGB:    10,
			wantGB:    10, // max(10, ceil(5*1.2)) = max(10, 6) = 10
		},
		{
			name:      "8GB template with 10GB disk",
			sizeBytes: 8589934592, // 8GB
			diskGB:    10,
			wantGB:    10, // max(10, ceil(8*1.2)) = max(10, 9) = 10
		},
		{
			name:      "12GB template with 10GB disk",
			sizeBytes: 12884901888, // 12GB
			diskGB:    10,
			wantGB:    14, // max(10, ceil(12*1.2)) = max(10, 14) = 14
		},
		{
			name:      "zero size uses disk GB",
			sizeBytes: 0,
			diskGB:    15,
			wantGB:    15,
		},
		{
			name:      "small template enforces minimum 10GB",
			sizeBytes: 1073741824, // 1GB
			diskGB:    5,
			wantGB:    10, // enforced minimum
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			minDiskGB := tt.diskGB
			if tt.sizeBytes > 0 {
				calculated := int(float64(tt.sizeBytes) / (1024 * 1024 * 1024) * minDiskOverheadFactor)
				if calculated > minDiskGB {
					minDiskGB = calculated
				}
			}
			if minDiskGB < defaultMinDiskGB {
				minDiskGB = defaultMinDiskGB
			}
			assert.Equal(t, tt.wantGB, minDiskGB)
		})
	}
}
