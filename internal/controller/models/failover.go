// Package models provides data model types for VirtueStack Controller.
package models

import (
	"encoding/json"
	"time"
)

// Failover status constants define the lifecycle states of a failover request.
const (
	// FailoverStatusPending indicates the request is awaiting admin approval.
	FailoverStatusPending = "pending"
	// FailoverStatusApproved indicates the request has been approved and is ready for execution.
	FailoverStatusApproved = "approved"
	// FailoverStatusInProgress indicates the failover operation is currently executing.
	FailoverStatusInProgress = "in_progress"
	// FailoverStatusCompleted indicates the failover operation finished successfully.
	FailoverStatusCompleted = "completed"
	// FailoverStatusFailed indicates the failover operation encountered an error.
	FailoverStatusFailed = "failed"
	// FailoverStatusCancelled indicates the request was cancelled before execution.
	FailoverStatusCancelled = "cancelled"
)

// FailoverRequest represents a request to evacuate all VMs from a failing node.
type FailoverRequest struct {
	ID          string          `json:"id" db:"id"`
	NodeID      string          `json:"node_id" db:"node_id"`
	RequestedBy string          `json:"requested_by" db:"requested_by"`
	Status      string          `json:"status" db:"status"`
	Reason      *string         `json:"reason,omitempty" db:"reason"`
	Result      json.RawMessage `json:"result,omitempty" db:"result"`
	ApprovedAt  *time.Time      `json:"approved_at,omitempty" db:"approved_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty" db:"completed_at"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
}

// FailoverRequestListFilter holds query parameters for filtering failover request list results.
type FailoverRequestListFilter struct {
	PaginationParams
	NodeID *string
	Status *string
}
