package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

// handleTemplateDistribute distributes a template to specified QCOW/LVM nodes.
// For each node, it calls EnsureTemplateCached via gRPC and updates the cache status.
func handleTemplateDistribute(ctx context.Context, task *models.Task, deps *HandlerDeps) error {
	logger := taskLogger(deps.Logger, task)

	var payload TemplateDistributePayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal template distribute payload: %w", err)
	}

	if payload.TemplateID == "" || len(payload.NodeIDs) == 0 {
		return fmt.Errorf("template_id and node_ids are required")
	}

	template, err := deps.TemplateRepo.GetByID(ctx, payload.TemplateID)
	if err != nil {
		return fmt.Errorf("getting template %s: %w", payload.TemplateID, err)
	}

	if template.StorageBackend == "ceph" {
		logger.Info("skipping distribution for ceph template (shared pool access)",
			"template_id", template.ID, "template_name", template.Name)
		return nil
	}

	sourceURL := buildTemplateSourceURL(template)
	if err := models.ValidateTemplateDistributionSourceURL(sourceURL); err != nil {
		return fmt.Errorf("invalid template distribution source: %w", err)
	}

	var distributed, failed int
	for _, nodeID := range payload.NodeIDs {
		node, nodeErr := deps.NodeRepo.GetByID(ctx, nodeID)
		if nodeErr != nil {
			logger.Warn("skipping node: not found",
				"node_id", nodeID, "error", nodeErr)
			failed++
			continue
		}
		if node.Status != "online" {
			logger.Warn("skipping offline node",
				"node_id", nodeID, "status", node.Status)
			failed++
			continue
		}

		cacheErr := ensureTemplateCachedOnNode(ctx, deps, template, nodeID, sourceURL)
		if cacheErr != nil {
			logger.Error("failed to cache template on node",
				"template_id", template.ID,
				"node_id", nodeID,
				"error", cacheErr)
			failed++
		} else {
			distributed++
		}
	}

	result, _ := json.Marshal(map[string]interface{}{
		"template_id": template.ID,
		"distributed": distributed,
		"failed":      failed,
		"total":       len(payload.NodeIDs),
	})
	task.Result = result

	if failed > 0 && distributed == 0 {
		return fmt.Errorf("failed to distribute template to all %d nodes", failed)
	}

	logger.Info("template distribution completed",
		"template_id", template.ID,
		"distributed", distributed,
		"failed", failed)

	return nil
}

// ensureTemplateCachedOnNode ensures a template is cached on a specific node.
// Updates the template_node_cache table with status transitions.
func ensureTemplateCachedOnNode(ctx context.Context, deps *HandlerDeps, template *models.Template, nodeID, sourceURL string) error {
	if deps.TemplateCacheRepo == nil {
		return fmt.Errorf("template cache repository not configured")
	}

	entry := &models.TemplateCacheEntry{
		TemplateID: template.ID,
		NodeID:     nodeID,
		Status:     models.TemplateCacheStatusDownloading,
	}
	if err := deps.TemplateCacheRepo.Upsert(ctx, entry); err != nil {
		return fmt.Errorf("creating cache entry: %w", err)
	}

	resp, err := deps.NodeClient.EnsureTemplateCached(ctx, nodeID, buildEnsureTemplateCachedRequest(template, sourceURL))
	if err != nil {
		errMsg := err.Error()
		_ = deps.TemplateCacheRepo.UpdateStatus(ctx, template.ID, nodeID,
			models.TemplateCacheStatusFailed, nil, nil, &errMsg)
		return err
	}

	localPath := resp.LocalPath
	sizeBytes := resp.SizeBytes
	return deps.TemplateCacheRepo.UpdateStatus(ctx, template.ID, nodeID,
		models.TemplateCacheStatusReady, &localPath, &sizeBytes, nil)
}

func buildEnsureTemplateCachedRequest(template *models.Template, sourceURL string) *EnsureTemplateCachedRequest {
	return &EnsureTemplateCachedRequest{
		TemplateID:     template.ID,
		TemplateName:   template.Name,
		StorageBackend: template.StorageBackend,
		SourceURL:      sourceURL,
		// The controller does not currently persist the actual downloadable artifact
		// size for distributed templates. Sending MinDiskGB here is incorrect because
		// it is the guest's virtual minimum disk size, not the QCOW/LVM image size.
		ExpectedSizeBytes: 0,
	}
}

// buildTemplateSourceURL constructs the download URL for a template.
// For now, this returns the file_path which can be served via a file server.
// In production, this would be an internal HTTP endpoint on the controller.
func buildTemplateSourceURL(template *models.Template) string {
	if template.FilePath != nil {
		return *template.FilePath
	}
	return ""
}
