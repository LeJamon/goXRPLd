package nodestore

import (
	"fmt"
	"sync"
)

// BackendFactory is a function that creates a new backend instance.
type BackendFactory func(config *Config) (Backend, error)

var (
	backendMu        sync.RWMutex
	backendFactories = make(map[string]BackendFactory)
)

// RegisterBackend registers a backend factory with the given name.
func RegisterBackend(name string, factory BackendFactory) {
	backendMu.Lock()
	defer backendMu.Unlock()
	backendFactories[name] = factory
}

// CreateBackend creates a new backend instance for the given name and configuration.
func CreateBackend(name string, config *Config) (Backend, error) {
	backendMu.RLock()
	factory, ok := backendFactories[name]
	backendMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown backend: %s", name)
	}

	return factory(config)
}

// AvailableBackends returns a list of available backend names.
func AvailableBackends() []string {
	backendMu.RLock()
	defer backendMu.RUnlock()

	names := make([]string, 0, len(backendFactories))
	for name := range backendFactories {
		names = append(names, name)
	}
	return names
}

// IsBackendAvailable checks if a backend with the given name is available.
func IsBackendAvailable(name string) bool {
	backendMu.RLock()
	_, ok := backendFactories[name]
	backendMu.RUnlock()
	return ok
}

// BackendInfo provides information about a backend.
type BackendInfo struct {
	Name            string // Backend name
	Description     string // Human-readable description
	FileDescriptors int    // Number of file descriptors required
	Persistent      bool   // Whether the backend provides persistent storage
	Compression     bool   // Whether the backend supports compression
}

// String returns a string representation of the backend info.
func (bi BackendInfo) String() string {
	features := []string{}
	if bi.Persistent {
		features = append(features, "persistent")
	} else {
		features = append(features, "in-memory")
	}
	if bi.Compression {
		features = append(features, "compression")
	}

	return fmt.Sprintf("%s: %s (FDs: %d, Features: %v)",
		bi.Name, bi.Description, bi.FileDescriptors, features)
}

// BackendWithInfo is an interface that backends can implement to provide
// additional information about their capabilities.
type BackendWithInfo interface {
	Backend
	Info() BackendInfo
}

// init registers the built-in backends.
func init() {
	RegisterBackend("pebble", NewPebbleBackend)
}
