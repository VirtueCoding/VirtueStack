package payments

import (
	"fmt"
	"sync"

	sharederrors "github.com/AbuGosok/VirtueStack/internal/shared/errors"
)

// PaymentRegistry manages registered payment providers.
type PaymentRegistry struct {
	mu        sync.RWMutex
	providers map[string]PaymentProvider
}

// NewPaymentRegistry creates an empty PaymentRegistry.
func NewPaymentRegistry() *PaymentRegistry {
	return &PaymentRegistry{
		providers: make(map[string]PaymentProvider),
	}
}

// Register adds a payment provider to the registry.
func (r *PaymentRegistry) Register(name string, p PaymentProvider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("payment provider %q already registered", name)
	}
	r.providers[name] = p
	return nil
}

// Get retrieves a payment provider by name.
// Returns ErrNotFound if the provider is not registered.
func (r *PaymentRegistry) Get(name string) (PaymentProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("payment provider %q: %w", name, sharederrors.ErrNotFound)
	}
	return p, nil
}

// Available returns the names of all registered providers.
func (r *PaymentRegistry) Available() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}
