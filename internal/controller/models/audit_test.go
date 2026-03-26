package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAuditActorConstants(t *testing.T) {
	assert.Equal(t, "admin", AuditActorAdmin)
	assert.Equal(t, "customer", AuditActorCustomer)
	assert.Equal(t, "provisioning", AuditActorProvisioning)
	assert.Equal(t, "system", AuditActorSystem)
}

func TestAuditActorConstants_Unique(t *testing.T) {
	actors := []string{
		AuditActorAdmin,
		AuditActorCustomer,
		AuditActorProvisioning,
		AuditActorSystem,
	}

	seen := make(map[string]bool)
	for _, a := range actors {
		assert.False(t, seen[a], "audit actor %q should be unique", a)
		seen[a] = true
	}
	assert.Len(t, seen, 4, "should have exactly 4 audit actor types")
}

func TestAuditLog_Fields(t *testing.T) {
	actorID := "admin-123"
	actorIP := "192.168.1.1"
	resourceID := "vm-456"
	corrID := "req-abc"

	log := AuditLog{
		ID:            "audit-789",
		Timestamp:     time.Now(),
		ActorID:       &actorID,
		ActorType:     AuditActorAdmin,
		ActorIP:       &actorIP,
		Action:        "vm.create",
		ResourceType:  "vm",
		ResourceID:    &resourceID,
		CorrelationID: &corrID,
		Success:       true,
	}

	assert.Equal(t, "audit-789", log.ID)
	assert.Equal(t, &actorID, log.ActorID)
	assert.Equal(t, AuditActorAdmin, log.ActorType)
	assert.Equal(t, "vm.create", log.Action)
	assert.Equal(t, "vm", log.ResourceType)
	assert.True(t, log.Success)
}

func TestAuditLogFilter_Fields(t *testing.T) {
	actorType := "admin"
	action := "vm.create"
	resourceType := "vm"
	success := true
	start := time.Now().Add(-24 * time.Hour)
	end := time.Now()

	filter := AuditLogFilter{
		ActorType:    &actorType,
		Action:       &action,
		ResourceType: &resourceType,
		Success:      &success,
		StartTime:    &start,
		EndTime:      &end,
	}

	assert.Equal(t, &actorType, filter.ActorType)
	assert.Equal(t, &action, filter.Action)
	assert.Equal(t, &resourceType, filter.ResourceType)
	assert.Equal(t, &success, filter.Success)
	assert.NotNil(t, filter.StartTime)
	assert.NotNil(t, filter.EndTime)
}
