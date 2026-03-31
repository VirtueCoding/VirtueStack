package models

import (
	"encoding/json"
	"time"
)

// In-app notification type constants.
const (
	NotifTypeBillingLowBalance     = "billing.low_balance"
	NotifTypeBillingPaymentReceived = "billing.payment_received"
	NotifTypeBillingVMSuspended    = "billing.vm_suspended"
	NotifTypeBillingInvoiceGenerated = "billing.invoice_generated"
	NotifTypeSystemMaintenance     = "system.maintenance"
	NotifTypeVMStatusChange        = "vm.status_change"
	NotifTypeBackupCompleted       = "backup.completed"
	NotifTypeBackupFailed          = "backup.failed"
)

// ValidNotificationTypes lists all supported in-app notification types.
var ValidNotificationTypes = []string{
	NotifTypeBillingLowBalance,
	NotifTypeBillingPaymentReceived,
	NotifTypeBillingVMSuspended,
	NotifTypeBillingInvoiceGenerated,
	NotifTypeSystemMaintenance,
	NotifTypeVMStatusChange,
	NotifTypeBackupCompleted,
	NotifTypeBackupFailed,
}

// InAppNotification represents a stored in-app notification for a customer or admin.
type InAppNotification struct {
	ID         string          `json:"id"`
	CustomerID *string         `json:"customer_id,omitempty"`
	AdminID    *string         `json:"admin_id,omitempty"`
	Type       string          `json:"type"`
	Title      string          `json:"title"`
	Message    string          `json:"message"`
	Data       json.RawMessage `json:"data"`
	Read       bool            `json:"read"`
	CreatedAt  time.Time       `json:"created_at"`
}

// InAppNotificationResponse is the API response representation.
type InAppNotificationResponse struct {
	ID         string          `json:"id"`
	CustomerID *string         `json:"customer_id,omitempty"`
	AdminID    *string         `json:"admin_id,omitempty"`
	Type       string          `json:"type"`
	Title      string          `json:"title"`
	Message    string          `json:"message"`
	Data       json.RawMessage `json:"data"`
	Read       bool            `json:"read"`
	CreatedAt  string          `json:"created_at"`
}

// ToResponse converts an InAppNotification to its API response form.
func (n *InAppNotification) ToResponse() *InAppNotificationResponse {
	return &InAppNotificationResponse{
		ID:         n.ID,
		CustomerID: n.CustomerID,
		AdminID:    n.AdminID,
		Type:       n.Type,
		Title:      n.Title,
		Message:    n.Message,
		Data:       n.Data,
		Read:       n.Read,
		CreatedAt:  n.CreatedAt.Format(time.RFC3339),
	}
}

// CreateInAppNotificationRequest is the input for creating an in-app notification.
type CreateInAppNotificationRequest struct {
	CustomerID *string         `json:"customer_id,omitempty" validate:"omitempty,uuid"`
	AdminID    *string         `json:"admin_id,omitempty" validate:"omitempty,uuid"`
	Type       string          `json:"type" validate:"required,max=50"`
	Title      string          `json:"title" validate:"required,max=255"`
	Message    string          `json:"message" validate:"required"`
	Data       json.RawMessage `json:"data,omitempty"`
}

// UnreadCountResponse wraps the unread notification count for the API.
type UnreadCountResponse struct {
	Count int `json:"count"`
}
