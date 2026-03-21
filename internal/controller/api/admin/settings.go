package admin

import (
	"net/http"

	"github.com/AbuGosok/VirtueStack/internal/controller/api/middleware"
	"github.com/AbuGosok/VirtueStack/internal/controller/models"
	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
	"github.com/gin-gonic/gin"
)

// Setting represents a system setting key-value pair.
type Setting struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
}

// SettingUpdateRequest represents the request body for updating a setting.
type SettingUpdateRequest struct {
	Value string `json:"value" validate:"required"`
}

// Default values for system settings. These are used when no database override exists.
// SMTP defaults align with common submission port conventions (RFC 6409).
const (
	defaultSMTPPort       = "587"
	defaultSMTPFrom       = "noreply@virtuestack.com"
	defaultMaxVMsPerCust  = "10"
)

var defaultSettings = []Setting{
	{Key: "maintenance_mode", Value: "false", Description: "When true, no new VMs can be created"},
	{Key: "default_backup_retention_days", Value: "30", Description: "Default backup retention period in days"},
	{Key: "max_vms_per_customer", Value: defaultMaxVMsPerCust, Description: "Maximum number of VMs a customer can create"},
	{Key: "bandwidth_overage_rate", Value: "0.05", Description: "Cost per GB for bandwidth overage in USD"},
	{Key: "smtp_host", Value: "", Description: "SMTP server hostname for email notifications"},
	{Key: "smtp_port", Value: defaultSMTPPort, Description: "SMTP server port"},
	{Key: "smtp_from", Value: defaultSMTPFrom, Description: "From email address for notifications"},
	{Key: "alert_email_recipients", Value: "", Description: "Comma-separated list of alert recipient emails"},
	{Key: "node_heartbeat_timeout_seconds", Value: "300", Description: "Seconds before a node is marked offline"},
	{Key: "backup_schedule_hour", Value: "2", Description: "Hour of day for automatic backups (0-23)"},
}

// GetSettings handles GET /settings - retrieves all system settings.
func (h *AdminHandler) GetSettings(c *gin.Context) {
	settings := make([]Setting, 0, len(defaultSettings))
	for _, s := range defaultSettings {
		settings = append(settings, s)
	}

	if h.settingsRepo != nil {
		stored, err := h.settingsRepo.List(c.Request.Context())
		if err != nil {
			h.logger.Error("failed to list settings",
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			middleware.RespondWithError(c, http.StatusInternalServerError, "SETTINGS_LIST_FAILED", "Failed to retrieve settings")
			return
		}

		for i := range settings {
			for _, dbSetting := range stored {
				if settings[i].Key == dbSetting.Key {
					settings[i].Value = dbSetting.Value
					if dbSetting.Description != "" {
						settings[i].Description = dbSetting.Description
					}
					break
				}
			}
		}
	}

	c.JSON(http.StatusOK, models.Response{Data: settings})
}

// UpdateSetting handles PUT /settings/:key - updates a specific setting.
func (h *AdminHandler) UpdateSetting(c *gin.Context) {
	key := c.Param("key")

	var req SettingUpdateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		if apiErr, ok := err.(*sharederrors.APIError); ok {
			middleware.RespondWithError(c, apiErr.HTTPStatus, apiErr.Code, apiErr.Message)
			return
		}
		middleware.RespondWithError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request")
		return
	}

	// Validate that the setting key exists
	validKey := false
	for _, s := range defaultSettings {
		if s.Key == key {
			validKey = true
			break
		}
	}

	if !validKey {
		middleware.RespondWithError(c, http.StatusNotFound, "SETTING_NOT_FOUND", "Setting key not found")
		return
	}

	if h.settingsRepo != nil {
		if err := h.settingsRepo.Upsert(c.Request.Context(), key, req.Value); err != nil {
			h.logger.Error("failed to persist setting",
				"key", key,
				"error", err,
				"correlation_id", middleware.GetCorrelationID(c))
			middleware.RespondWithError(c, http.StatusInternalServerError, "SETTING_UPDATE_FAILED", "Failed to persist setting")
			return
		}
	}

	// Log audit event
	h.logAuditEvent(c, "setting.update", "setting", key, map[string]interface{}{
		"key":   key,
		"value": req.Value,
	}, true)

	h.logger.Info("setting updated via admin API",
		"key", key,
		"value", req.Value,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: Setting{
		Key:   key,
		Value: req.Value,
	}})
}
