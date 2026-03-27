package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
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
	var payload TemplateBuildPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal template build payload: %w", err)
	}

	if payload.TemplateName == "" || payload.ISOPath == "" || payload.NodeID == "" {
		return fmt.Errorf("missing required fields in template build payload")
	}

	deps.Logger.Info("starting template build from ISO",
		"task_id", task.ID,
		"template_name", payload.TemplateName,
		"os_family", payload.OSFamily,
		"iso_path", payload.ISOPath,
		"node_id", payload.NodeID,
		"storage_backend", payload.StorageBackend)

	req := &BuildTemplateFromISORequest{
		TemplateName:        payload.TemplateName,
		ISOPath:             payload.ISOPath,
		OSFamily:            payload.OSFamily,
		OSVersion:           payload.OSVersion,
		DiskSizeGB:          payload.DiskSizeGB,
		MemoryMB:            payload.MemoryMB,
		VCPUs:               payload.VCPUs,
		StorageBackend:      payload.StorageBackend,
		RootPassword:        payload.RootPassword,
		CustomInstallConfig: payload.CustomInstallConfig,
	}

	resp, err := deps.NodeClient.BuildTemplateFromISO(ctx, payload.NodeID, req)
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

	if err := deps.TemplateRepo.Create(ctx, template); err != nil {
		return fmt.Errorf("creating template record: %w", err)
	}

	result, _ := json.Marshal(map[string]interface{}{
		"template_id":     template.ID,
		"template_name":   template.Name,
		"storage_backend": template.StorageBackend,
		"template_ref":    resp.TemplateRef,
		"min_disk_gb":     minDiskGB,
	})
	task.Result = result

	deps.Logger.Info("template build from ISO completed",
		"task_id", task.ID,
		"template_id", template.ID,
		"template_name", template.Name,
		"storage_backend", payload.StorageBackend,
		"template_ref", resp.TemplateRef)

	return nil
}
