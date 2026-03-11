package admin

import (
	"encoding/json"
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	"github.com/gin-gonic/gin"
)

// respondWithError sends a standardized error response.
func respondWithError(c *gin.Context, status int, code, message string) {
	resp := models.ErrorResponse{
		Error: models.ErrorDetail{
			Code:          code,
			Message:       message,
			CorrelationID: middleware.GetCorrelationID(c),
		},
	}
	c.AbortWithStatusJSON(status, resp)
}

// logAuditEvent logs an audit event for admin actions.
func (h *AdminHandler) logAuditEvent(c *gin.Context, action, resourceType, resourceID string, changes interface{}, success bool) {
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