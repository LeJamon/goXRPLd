package nodestore_test

import (
	"sync"
	"testing"
	"time"

	"github.com/LeJamon/goXRPLd/internal/storage/nodestore"
)

func TestBatchWriter(t *testing.T) {
	t.Run("Creation", func(t *testing.T) {
		backend := nodestore.NewMemoryBackend()
		if err := backend.Open(true); err != nil {
			t.Fatalf("failed to open backend: %v", err)
		}
		defer backend.Close()

		bw, err := nodestore.NewBatchWriter(backend, nil)
		if err != nil {
			t.Fatalf("failed to create batch writer: %v", err)
		}
		defer bw.Close()

		if bw.PendingCount() != 0 {
			t.Errorf("expected 0 pending, got %d", bw.PendingCount())
		}
	})

	t.Run("CreationWithConfig", func(t *testing.T) {
		backend := nodestore.NewMemoryBackend()
		if err := backend.Open(true); err != nil {
			t.Fatalf("failed to open backend: %v", err)
		}
		defer backend.Close()

		config := &nodestore.BatchWriteConfig{
			PreallocationSize: 100,
			LimitSize:         1000,
			FlushInterval:     50 * time.Millisecond,
			SyncOnFlush:       true,
		}

		bw, err := nodestore.NewBatchWriter(backend, config)
		if err != nil {
			t.Fatalf("failed to create batch writer: %v", err)
		}
		defer bw.Close()
	})

	t.Run("InvalidConfig", func(t *testing.T) {
		backend := nodestore.NewMemoryBackend()
		if err := backend.Open(true); err != nil {
			t.Fatalf("failed to open backend: %v", err)
		}
		defer backend.Close()

		// Invalid preallocation size
		config := &nodestore.BatchWriteConfig{
			PreallocationSize: 0,
			LimitSize:         1000,
			FlushInterval:     50 * time.Millisecond,
		}

		_, err := nodestore.NewBatchWriter(backend, config)
		if err == nil {
			t.Error("expected error for invalid config")
		}
	})

	t.Run("NilBackend", func(t *testing.T) {
		_, err := nodestore.NewBatchWriter(nil, nil)
		if err == nil {
			t.Error("expected error for nil backend")
		}
	})

	t.Run("WriteAndFlush", func(t *testing.T) {
		backend := nodestore.NewMemoryBackend()
		if err := backend.Open(true); err != nil {
			t.Fatalf("failed to open backend: %v", err)
		}
		defer backend.Close()

		config := &nodestore.BatchWriteConfig{
			PreallocationSize: 10,
			LimitSize:         100,
			FlushInterval:     10 * time.Millisecond,
		}

		bw, err := nodestore.NewBatchWriter(backend, config)
		if err != nil {
			t.Fatalf("failed to create batch writer: %v", err)
		}
		defer bw.Close()

		// Write some data
		data := nodestore.Blob("batch write test")
		hash := nodestore.ComputeHash256(data)

		resultCh := bw.Write(hash, data)

		// Wait for result
		err = <-resultCh
		if err != nil {
			t.Errorf("write returned error: %v", err)
		}

		// Verify data was written to backend
		time.Sleep(50 * time.Millisecond) // Give time for flush

		node, status := backend.Fetch(hash)
		if status != nodestore.OK {
			t.Errorf("failed to fetch written node: %v", status)
		}
		if node == nil {
			t.Error("fetched node is nil")
		}
	})

	t.Run("WriteSync", func(t *testing.T) {
		backend := nodestore.NewMemoryBackend()
		if err := backend.Open(true); err != nil {
			t.Fatalf("failed to open backend: %v", err)
		}
		defer backend.Close()

		bw, err := nodestore.NewBatchWriter(backend, nil)
		if err != nil {
			t.Fatalf("failed to create batch writer: %v", err)
		}
		defer bw.Close()

		data := nodestore.Blob("sync write test")
		hash := nodestore.ComputeHash256(data)

		err = bw.WriteSync(hash, data)
		if err != nil {
			t.Errorf("WriteSync returned error: %v", err)
		}
	})

	t.Run("WriteNode", func(t *testing.T) {
		backend := nodestore.NewMemoryBackend()
		if err := backend.Open(true); err != nil {
			t.Fatalf("failed to open backend: %v", err)
		}
		defer backend.Close()

		bw, err := nodestore.NewBatchWriter(backend, nil)
		if err != nil {
			t.Fatalf("failed to create batch writer: %v", err)
		}
		defer bw.Close()

		node := nodestore.NewNode(nodestore.NodeTransaction, nodestore.Blob("node write test"))

		resultCh := bw.WriteNode(node)

		err = <-resultCh
		if err != nil {
			t.Errorf("WriteNode returned error: %v", err)
		}
	})

	t.Run("WriteNilNode", func(t *testing.T) {
		backend := nodestore.NewMemoryBackend()
		if err := backend.Open(true); err != nil {
			t.Fatalf("failed to open backend: %v", err)
		}
		defer backend.Close()

		bw, err := nodestore.NewBatchWriter(backend, nil)
		if err != nil {
			t.Fatalf("failed to create batch writer: %v", err)
		}
		defer bw.Close()

		resultCh := bw.WriteNode(nil)

		err = <-resultCh
		if err == nil {
			t.Error("expected error for nil node")
		}
	})

	t.Run("FlushOnLimitSize", func(t *testing.T) {
		backend := nodestore.NewMemoryBackend()
		if err := backend.Open(true); err != nil {
			t.Fatalf("failed to open backend: %v", err)
		}
		defer backend.Close()

		config := &nodestore.BatchWriteConfig{
			PreallocationSize: 5,
			LimitSize:         10,
			FlushInterval:     50 * time.Millisecond, // Use short interval so writes complete
		}

		bw, err := nodestore.NewBatchWriter(backend, config)
		if err != nil {
			t.Fatalf("failed to create batch writer: %v", err)
		}
		defer bw.Close()

		// Write more than limit size
		results := make([]<-chan error, 15)
		for i := 0; i < 15; i++ {
			data := nodestore.Blob("limit test " + string(rune('A'+i)))
			hash := nodestore.ComputeHash256(data)
			results[i] = bw.Write(hash, data)
		}

		// Wait for all results with timeout
		for i, ch := range results {
			select {
			case err := <-ch:
				if err != nil {
					t.Errorf("write %d returned error: %v", i, err)
				}
			case <-time.After(time.Second):
				t.Errorf("write %d timed out", i)
			}
		}
	})

	t.Run("ManualFlush", func(t *testing.T) {
		backend := nodestore.NewMemoryBackend()
		if err := backend.Open(true); err != nil {
			t.Fatalf("failed to open backend: %v", err)
		}
		defer backend.Close()

		config := &nodestore.BatchWriteConfig{
			PreallocationSize: 100,
			LimitSize:         1000,
			FlushInterval:     time.Hour, // Long interval
		}

		bw, err := nodestore.NewBatchWriter(backend, config)
		if err != nil {
			t.Fatalf("failed to create batch writer: %v", err)
		}
		defer bw.Close()

		// Write some data
		for i := 0; i < 5; i++ {
			data := nodestore.Blob("manual flush test " + string(rune('A'+i)))
			hash := nodestore.ComputeHash256(data)
			bw.Write(hash, data)
		}

		// Manual flush
		if err := bw.Flush(); err != nil {
			t.Errorf("Flush returned error: %v", err)
		}

		// Give time for flush to complete
		time.Sleep(50 * time.Millisecond)
	})

	t.Run("Stats", func(t *testing.T) {
		backend := nodestore.NewMemoryBackend()
		if err := backend.Open(true); err != nil {
			t.Fatalf("failed to open backend: %v", err)
		}
		defer backend.Close()

		bw, err := nodestore.NewBatchWriter(backend, nil)
		if err != nil {
			t.Fatalf("failed to create batch writer: %v", err)
		}
		defer bw.Close()

		// Write some data
		for i := 0; i < 5; i++ {
			data := nodestore.Blob("stats test " + string(rune('A'+i)))
			hash := nodestore.ComputeHash256(data)
			err := bw.WriteSync(hash, data)
			if err != nil {
				t.Errorf("WriteSync returned error: %v", err)
			}
		}

		stats := bw.Stats()

		if stats.TotalWrites < 5 {
			t.Errorf("expected at least 5 total writes, got %d", stats.TotalWrites)
		}

		if stats.Flushes < 1 {
			t.Errorf("expected at least 1 flush, got %d", stats.Flushes)
		}
	})

	t.Run("Close", func(t *testing.T) {
		backend := nodestore.NewMemoryBackend()
		if err := backend.Open(true); err != nil {
			t.Fatalf("failed to open backend: %v", err)
		}
		defer backend.Close()

		bw, err := nodestore.NewBatchWriter(backend, nil)
		if err != nil {
			t.Fatalf("failed to create batch writer: %v", err)
		}

		// Write some data
		data := nodestore.Blob("close test")
		hash := nodestore.ComputeHash256(data)
		bw.Write(hash, data)

		// Close should flush pending writes
		if err := bw.Close(); err != nil {
			t.Errorf("Close returned error: %v", err)
		}

		// Double close should be safe
		if err := bw.Close(); err != nil {
			t.Errorf("double Close returned error: %v", err)
		}
	})

	t.Run("WriteAfterClose", func(t *testing.T) {
		backend := nodestore.NewMemoryBackend()
		if err := backend.Open(true); err != nil {
			t.Fatalf("failed to open backend: %v", err)
		}
		defer backend.Close()

		bw, err := nodestore.NewBatchWriter(backend, nil)
		if err != nil {
			t.Fatalf("failed to create batch writer: %v", err)
		}

		bw.Close()

		// Write after close should return shutdown error
		data := nodestore.Blob("after close")
		hash := nodestore.ComputeHash256(data)
		resultCh := bw.Write(hash, data)

		err = <-resultCh
		if err == nil {
			t.Error("expected error when writing after close")
		}
	})

	t.Run("ConcurrentWrites", func(t *testing.T) {
		backend := nodestore.NewMemoryBackend()
		if err := backend.Open(true); err != nil {
			t.Fatalf("failed to open backend: %v", err)
		}
		defer backend.Close()

		bw, err := nodestore.NewBatchWriter(backend, nil)
		if err != nil {
			t.Fatalf("failed to create batch writer: %v", err)
		}
		defer bw.Close()

		const goroutines = 10
		const writesPerGoroutine = 20

		var wg sync.WaitGroup
		wg.Add(goroutines)

		for g := 0; g < goroutines; g++ {
			go func(id int) {
				defer wg.Done()

				for i := 0; i < writesPerGoroutine; i++ {
					data := nodestore.Blob("concurrent write " + string(rune('A'+id)) + string(rune('0'+i%10)))
					hash := nodestore.ComputeHash256(data)
					resultCh := bw.Write(hash, data)
					<-resultCh // Wait for result
				}
			}(g)
		}

		wg.Wait()

		stats := bw.Stats()
		expectedWrites := int64(goroutines * writesPerGoroutine)
		if stats.TotalWrites != expectedWrites {
			t.Errorf("expected %d total writes, got %d", expectedWrites, stats.TotalWrites)
		}
	})
}

