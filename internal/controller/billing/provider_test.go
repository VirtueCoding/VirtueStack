package billing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVMRef_Fields(t *testing.T) {
	ref := VMRef{
		ID:         "vm-123",
		CustomerID: "cust-456",
		PlanID:     "plan-789",
		Hostname:   "test-vm",
	}
	assert.Equal(t, "vm-123", ref.ID)
	assert.Equal(t, "cust-456", ref.CustomerID)
	assert.Equal(t, "plan-789", ref.PlanID)
	assert.Equal(t, "test-vm", ref.Hostname)
}

func TestCreateUserRequest_Fields(t *testing.T) {
	req := CreateUserRequest{
		CustomerID: "cust-123",
		Email:      "user@example.com",
		Name:       "Test User",
	}
	assert.Equal(t, "cust-123", req.CustomerID)
	assert.Equal(t, "user@example.com", req.Email)
	assert.Equal(t, "Test User", req.Name)
}

func TestPaginationOpts_Defaults(t *testing.T) {
	opts := PaginationOpts{}
	assert.Equal(t, 0, opts.Page)
	assert.Equal(t, 0, opts.PerPage)
}

func TestBalance_Fields(t *testing.T) {
	b := Balance{
		CustomerID:   "cust-123",
		BalanceCents: 5000,
		Currency:     "USD",
	}
	assert.Equal(t, "cust-123", b.CustomerID)
	assert.Equal(t, int64(5000), b.BalanceCents)
	assert.Equal(t, "USD", b.Currency)
}

func TestTopUpRequest_Fields(t *testing.T) {
	req := TopUpRequest{
		CustomerID:  "cust-123",
		AmountCents: 1000,
		Currency:    "USD",
		Reference:   "stripe_pi_abc",
	}
	assert.Equal(t, "cust-123", req.CustomerID)
	assert.Equal(t, int64(1000), req.AmountCents)
	assert.Equal(t, "USD", req.Currency)
	assert.Equal(t, "stripe_pi_abc", req.Reference)
}

func TestUsageHistory_Empty(t *testing.T) {
	h := UsageHistory{
		Records:    []UsageRecord{},
		TotalCount: 0,
		Page:       1,
		PerPage:    20,
	}
	assert.Empty(t, h.Records)
	assert.Equal(t, 0, h.TotalCount)
	assert.Equal(t, 1, h.Page)
	assert.Equal(t, 20, h.PerPage)
}
