package billing

import "context"

// VMLifecycleHook is the subset of BillingProvider consumed by VMService.
type VMLifecycleHook interface {
	OnVMCreated(ctx context.Context, vm VMRef) error
	OnVMDeleted(ctx context.Context, vm VMRef) error
	OnVMResized(ctx context.Context, vm VMRef, oldPlanID, newPlanID string) error
}
