package database

import (
	"context"
	"sync"
	"testing"
)

// MockDB implements DB interface for testing
type MockDB struct {
	data     map[string][]byte
	mu       sync.RWMutex
	isClosed bool
}

func NewMockDB() *MockDB {
	return &MockDB{
		data: make(map[string][]byte),
	}
}

func (m *MockDB) Read(ctx context.Context, key []byte) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.isClosed {
		return nil, ErrDBClosed
	}
	if value, ok := m.data[string(key)]; ok {
		return value, nil
	}
	return nil, ErrKeyNotFound
}

func (m *MockDB) Write(ctx context.Context, key []byte, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isClosed {
		return ErrDBClosed
	}
	m.data[string(key)] = value
	return nil
}

func (m *MockDB) Delete(ctx context.Context, key []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isClosed {
		return ErrDBClosed
	}
	delete(m.data, string(key))
	return nil
}

func (m *MockDB) Batch(ctx context.Context, ops []BatchOperation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isClosed {
		return ErrDBClosed
	}
	for _, op := range ops {
		switch op.Type {
		case BatchPut:
			m.data[string(op.Key)] = op.Value
		case BatchDelete:
			delete(m.data, string(op.Key))
		}
	}
	return nil
}

// MockIterator implements Iterator interface
type MockIterator struct {
	data     [][]byte
	keys     [][]byte
	position int
	err      error
}

func (m *MockDB) Iterator(ctx context.Context, start, end []byte) (Iterator, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.isClosed {
		return nil, ErrDBClosed
	}

	// Collect all keys and values within range
	var keys, values [][]byte
	for k, v := range m.data {
		key := []byte(k)
		if (start == nil || string(key) >= string(start)) &&
			(end == nil || string(key) <= string(end)) {
			keys = append(keys, key)
			values = append(values, v)
		}
	}

	return &MockIterator{
		data:     values,
		keys:     keys,
		position: -1,
	}, nil
}

func (it *MockIterator) Next() bool {
	it.position++
	return it.position < len(it.data)
}

func (it *MockIterator) Key() []byte {
	if it.position >= 0 && it.position < len(it.keys) {
		return it.keys[it.position]
	}
	return nil
}

func (it *MockIterator) Value() []byte {
	if it.position >= 0 && it.position < len(it.data) {
		return it.data[it.position]
	}
	return nil
}

func (it *MockIterator) Error() error {
	return it.err
}

func (it *MockIterator) Close() error {
	return nil
}

// Tests
func TestDB(t *testing.T) {
	ctx := context.Background()
	db := NewMockDB()

	t.Run("Write and Read", func(t *testing.T) {
		key := []byte("test-key")
		value := []byte("test-value")

		err := db.Write(ctx, key, value)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		got, err := db.Read(ctx, key)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		if string(got) != string(value) {
			t.Errorf("Read returned wrong value: got %s, want %s", got, value)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		key := []byte("test-key")

		err := db.Delete(ctx, key)
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		_, err = db.Read(ctx, key)
		if err != ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("Batch Operations", func(t *testing.T) {
		ops := []BatchOperation{
			{Type: BatchPut, Key: []byte("key1"), Value: []byte("value1")},
			{Type: BatchPut, Key: []byte("key2"), Value: []byte("value2")},
			{Type: BatchDelete, Key: []byte("key1")},
		}

		err := db.Batch(ctx, ops)
		if err != nil {
			t.Fatalf("Batch failed: %v", err)
		}

		// key1 should be deleted
		_, err = db.Read(ctx, []byte("key1"))
		if err != ErrKeyNotFound {
			t.Errorf("Expected key1 to be deleted")
		}

		// key2 should exist
		value, err := db.Read(ctx, []byte("key2"))
		if err != nil {
			t.Fatalf("Read key2 failed: %v", err)
		}
		if string(value) != "value2" {
			t.Errorf("Wrong value for key2: got %s, want value2", value)
		}
	})

	t.Run("Iterator", func(t *testing.T) {
		// Write some test data
		testData := map[string]string{
			"a": "value-a",
			"b": "value-b",
			"c": "value-c",
		}

		for k, v := range testData {
			err := db.Write(ctx, []byte(k), []byte(v))
			if err != nil {
				t.Fatalf("Write failed: %v", err)
			}
		}

		iter, err := db.Iterator(ctx, []byte("a"), []byte("c"))
		if err != nil {
			t.Fatalf("Iterator creation failed: %v", err)
		}
		defer func(iter Iterator) {
			err := iter.Close()
			if err != nil {
				t.Fatalf("Iterator close failed: %v", err)
			}
		}(iter)

		count := 0
		for iter.Next() {
			key := string(iter.Key())
			value := string(iter.Value())
			expectedValue, ok := testData[key]
			if !ok {
				t.Errorf("Unexpected key: %s", key)
			}
			if value != expectedValue {
				t.Errorf("Wrong value for key %s: got %s, want %s", key, value, expectedValue)
			}
			count++
		}

		if count != len(testData) {
			t.Errorf("Iterator returned wrong number of items: got %d, want %d", count, len(testData))
		}

		if err := iter.Error(); err != nil {
			t.Errorf("Iterator error: %v", err)
		}
	})
}
