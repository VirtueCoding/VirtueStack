package admin

import (
	"encoding/json"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/audit"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
)

type adminAuditLogEntry struct {
	actorID      string
	action       string
	resourceType string
	resourceID   string
	changes      any
	success      bool
	errorMessage string
}

// logAuditEvent logs an audit event for admin actions.
// changes accepts any JSON-marshalable value (e.g. map[string]any, a struct, or json.RawMessage).
// Pass nil when there are no changes to record.
// Sensitive fields are automatically masked before logging.
func (h *AdminHandler) logAuditEvent(c *gin.Context, action, resourceType, resourceID string, changes any, success bool) {
	h.logAuditEventWithActor(c, middleware.GetUserID(c), action, resourceType, resourceID, changes, success)
}

func (h *AdminHandler) logAuditEventWithActor(c *gin.Context, actorID, action, resourceType, resourceID string, changes any, success bool) {
	normalizedChanges := normalizeAuditChanges(changes)
	if middleware.HasAuditMiddleware(c) {
		middleware.SetAuditOverride(c, &middleware.AuditOverride{
			ActorID:      actorID,
			Action:       action,
			ResourceType: resourceType,
			ResourceID:   resourceID,
			Changes:      normalizedChanges,
		})
		return
	}

	h.appendAuditLog(c, &adminAuditLogEntry{
		actorID:      actorID,
		action:       action,
		resourceType: resourceType,
		resourceID:   resourceID,
		changes:      normalizedChanges,
		success:      success,
	})
}

func (h *AdminHandler) appendAuditLog(c *gin.Context, entry *adminAuditLogEntry) {
	if h.auditRepo == nil {
		return
	}

	correlationID := middleware.GetCorrelationID(c)
	clientIP := c.ClientIP()

	auditLog := &models.AuditLog{
		ActorID:       nil,
		ActorType:     "admin",
		ActorIP:       &clientIP,
		Action:        entry.action,
		ResourceType:  entry.resourceType,
		ResourceID:    &entry.resourceID,
		Changes:       marshalAuditChanges(entry.changes),
		CorrelationID: &correlationID,
		Success:       entry.success,
	}
	if entry.actorID != "" {
		auditLog.ActorID = &entry.actorID
	}
	if entry.errorMessage != "" {
		auditLog.ErrorMessage = &entry.errorMessage
	}

	if err := h.auditRepo.Append(c.Request.Context(), auditLog); err != nil {
		h.logger.Warn("failed to append audit log",
			"action", entry.action,
			"resource_type", entry.resourceType,
			"resource_id", entry.resourceID,
			"error", err,
			"correlation_id", correlationID)
	}
}

func normalizeAuditChanges(changes any) any {
	if changes == nil {
		return nil
	}

	if changeMap, ok := changes.(map[string]any); ok {
		return audit.MaskSensitiveFields(changeMap)
	}

	data, err := json.Marshal(changes)
	if err != nil {
		return changes
	}

	var genericChanges any
	if err := json.Unmarshal(data, &genericChanges); err != nil {
		return changes
	}

	changeMap, ok := genericChanges.(map[string]any)
	if !ok {
		return genericChanges
	}

	return audit.MaskSensitiveFields(changeMap)
}

func marshalAuditChanges(changes any) json.RawMessage {
	if changes == nil {
		return nil
	}

	data, err := json.Marshal(changes)
	if err != nil {
		return nil
	}

	return data
}
