package handler

import (
	"fmt"
	"sync"
)

// Registry manages transaction handlers by type.
// It provides thread-safe registration and lookup of handlers.
type Registry struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

// NewRegistry creates a new handler registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]Handler),
	}
}

// Register adds a handler for a transaction type.
// Returns an error if a handler is already registered for that type.
func (r *Registry) Register(h Handler) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	txType := h.TransactionType()
	if _, exists := r.handlers[txType]; exists {
		return fmt.Errorf("handler already registered for transaction type: %s", txType)
	}

	r.handlers[txType] = h
	return nil
}

// MustRegister adds a handler and panics if registration fails.
// Useful for init() functions.
func (r *Registry) MustRegister(h Handler) {
	if err := r.Register(h); err != nil {
		panic(err)
	}
}

// Get returns the handler for a transaction type.
// Returns nil if no handler is registered.
func (r *Registry) Get(txType string) Handler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.handlers[txType]
}

// Has returns true if a handler is registered for the transaction type.
func (r *Registry) Has(txType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.handlers[txType]
	return exists
}

// Types returns all registered transaction types.
func (r *Registry) Types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.handlers))
	for t := range r.handlers {
		types = append(types, t)
	}
	return types
}

// Count returns the number of registered handlers.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.handlers)
}

// DefaultRegistry is the global handler registry.
var DefaultRegistry = NewRegistry()

// Register adds a handler to the default registry.
func Register(h Handler) error {
	return DefaultRegistry.Register(h)
}

// MustRegister adds a handler to the default registry, panicking on error.
func MustRegister(h Handler) {
	DefaultRegistry.MustRegister(h)
}

// Get returns a handler from the default registry.
func Get(txType string) Handler {
	return DefaultRegistry.Get(txType)
}
