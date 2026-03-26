package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFailoverStatusConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		want     string
	}{
		{"pending", FailoverStatusPending, "pending"},
		{"approved", FailoverStatusApproved, "approved"},
		{"in progress", FailoverStatusInProgress, "in_progress"},
		{"completed", FailoverStatusCompleted, "completed"},
		{"failed", FailoverStatusFailed, "failed"},
		{"cancelled", FailoverStatusCancelled, "cancelled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.constant)
		})
	}
}

func TestFailoverStatusConstants_Unique(t *testing.T) {
	statuses := []string{
		FailoverStatusPending,
		FailoverStatusApproved,
		FailoverStatusInProgress,
		FailoverStatusCompleted,
		FailoverStatusFailed,
		FailoverStatusCancelled,
	}

	seen := make(map[string]bool)
	for _, s := range statuses {
		assert.False(t, seen[s], "failover status %q should be unique", s)
		seen[s] = true
	}
	assert.Len(t, seen, 6, "should have exactly 6 failover statuses")
}

func TestFailoverRequest_Fields(t *testing.T) {
	req := FailoverRequest{
		ID:          "fo-123",
		NodeID:      "node-456",
		RequestedBy: "admin-789",
		Status:      FailoverStatusPending,
	}

	assert.Equal(t, "fo-123", req.ID)
	assert.Equal(t, "node-456", req.NodeID)
	assert.Equal(t, "admin-789", req.RequestedBy)
	assert.Equal(t, FailoverStatusPending, req.Status)
}

func TestFailoverRequestListFilter_Fields(t *testing.T) {
	nodeID := "node-123"
	status := "pending"

	filter := FailoverRequestListFilter{
		NodeID: &nodeID,
		Status: &status,
	}

	assert.Equal(t, &nodeID, filter.NodeID)
	assert.Equal(t, &status, filter.Status)
}
