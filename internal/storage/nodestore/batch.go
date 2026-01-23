package nodestore

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// DefaultPreallocationSize is the default number of writes to preallocate space for.
	DefaultPreallocationSize = 256

	// DefaultLimitSize is the default maximum number of writes in a batch before flushing.
	DefaultLimitSize = 65536

	// DefaultFlushInterval is the default interval between periodic flushes.
	DefaultFlushInterval = 100 * time.Millisecond
)

// BatchWriteConfig holds configuration for the batch writer.
type BatchWriteConfig struct {
	// PreallocationSize is the initial capacity of the write buffer.
	PreallocationSize int

	// LimitSize is the maximum number of writes to batch before flushing.
	LimitSize int

	// FlushInterval is the maximum time between flushes.
	FlushInterval time.Duration

	// SyncOnFlush determines whether to sync the backend after each flush.
	SyncOnFlush bool
}

// DefaultBatchWriteConfig returns a BatchWriteConfig with sensible defaults.
func DefaultBatchWriteConfig() *BatchWriteConfig {
	return &BatchWriteConfig{
		PreallocationSize: DefaultPreallocationSize,
		LimitSize:         DefaultLimitSize,
		FlushInterval:     DefaultFlushInterval,
		SyncOnFlush:       false,
	}
}

// Validate checks if the configuration is valid.
func (c *BatchWriteConfig) Validate() error {
	if c.PreallocationSize <= 0 {
		return fmt.Errorf("preallocation_size must be positive")
	}
	if c.LimitSize <= 0 {
		return fmt.Errorf("limit_size must be positive")
	}
	if c.LimitSize < c.PreallocationSize {
		return fmt.Errorf("limit_size must be >= preallocation_size")
	}
	if c.FlushInterval <= 0 {
		return fmt.Errorf("flush_interval must be positive")
	}
	return nil
}

// pendingWrite represents a pending write operation.
type pendingWrite struct {
	hash   Hash256
	data   []byte
	result chan error
}

// BatchWriter provides batched write operations to a backend.
// It accumulates writes and flushes them periodically or when the batch limit is reached.
type BatchWriter struct {
	backend Backend
	config  *BatchWriteConfig

	// Write queue and buffer
	mu       sync.Mutex
	pending  []*pendingWrite
	shutdown int64

	// Background goroutine management
	stopCh chan struct{}
	wg     sync.WaitGroup

	// Statistics
	stats struct {
		totalWrites   int64
		batchedWrites int64
		flushes       int64
		errors        int64
		bytesWritten  int64
	}
}

// NewBatchWriter creates a new BatchWriter with the given backend and configuration.
func NewBatchWriter(backend Backend, config *BatchWriteConfig) (*BatchWriter, error) {
	if backend == nil {
		return nil, fmt.Errorf("backend must not be nil")
	}

	if config == nil {
		config = DefaultBatchWriteConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	bw := &BatchWriter{
		backend: backend,
		config:  config,
		pending: make([]*pendingWrite, 0, config.PreallocationSize),
		stopCh:  make(chan struct{}),
	}

	// Start background flush goroutine
	bw.wg.Add(1)
	go bw.flushWorker()

	return bw, nil
}

// Write submits a write operation asynchronously.
// It returns a channel that will receive the error result when the write completes.
func (bw *BatchWriter) Write(hash Hash256, data []byte) <-chan error {
	result := make(chan error, 1)

	if atomic.LoadInt64(&bw.shutdown) != 0 {
		result <- ErrShutdown
		close(result)
		return result
	}

	// Create pending write with copy of data
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	pw := &pendingWrite{
		hash:   hash,
		data:   dataCopy,
		result: result,
	}

	bw.mu.Lock()
	bw.pending = append(bw.pending, pw)
	shouldFlush := len(bw.pending) >= bw.config.LimitSize
	bw.mu.Unlock()

	atomic.AddInt64(&bw.stats.totalWrites, 1)

	// Flush immediately if limit reached
	if shouldFlush {
		bw.flush()
	}

	return result
}

// WriteSync submits a write operation and waits for completion.
func (bw *BatchWriter) WriteSync(hash Hash256, data []byte) error {
	return <-bw.Write(hash, data)
}

// WriteNode submits a node for batched writing.
func (bw *BatchWriter) WriteNode(node *Node) <-chan error {
	if node == nil {
		result := make(chan error, 1)
		result <- fmt.Errorf("node cannot be nil")
		close(result)
		return result
	}
	return bw.Write(node.Hash, node.Data)
}

// WriteNodeSync submits a node for batched writing and waits for completion.
func (bw *BatchWriter) WriteNodeSync(node *Node) error {
	return <-bw.WriteNode(node)
}

// flushWorker is the background goroutine that periodically flushes pending writes.
func (bw *BatchWriter) flushWorker() {
	defer bw.wg.Done()

	ticker := time.NewTicker(bw.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-bw.stopCh:
			// Final flush before shutdown
			bw.flush()
			return
		case <-ticker.C:
			bw.flush()
		}
	}
}

