package pebble

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/LeJamon/goXRPLd/internal/storage/database"
	"github.com/cockroachdb/pebble"
)

type Manager struct {
	dbs  map[string]*pebble.DB
	path string
	mu   sync.Mutex
}

func NewManager(path string) *Manager {
	return &Manager{
		dbs:  make(map[string]*pebble.DB),
		path: path,
	}
}

func (m *Manager) OpenDB(name string) (database.DB, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if db, exists := m.dbs[name]; exists {
		return NewDB(db), nil // Already opened
	}

	dbPath := filepath.Join(m.path, name+".db")
	opts := &pebble.Options{
		// Customize options here if needed (cache size, compaction, etc.)
	}

	db, err := pebble.Open(dbPath, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open database %s: %w", name, err)
	}

	m.dbs[name] = db

	return NewDB(db), nil
}

func (m *Manager) CloseDB(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	db, exists := m.dbs[name]
	if !exists {
		return fmt.Errorf("database %s not found", name)
	}

	err := db.Close()
	if err != nil {
		return err
	}

	delete(m.dbs, name)
	return nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var lastErr error
	for name, db := range m.dbs {
		if err := db.Close(); err != nil {
			lastErr = fmt.Errorf("failed to close database %s: %w", name, err)
		}
		delete(m.dbs, name)
	}
	return lastErr
}
