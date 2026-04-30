package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

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
	defaultSMTPPort      = "587"
	defaultSMTPFrom      = "noreply@virtuestack.com"
	defaultMaxVMsPerCust = "10"
)

var defaultSettings = []Setting{
	{Key: "maintenance_mode", Value: "false", Description: "When true, no new VMs can be created"},
	{Key: "default_backup_retention_days", Value: "30", Description: "Default backup retention period in days"},
	{Key: "max_vms_per_customer", Value: defaultMaxVMsPerCust, Description: "Maximum number of VMs a customer can create"},
	{Key: "bandwidth_overage_rate", Value: "0.05", Description: "Cost per GB for bandwidth overage in USD"},
	{Key: "smtp_host", Value: "", Description: "SMTP server hostname for email notifications"},
	{Key: "smtp_port", Value: defaultSMTPPort, Description: "SMTP server port"},
	{Key: "smtp_from", Value: defaultSMTPFrom, Description: "From email address for notifications"},
	{Key: "smtp_password", Value: "", Description: "SMTP server password for authentication"},
	{Key: "jwt_secret", Value: "", Description: "JWT signing secret (managed by system)"},
	{Key: "alert_email_recipients", Value: "", Description: "Comma-separated list of alert recipient emails"},
	{Key: "node_heartbeat_timeout_seconds", Value: "300", Description: "Seconds before a node is marked offline"},
	{Key: "backup_schedule_hour", Value: "2", Description: "Hour of day for automatic backups (0-23)"},
}

// sensitiveSettingKeys is the set of setting keys whose values must never appear in logs or audit entries.
var sensitiveSettingKeys = map[string]struct{}{
	"smtp_password": {},
	"jwt_secret":    {},
}

// isSensitiveSetting returns true if the given key holds a sensitive value.
func isSensitiveSetting(key string) bool {
	_, ok := sensitiveSettingKeys[key]
	return ok
}

// settingValueValidator is a per-key validation function.
// Returns an error message if the value is invalid, or "" when valid.
type settingValueValidator func(value string) error

// settingValidators maps setting keys to their type-specific validators.
var settingValidators = map[string]settingValueValidator{
	"maintenance_mode": func(v string) error {
		if v != "true" && v != "false" {
			return fmt.Errorf("maintenance_mode must be 'true' or 'false'")
		}
		return nil
	},
	"default_backup_retention_days": func(v string) error {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 365 {
			return fmt.Errorf("default_backup_retention_days must be an integer between 1 and 365")
		}
		return nil
	},
	"max_vms_per_customer": func(v string) error {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 10000 {
			return fmt.Errorf("max_vms_per_customer must be an integer between 1 and 10000")
		}
		return nil
	},
	"bandwidth_overage_rate": func(v string) error {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 {
			return fmt.Errorf("bandwidth_overage_rate must be a non-negative decimal number")
		}
		return nil
	},
	"smtp_port": func(v string) error {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("smtp_port must be an integer between 1 and 65535")
		}
		return nil
	},
	"node_heartbeat_timeout_seconds": func(v string) error {
		n, err := strconv.Atoi(v)
		if err != nil || n < 10 || n > 86400 {
			return fmt.Errorf("node_heartbeat_timeout_seconds must be an integer between 10 and 86400")
		}
		return nil
	},
	"backup_schedule_hour": func(v string) error {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 23 {
			return fmt.Errorf("backup_schedule_hour must be an integer between 0 and 23")
		}
		return nil
	},
}

// GetSettings handles GET /settings - retrieves all system settings.
// @Tags Admin
// @Summary Get settings
// @Description Returns all platform settings visible to admins.
// @Produce json
// @Security BearerAuth
// @Success 200 {object} models.Response
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Router /api/v1/admin/settings [get]
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
// @Tags Admin
// @Summary Update setting
// @Description Updates a platform setting by key.
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param key path string true "Setting key"
// @Param request body object true "Setting update request"
// @Success 200 {object} models.Response
// @Failure 400 {object} models.ErrorResponse
// @Failure 401 {object} models.ErrorResponse
// @Failure 403 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /api/v1/admin/settings/{key} [put]
func (h *AdminHandler) UpdateSetting(c *gin.Context) {
	key := c.Param("key")

	var req SettingUpdateRequest
	if err := middleware.BindAndValidate(c, &req); err != nil {
		var apiErr *sharederrors.APIError
		if errors.As(err, &apiErr) {
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

	// Per-key type and range validation (F-044)
	if validator, ok := settingValidators[key]; ok {
		if err := validator(req.Value); err != nil {
			middleware.RespondWithError(c, http.StatusBadRequest, "INVALID_SETTING_VALUE", err.Error())
			return
		}
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

	// Redact sensitive values before logging and audit entries (F-028)
	logValue := req.Value
	if isSensitiveSetting(key) {
		logValue = "[REDACTED]"
	}

	// Log audit event with redacted value for sensitive keys
	h.logAuditEvent(c, "setting.update", "setting", key, map[string]interface{}{
		"key":   key,
		"value": logValue,
	}, true)

	h.logger.Info("setting updated via admin API",
		"key", key,
		"value", logValue,
		"correlation_id", middleware.GetCorrelationID(c))

	c.JSON(http.StatusOK, models.Response{Data: Setting{
		Key:   key,
		Value: req.Value,
	}})
}
