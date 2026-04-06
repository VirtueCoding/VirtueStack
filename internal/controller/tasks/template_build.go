package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// minDiskOverheadFactor adds 20% overhead to the actual template size to account
// for filesystem metadata, journaling, and cloud-init expansion at first boot.
const minDiskOverheadFactor = 1.2

// defaultMinDiskGB is the minimum disk size for any template.
const defaultMinDiskGB = 10

// handleTemplateBuild processes a template.build_from_iso task.
// It sends a BuildTemplateFromISO gRPC call to the target node agent,
// then creates a template record in the database from the result.
func handleTemplateBuild(ctx context.Context, task *models.Task, deps *HandlerDeps) error {
	logger := taskLogger(deps.Logger, task)

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

	logger.Info("starting template build from ISO",
		"template_name", payload.TemplateName,
		"os_family", payload.OSFamily,
		"iso_path", payload.ISOPath,
		"iso_url", payload.ISOURL,
		"node_id", payload.NodeID,
		"storage_backend", payload.StorageBackend)

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

	template, resp, err := resolveBuiltTemplate(ctx, deps, &payload, req)
	if err != nil {
		return err
	}
	if cacheErr := seedTemplateBuildNodeCache(ctx, deps, template, payload.NodeID, resp); cacheErr != nil {
		return fmt.Errorf("recording template cache entry: %w", cacheErr)
	}

	resultJSON, err := json.Marshal(map[string]any{
		"template_id":     template.ID,
		"template_name":   template.Name,
		"storage_backend": template.StorageBackend,
		"template_ref":    resp.TemplateRef,
		"min_disk_gb":     template.MinDiskGB,
	})
	if err != nil {
		return fmt.Errorf("marshal template build result: %w", err)
	}
	task.Result = resultJSON

	deps.Logger.Info("template build from ISO completed",
		"task_id", task.ID,
		"template_id", template.ID,
		"template_name", template.Name,
		"storage_backend", payload.StorageBackend,
		"template_ref", resp.TemplateRef)

	return nil
}

func resolveBuiltTemplate(
	ctx context.Context,
	deps *HandlerDeps,
	payload *TemplateBuildPayload,
	req *BuildTemplateFromISORequest,
) (*models.Template, *BuildTemplateFromISOResponse, error) {
	template, err := deps.TemplateRepo.GetByName(ctx, payload.TemplateName)
	if err == nil {
		resp := buildTemplateResponseFromRecord(template)
		if validateErr := validateRecoveredBuiltTemplate(template, payload, resp); validateErr != nil {
			return nil, nil, validateErr
		}
		return template, resp, nil
	}
	if !sharederrors.Is(err, sharederrors.ErrNotFound) {
		return nil, nil, fmt.Errorf("getting existing template %s: %w", payload.TemplateName, err)
	}

	resp, err := deps.NodeClient.BuildTemplateFromISO(ctx, payload.NodeID, req)
	if err != nil {
		return nil, nil, fmt.Errorf("build template from ISO on node %s: %w", payload.NodeID, err)
	}

	template = buildTemplateRecord(payload, resp)
	if err := deps.TemplateRepo.Create(ctx, template); err != nil {
		return nil, nil, fmt.Errorf("creating template record: %w", err)
	}
	return template, resp, nil
}

func buildTemplateRecord(payload *TemplateBuildPayload, resp *BuildTemplateFromISOResponse) *models.Template {
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

	if payload.StorageBackend == models.StorageBackendCeph {
		template.RBDImage = resp.TemplateRef
		template.RBDSnapshot = resp.SnapshotRef
	} else {
		template.FilePath = &resp.TemplateRef
	}
	return template
}

func buildTemplateResponseFromRecord(template *models.Template) *BuildTemplateFromISOResponse {
	resp := &BuildTemplateFromISOResponse{
		SnapshotRef: template.RBDSnapshot,
	}
	if template.StorageBackend == "" || template.StorageBackend == models.StorageBackendCeph {
		resp.TemplateRef = template.RBDImage
		return resp
	}
	if template.FilePath != nil {
		resp.TemplateRef = *template.FilePath
	}
	return resp
}

func validateRecoveredBuiltTemplate(template *models.Template, payload *TemplateBuildPayload, resp *BuildTemplateFromISOResponse) error {
	if template.StorageBackend == "" {
		template.StorageBackend = models.StorageBackendCeph
	}
	if template.StorageBackend != payload.StorageBackend {
		return fmt.Errorf("existing template %s uses storage backend %s, expected %s",
			template.Name, template.StorageBackend, payload.StorageBackend)
	}
	if template.StorageBackend == models.StorageBackendCeph {
		if resp.TemplateRef == "" || resp.SnapshotRef == "" {
			return fmt.Errorf("existing template %s is missing ceph template references", template.Name)
		}
		return nil
	}
	if resp.TemplateRef == "" {
		return fmt.Errorf("existing template %s is missing local template path", template.Name)
	}
	return nil
}

func seedTemplateBuildNodeCache(
	ctx context.Context,
	deps *HandlerDeps,
	template *models.Template,
	nodeID string,
	resp *BuildTemplateFromISOResponse,
) error {
	if template.StorageBackend == "" || template.StorageBackend == models.StorageBackendCeph {
		return nil
	}
	if deps.TemplateCacheRepo == nil {
		return fmt.Errorf("template cache repository not configured")
	}

	localPath := resp.TemplateRef
	sizeBytes := resp.SizeBytes
	now := time.Now().UTC()
	entry := &models.TemplateCacheEntry{
		TemplateID: template.ID,
		NodeID:     nodeID,
		Status:     models.TemplateCacheStatusReady,
		LocalPath:  &localPath,
		SizeBytes:  &sizeBytes,
		SyncedAt:   &now,
	}
	if err := deps.TemplateCacheRepo.Upsert(ctx, entry); err != nil {
		return err
	}
	return nil
}