func TestBatchWriteCollector(t *testing.T) {
	t.Run("Creation", func(t *testing.T) {
		collector := nodestore.NewBatchWriteCollector()
		if collector == nil {
			t.Fatal("NewBatchWriteCollector returned nil")
		}

		if collector.Count() != 0 {
			t.Errorf("expected count 0, got %d", collector.Count())
		}
	})

	t.Run("AddAndWait", func(t *testing.T) {
		backend := nodestore.NewMemoryBackend()
		if err := backend.Open(true); err != nil {
			t.Fatalf("failed to open backend: %v", err)
		}
		defer backend.Close()

		bw, err := nodestore.NewBatchWriter(backend, nil)
		if err != nil {
			t.Fatalf("failed to create batch writer: %v", err)
		}
		defer bw.Close()

		collector := nodestore.NewBatchWriteCollector()

		// Add some writes
		for i := 0; i < 5; i++ {
			data := nodestore.Blob("collector test " + string(rune('A'+i)))
			hash := nodestore.ComputeHash256(data)
			resultCh := bw.Write(hash, data)
			collector.Add(hash, resultCh)
		}

		if collector.Count() != 5 {
			t.Errorf("expected count 5, got %d", collector.Count())
		}

		// Wait for all results
		results := collector.Wait()

		if len(results) != 5 {
			t.Errorf("expected 5 results, got %d", len(results))
		}

		for i, result := range results {
			if result.Error != nil {
				t.Errorf("result %d has error: %v", i, result.Error)
			}
		}
	})

	t.Run("WaitWithErrors", func(t *testing.T) {
		backend := nodestore.NewMemoryBackend()
		if err := backend.Open(true); err != nil {
			t.Fatalf("failed to open backend: %v", err)
		}
		defer backend.Close()

		bw, err := nodestore.NewBatchWriter(backend, nil)
		if err != nil {
			t.Fatalf("failed to create batch writer: %v", err)
		}
		defer bw.Close()

		collector := nodestore.NewBatchWriteCollector()

		// Add some writes
		for i := 0; i < 3; i++ {
			data := nodestore.Blob("wait errors test " + string(rune('A'+i)))
			hash := nodestore.ComputeHash256(data)
			resultCh := bw.Write(hash, data)
			collector.Add(hash, resultCh)
		}

		// Wait for errors (should be nil for successful writes)
		err = collector.WaitWithErrors()
		if err != nil {
			t.Errorf("WaitWithErrors returned error: %v", err)
		}
	})

	t.Run("Clear", func(t *testing.T) {
		collector := nodestore.NewBatchWriteCollector()

		// Add a dummy channel
		ch := make(chan error, 1)
		ch <- nil
		close(ch)
		collector.Add(nodestore.Hash256{}, ch)

		if collector.Count() != 1 {
			t.Fatal("expected count 1")
		}

		collector.Clear()

		if collector.Count() != 0 {
			t.Errorf("expected count 0 after clear, got %d", collector.Count())
		}
	})
}

