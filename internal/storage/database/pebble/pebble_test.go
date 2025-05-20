package pebble

import (
	"context"
	"fmt"
	"github.com/LeJamon/goXRPLd/internal/storage/database"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) (*Manager, func()) {
	tempDir, err := os.MkdirTemp("", "pebble-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	manager := NewManager(tempDir)

	cleanup := func() {
		err := manager.Close()
		if err != nil {
			return
		}
		err = os.RemoveAll(tempDir)
		if err != nil {
			return
		}
	}

	return manager, cleanup
}

func TestPebbleDB(t *testing.T) {
	manager, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("Database Lifecycle", func(t *testing.T) {
		db, err := manager.OpenDB("test")
		if err != nil {
			t.Fatalf("Failed to open database: %v", err)
		}

		key := []byte("lifecycle-test")
		value := []byte("test-value")

		err = db.Write(ctx, key, value)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		got, err := db.Read(ctx, key)
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}

		if string(got) != string(value) {
			t.Errorf("Wrong value read: got %s, want %s", got, value)
		}

		err = manager.CloseDB("test")
		if err != nil {
			t.Fatalf("Failed to close database: %v", err)
		}

		dbPath := filepath.Join(manager.path, "test.db")
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			t.Error("Database file was not created")
		}
	})

	t.Run("Batch Operations", func(t *testing.T) {
		db, err := manager.OpenDB("batch-test")
		if err != nil {
			t.Fatalf("Failed to open database: %v", err)
		}

		ops := []database.BatchOperation{
			{Type: database.BatchPut, Key: []byte("batch1"), Value: []byte("value1")},
			{Type: database.BatchPut, Key: []byte("batch2"), Value: []byte("value2")},
			{Type: database.BatchDelete, Key: []byte("batch1")},
		}

		err = db.Batch(ctx, ops)
		if err != nil {
			t.Fatalf("Batch operation failed: %v", err)
		}

		_, err = db.Read(ctx, []byte("batch1"))
		if err == nil {
			t.Error("Expected batch1 to be deleted")
		}

		value, err := db.Read(ctx, []byte("batch2"))
		if err != nil {
			t.Fatalf("Failed to read batch2: %v", err)
		}
		if string(value) != "value2" {
			t.Errorf("Wrong value for batch2: got %s, want value2", value)
		}
	})

	t.Run("Iterator", func(t *testing.T) {
		db, err := manager.OpenDB("iterator-test")
		if err != nil {
			t.Fatalf("Failed to open database: %v", err)
		}

		testData := map[string]string{
			"iter1": "value1",
			"iter2": "value2",
			"iter3": "value3",
		}

		for k, v := range testData {
			err := db.Write(ctx, []byte(k), []byte(v))
			if err != nil {
				t.Fatalf("Write failed: %v", err)
			}
		}

		iter, err := db.Iterator(ctx, []byte("iter1"), []byte("iter3"))
		if err != nil {
			t.Fatalf("Failed to create iterator: %v", err)
		}
		defer func(iter database.Iterator) {
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

		if err := iter.Error(); err != nil {
			t.Errorf("Iterator error: %v", err)
		}

		expectedCount := 2
		if count != expectedCount {
			t.Errorf("Iterator returned wrong number of items: got %d, want %d", count, expectedCount)
		}
	})

	t.Run("Concurrent Access", func(t *testing.T) {
		db, err := manager.OpenDB("concurrent-test")
		if err != nil {
			t.Fatalf("Failed to open database: %v", err)
		}

		const numGoroutines = 10
		const numOperations = 100

		errCh := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				var err error
				for j := 0; j < numOperations; j++ {
					key := []byte(fmt.Sprintf("concurrent-%d-%d", id, j))
					value := []byte(fmt.Sprintf("value-%d-%d", id, j))

					err = db.Write(ctx, key, value)
					if err != nil {
						break
					}

					_, err = db.Read(ctx, key)
					if err != nil {
						break
					}

					time.Sleep(time.Millisecond)
				}
				errCh <- err
			}(i)
		}

		for i := 0; i < numGoroutines; i++ {
			if err := <-errCh; err != nil {
				t.Errorf("Goroutine error: %v", err)
			}
		}
	})
}
