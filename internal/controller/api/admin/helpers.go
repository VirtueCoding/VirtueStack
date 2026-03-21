package admin

import (
	"encoding/json"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
)

// logAuditEvent logs an audit event for admin actions.
// changes accepts any JSON-marshalable value (e.g. map[string]any, a struct, or json.RawMessage).
// Pass nil when there are no changes to record.
func (h *AdminHandler) logAuditEvent(c *gin.Context, action, resourceType, resourceID string, changes any, success bool) {
	actorID := middleware.GetUserID(c)
	correlationID := middleware.GetCorrelationID(c)
	clientIP := c.ClientIP()

	var changesJSON json.RawMessage
	if changes != nil {
		data, err := json.Marshal(changes)
		if err == nil {
			changesJSON = data
		}
	}

	audit := &models.AuditLog{
		ActorID:       &actorID,
		ActorType:     "admin",
		ActorIP:       &clientIP,
		Action:        action,
		ResourceType:  resourceType,
		ResourceID:    &resourceID,
		Changes:       changesJSON,
		CorrelationID: &correlationID,
		Success:       success,
	}

	if err := h.auditRepo.Append(c.Request.Context(), audit); err != nil {
		h.logger.Warn("failed to append audit log",
			"action", action,
			"resource_type", resourceType,
			"resource_id", resourceID,
			"error", err,
			"correlation_id", correlationID)
	}
}