func TestBatchWriteConfig(t *testing.T) {
	t.Run("Defaults", func(t *testing.T) {
		config := nodestore.DefaultBatchWriteConfig()

		if config.PreallocationSize != nodestore.DefaultPreallocationSize {
			t.Errorf("expected PreallocationSize %d, got %d",
				nodestore.DefaultPreallocationSize, config.PreallocationSize)
		}

		if config.LimitSize != nodestore.DefaultLimitSize {
			t.Errorf("expected LimitSize %d, got %d",
				nodestore.DefaultLimitSize, config.LimitSize)
		}

		if config.FlushInterval != nodestore.DefaultFlushInterval {
			t.Errorf("expected FlushInterval %v, got %v",
				nodestore.DefaultFlushInterval, config.FlushInterval)
		}
	})

	t.Run("Validation", func(t *testing.T) {
		// Valid config
		valid := &nodestore.BatchWriteConfig{
			PreallocationSize: 100,
			LimitSize:         1000,
			FlushInterval:     time.Second,
		}
		if err := valid.Validate(); err != nil {
			t.Errorf("valid config returned error: %v", err)
		}

		// Invalid preallocation
		invalid1 := &nodestore.BatchWriteConfig{
			PreallocationSize: 0,
			LimitSize:         1000,
			FlushInterval:     time.Second,
		}
		if err := invalid1.Validate(); err == nil {
			t.Error("expected error for invalid preallocation size")
		}

		// Invalid limit
		invalid2 := &nodestore.BatchWriteConfig{
			PreallocationSize: 100,
			LimitSize:         0,
			FlushInterval:     time.Second,
		}
		if err := invalid2.Validate(); err == nil {
			t.Error("expected error for invalid limit size")
		}

		// Limit < preallocation
		invalid3 := &nodestore.BatchWriteConfig{
			PreallocationSize: 1000,
			LimitSize:         100,
			FlushInterval:     time.Second,
		}
		if err := invalid3.Validate(); err == nil {
			t.Error("expected error when limit < preallocation")
		}

		// Invalid flush interval
		invalid4 := &nodestore.BatchWriteConfig{
			PreallocationSize: 100,
			LimitSize:         1000,
			FlushInterval:     0,
		}
		if err := invalid4.Validate(); err == nil {
			t.Error("expected error for invalid flush interval")
		}
	})
}

func TestBatchWriterStats(t *testing.T) {
	t.Run("String", func(t *testing.T) {
		stats := nodestore.BatchWriterStats{
			TotalWrites:   100,
			BatchedWrites: 95,
			Flushes:       10,
			Errors:        2,
			BytesWritten:  10240,
			PendingCount:  5,
		}

		s := stats.String()

		if s == "" {
			t.Error("Stats.String() should not be empty")
		}

		// Should contain key metrics
		if !containsString(s, "100") {
			t.Error("String should contain total writes")
		}
	})
}
