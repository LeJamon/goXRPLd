package keyValueDb

import (
	"context"
)

// Manager handles the lifecycle of databases
type Manager interface {
	// OpenDB opens or creates a keyValueDb with the given name
	OpenDB(name string) (DB, error)

	// CloseDB closes a specific keyValueDb
	CloseDB(name string) error

	// Close closes all databases
	Close() error
}

// DBInteractor provides higher-level keyValueDb operations
type DBInteractor interface {
	// GetDB returns a keyValueDb instance for a namespace
	GetDB(namespace string) DB

	// BatchAcrossNamespaces executes operations across multiple namespaces
	BatchAcrossNamespaces(ctx context.Context, ops map[string][]BatchOperation) error
}