// flush writes all pending data to the backend.
func (bw *BatchWriter) flush() {
	bw.mu.Lock()
	if len(bw.pending) == 0 {
		bw.mu.Unlock()
		return
	}

	// Take ownership of pending writes
	toFlush := bw.pending
	bw.pending = make([]*pendingWrite, 0, bw.config.PreallocationSize)
	bw.mu.Unlock()

	// Create nodes for batch store
	nodes := make([]*Node, len(toFlush))
	var totalBytes int64

	for i, pw := range toFlush {
		nodes[i] = &Node{
			Type:      NodeUnknown, // Type will be determined by the node data
			Hash:      pw.hash,
			Data:      pw.data,
			CreatedAt: time.Now(),
		}
		totalBytes += int64(len(pw.data))
	}

	// Perform batch store
	status := bw.backend.StoreBatch(nodes)
	var err error
	if status != OK {
		err = fmt.Errorf("batch store failed: %s", status.String())
		atomic.AddInt64(&bw.stats.errors, 1)
	} else {
		atomic.AddInt64(&bw.stats.batchedWrites, int64(len(toFlush)))
		atomic.AddInt64(&bw.stats.bytesWritten, totalBytes)
	}

	atomic.AddInt64(&bw.stats.flushes, 1)

	// Sync if configured
	if bw.config.SyncOnFlush && status == OK {
		if syncStatus := bw.backend.Sync(); syncStatus != OK {
			err = fmt.Errorf("sync failed: %s", syncStatus.String())
		}
	}

	// Notify all pending writers
	for _, pw := range toFlush {
		select {
		case pw.result <- err:
		default:
		}
		close(pw.result)
	}
}

// Flush forces an immediate flush of all pending writes.
func (bw *BatchWriter) Flush() error {
	bw.flush()
	return nil
}

// Close shuts down the batch writer and flushes any pending writes.
func (bw *BatchWriter) Close() error {
	if !atomic.CompareAndSwapInt64(&bw.shutdown, 0, 1) {
		return nil // Already shutting down
	}

	// Signal the worker to stop
	close(bw.stopCh)

	// Wait for worker to complete (which will do final flush)
	bw.wg.Wait()

	return nil
}

// PendingCount returns the number of pending writes.
func (bw *BatchWriter) PendingCount() int {
	bw.mu.Lock()
	count := len(bw.pending)
	bw.mu.Unlock()
	return count
}

// Stats returns statistics about the batch writer.
func (bw *BatchWriter) Stats() BatchWriterStats {
	return BatchWriterStats{
		TotalWrites:   atomic.LoadInt64(&bw.stats.totalWrites),
		BatchedWrites: atomic.LoadInt64(&bw.stats.batchedWrites),
		Flushes:       atomic.LoadInt64(&bw.stats.flushes),
		Errors:        atomic.LoadInt64(&bw.stats.errors),
		BytesWritten:  atomic.LoadInt64(&bw.stats.bytesWritten),
		PendingCount:  bw.PendingCount(),
	}
}

// BatchWriterStats holds statistics for the batch writer.
type BatchWriterStats struct {
	TotalWrites   int64 // Total number of writes submitted
	BatchedWrites int64 // Number of writes successfully batched
	Flushes       int64 // Number of flush operations
	Errors        int64 // Number of errors encountered
	BytesWritten  int64 // Total bytes written
	PendingCount  int   // Current number of pending writes
}

// String returns a formatted string representation of the statistics.
func (s BatchWriterStats) String() string {
	return fmt.Sprintf(`BatchWriter Statistics:
  Total Writes: %d
  Batched Writes: %d
  Flushes: %d
  Errors: %d
  Bytes Written: %d
  Pending: %d`,
		s.TotalWrites,
		s.BatchedWrites,
		s.Flushes,
		s.Errors,
		s.BytesWritten,
		s.PendingCount)
}

// BatchWriteResult holds the result of a batch write operation.
type BatchWriteResult struct {
	Hash  Hash256 // Hash of the written node
	Error error   // Error that occurred (nil if successful)
}

// BatchWriteCollector collects results from multiple batch write operations.
type BatchWriteCollector struct {
	results []<-chan error
	hashes  []Hash256
}

// NewBatchWriteCollector creates a new collector for batch write results.
func NewBatchWriteCollector() *BatchWriteCollector {
	return &BatchWriteCollector{
		results: make([]<-chan error, 0),
		hashes:  make([]Hash256, 0),
	}
}

// Add adds a write result channel to the collector.
func (c *BatchWriteCollector) Add(hash Hash256, result <-chan error) {
	c.results = append(c.results, result)
	c.hashes = append(c.hashes, hash)
}

// Wait waits for all writes to complete and returns the results.
func (c *BatchWriteCollector) Wait() []BatchWriteResult {
	results := make([]BatchWriteResult, len(c.results))

	for i, ch := range c.results {
		err := <-ch
		results[i] = BatchWriteResult{
			Hash:  c.hashes[i],
			Error: err,
		}
	}

	return results
}

// WaitWithErrors waits for all writes and returns only the errors.
func (c *BatchWriteCollector) WaitWithErrors() error {
	var firstErr error
	var errCount int

	for _, ch := range c.results {
		if err := <-ch; err != nil {
			errCount++
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if errCount > 0 {
		if errCount == 1 {
			return firstErr
		}
		return fmt.Errorf("%d writes failed, first error: %w", errCount, firstErr)
	}

	return nil
}

// Count returns the number of tracked writes.
func (c *BatchWriteCollector) Count() int {
	return len(c.results)
}

// Clear resets the collector for reuse.
func (c *BatchWriteCollector) Clear() {
	c.results = c.results[:0]
	c.hashes = c.hashes[:0]
}
