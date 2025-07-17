package bbolt

import (
	"fmt"
	"github.com/LeJamon/goXRPLd/internal/storage/keyValueDb"
	"go.etcd.io/bbolt"
	"path/filepath"
)

type BBoltManager struct {
	dbs  map[string]*bbolt.DB
	path string
}

func NewBBoltManager(path string) *BBoltManager {
	return &BBoltManager{
		dbs:  make(map[string]*bbolt.DB),
		path: path,
	}
}

func (m *BBoltManager) OpenDB(name string) (keyValueDb.DB, error) {
	dbPath := filepath.Join(m.path, name+".db")
	db, err := bbolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open keyValueDb %s: %w", name, err)
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
	return NewBBoltDB(db, []byte(name)), nil
}

func (m *BBoltManager) CloseDB(name string) error {
	db, exists := m.dbs[name]
	if !exists {
		return fmt.Errorf("keyValueDb %s not found", name)
	}

	err := db.Close()
	if err != nil {
		return err
	}

	delete(m.dbs, name)
	return nil
}

func (m *BBoltManager) Close() error {
	var lastErr error
	for name, db := range m.dbs {
		if err := db.Close(); err != nil {
			lastErr = fmt.Errorf("failed to close keyValueDb %s: %w", name, err)
		}
		delete(m.dbs, name)
	}
	return lastErr
}
