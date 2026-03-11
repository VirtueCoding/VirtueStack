// Package models provides data model types for VirtueStack Controller.
package models

import (
	"encoding/json"
	"time"
)

// Actor type constants define who performed an audited action.
const (
	AuditActorAdmin        = "admin"
	AuditActorCustomer     = "customer"
	AuditActorProvisioning = "provisioning"
	AuditActorSystem       = "system"
)

// AuditLog represents an immutable record of a significant action taken within the system.
type AuditLog struct {
	ID            string          `json:"id" db:"id"`
	Timestamp     time.Time       `json:"timestamp" db:"timestamp"`
	ActorID       *string         `json:"actor_id,omitempty" db:"actor_id"`
	ActorType     string          `json:"actor_type" db:"actor_type"`       // admin, customer, provisioning, system
	ActorIP       *string         `json:"actor_ip,omitempty" db:"actor_ip"`
	Action        string          `json:"action" db:"action"`               // e.g. vm.create, vm.start
	ResourceType  string          `json:"resource_type" db:"resource_type"` // vm, node, customer, etc.
	ResourceID    *string         `json:"resource_id,omitempty" db:"resource_id"`
	Changes       json.RawMessage `json:"changes,omitempty" db:"changes"`
	CorrelationID *string         `json:"correlation_id,omitempty" db:"correlation_id"`
	Success       bool            `json:"success" db:"success"`
	ErrorMessage  *string         `json:"error_message,omitempty" db:"error_message"`
}

// AuditLogFilter holds query parameters for filtering and paginating audit log results.
type AuditLogFilter struct {
	ActorID      *string
	ActorType    *string
	Action       *string
	ResourceType *string
	ResourceID   *string
	StartDate    *time.Time
	EndDate      *time.Time
	PaginationParams
}
