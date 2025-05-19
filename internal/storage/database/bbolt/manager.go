package bbolt

import (
	"fmt"
	"github.com/LeJamon/goXRPLd/internal/storage/database"
	"go.etcd.io/bbolt"
	"path/filepath"
)

type Manager struct {
	dbs  map[string]*bbolt.DB
	path string
}

func NewManager(path string) *Manager {
	return &Manager{
		dbs:  make(map[string]*bbolt.DB),
		path: path,
	}
}

func (m *Manager) OpenDB(name string) (database.DB, error) {
	dbPath := filepath.Join(m.path, name+".db")
	db, err := bbolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open database %s: %w", name, err)
	}

	// Create default bucket
	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(name))
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create bucket for %s: %w", name, err)
	}

	m.dbs[name] = db
	return NewDB(db, []byte(name)), nil
}

func (m *Manager) CloseDB(name string) error {
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
	var lastErr error
	for name, db := range m.dbs {
		if err := db.Close(); err != nil {
			lastErr = fmt.Errorf("failed to close database %s: %w", name, err)
		}
		delete(m.dbs, name)
	}
	return lastErr
}
