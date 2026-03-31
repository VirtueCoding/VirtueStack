package billing

// RegistryHookAdapter wraps a Registry to satisfy the BillingHookResolver
// interface expected by VMService.
type RegistryHookAdapter struct {
	registry *Registry
}

func NewRegistryHookAdapter(r *Registry) *RegistryHookAdapter {
	return &RegistryHookAdapter{registry: r}
}

func (a *RegistryHookAdapter) ForCustomer(providerName string) (VMLifecycleHook, error) {
	return a.registry.ForCustomer(providerName)
}
