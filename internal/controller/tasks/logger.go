package tasks

import (
	"encoding/json"
	"log/slog"

	"github.com/AbuGosok/VirtueStack/internal/controller/models"
)

func taskLogger(base *slog.Logger, task *models.Task) *slog.Logger {
	logger := base.With(
		"task_id", task.ID,
		"task_type", task.Type,
	)

	addTaskPayloadFields(&logger, task)

	return logger
}

func addTaskPayloadFields(logger **slog.Logger, task *models.Task) {
	fields := map[string][]string{
		"vm_id":          {"vm_id", "vmId"},
		"node_id":        {"node_id", "nodeId"},
		"template_id":    {"template_id", "templateId"},
		"snapshot_id":    {"snapshot_id", "snapshotId"},
		"backup_id":      {"backup_id", "backupId"},
		"delivery_id":    {"delivery_id", "deliveryId"},
		"webhook_id":     {"webhook_id", "webhookId"},
		"source_node_id": {"source_node_id", "sourceNodeId"},
		"target_node_id": {"target_node_id", "targetNodeId"},
	}

	for field, keys := range fields {
		if value, ok := payloadString(task.Payload, keys...); ok {
			*logger = (*logger).With(field, value)
		}
	}
}

func payloadString(payload json.RawMessage, keys ...string) (string, bool) {
	if len(payload) == 0 {
		return "", false
	}

	var payloadMap map[string]any
	if err := json.Unmarshal(payload, &payloadMap); err != nil {
		return "", false
	}

	for _, key := range keys {
		if value, ok := payloadMap[key]; ok {
			if str, ok := value.(string); ok && str != "" {
				return str, true
			}
		}
	}

	return "", false
}
