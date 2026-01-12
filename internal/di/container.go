// Package di provides dependency injection infrastructure for the goXRPLd application.
package di

import (
	"errors"
	"sync"
)

// Container is the dependency injection container.
// It manages service registration and resolution.
type Container struct {
	mu       sync.RWMutex
	services map[string]interface{}
	builders map[string]Builder
}

// Builder is a function that creates a service instance.
type Builder func(c *Container) (interface{}, error)

// New creates a new dependency injection container.
func New() *Container {
	return &Container{
		services: make(map[string]interface{}),
		builders: make(map[string]Builder),
	}
}

// Register registers a service instance.
func (c *Container) Register(name string, service interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.services[name] = service
}

// RegisterBuilder registers a builder function for lazy instantiation.
func (c *Container) RegisterBuilder(name string, builder Builder) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.builders[name] = builder
}

// Get retrieves a service by name.
func (c *Container) Get(name string) (interface{}, error) {
	c.mu.RLock()
	service, exists := c.services[name]
	c.mu.RUnlock()

	if exists {
		return service, nil
	}

	// Try to build it
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check again in case it was built while waiting for lock
	if service, exists := c.services[name]; exists {
		return service, nil
	}

	builder, hasBuilder := c.builders[name]
	if !hasBuilder {
		return nil, errors.New("service not found: " + name)
	}

	service, err := builder(c)
	if err != nil {
		return nil, err
	}

	c.services[name] = service
	return service, nil
}

// MustGet retrieves a service or panics if not found.
func (c *Container) MustGet(name string) interface{} {
	service, err := c.Get(name)
	if err != nil {
		panic(err)
	}
	return service
}

// Has checks if a service is registered.
func (c *Container) Has(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, exists := c.services[name]
	if exists {
		return true
	}
	_, exists = c.builders[name]
	return exists
}

// ServiceNames returns all registered service names.
func (c *Container) ServiceNames() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	names := make(map[string]bool)
	for name := range c.services {
		names[name] = true
	}
	for name := range c.builders {
		names[name] = true
	}

	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	return result
}

// Clear removes all services and builders.
func (c *Container) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.services = make(map[string]interface{})
	c.builders = make(map[string]Builder)
}

// Service names constants for type-safe access.
const (
	ServiceLedger        = "ledger"
	ServiceNodeStore     = "nodestore"
	ServiceRelationalDB  = "relationaldb"
	ServiceRPCServer     = "rpc.server"
	ServiceConfig        = "config"
	ServiceTxEngine      = "tx.engine"
	ServiceEventPublisher = "event.publisher"
	ServiceFeeManager    = "fee.manager"
	ServiceTxIndex       = "tx.index"
)
