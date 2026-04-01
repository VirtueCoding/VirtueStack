package billing

import (
	"fmt"
	"log/slog"
	"sync"
)

// Registry manages all enabled billing providers and routes lifecycle
// events to the correct provider based on each customer's billing_provider column.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]BillingProvider
	primary   string
	logger    *slog.Logger
}

func NewRegistry(primary string, logger *slog.Logger) *Registry {
	return &Registry{
		providers: make(map[string]BillingProvider),
		primary:   primary,
		logger:    logger.With("component", "billing-registry"),
	}
}

func (r *Registry) Register(p BillingProvider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := p.Name()
	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("billing provider %q already registered", name)
	}
	r.providers[name] = p
	r.logger.Info("billing provider registered", "provider", name)
	return nil
}

func (r *Registry) ForCustomer(providerName string) (BillingProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("billing provider %q not registered", providerName)
	}
	return p, nil
}

func (r *Registry) Primary() BillingProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.primary == "" {
		return nil
	}
	return r.providers[r.primary]
}

func (r *Registry) All() []BillingProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]BillingProvider, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, p)
	}
	return result
}

func (r *Registry) HasProvider(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.providers[name]
	return ok
}
