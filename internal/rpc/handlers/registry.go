package handlers

import (
	"fmt"
	"sync"
)

// Registry manages RPC method handlers.
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

// Register adds a handler for an RPC method.
func (r *Registry) Register(h Handler) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := h.Name()
	if _, exists := r.handlers[name]; exists {
		return fmt.Errorf("handler already registered for method: %s", name)
	}

	r.handlers[name] = h
	return nil
}

// MustRegister adds a handler and panics if registration fails.
func (r *Registry) MustRegister(h Handler) {
	if err := r.Register(h); err != nil {
		panic(err)
	}
}

// Get returns the handler for an RPC method.
func (r *Registry) Get(method string) Handler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.handlers[method]
}

// Has returns true if a handler is registered for the method.
func (r *Registry) Has(method string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.handlers[method]
	return exists
}

// Methods returns all registered method names.
func (r *Registry) Methods() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	methods := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		methods = append(methods, name)
	}
	return methods
}

// Count returns the number of registered handlers.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.handlers)
}

// GetAdminMethods returns all admin method names.
func (r *Registry) GetAdminMethods() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	methods := make([]string, 0)
	for name, h := range r.handlers {
		if h.RequiresAdmin() {
			methods = append(methods, name)
		}
	}
	return methods
}

// GetPublicMethods returns all public method names.
func (r *Registry) GetPublicMethods() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	methods := make([]string, 0)
	for name, h := range r.handlers {
		if !h.RequiresAdmin() {
			methods = append(methods, name)
		}
	}
	return methods
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
func Get(method string) Handler {
	return DefaultRegistry.Get(method)
}
