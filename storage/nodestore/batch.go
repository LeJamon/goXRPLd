package nodestore

import (
	"fmt"
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
// Kept for backwards compatibility; batch writing is now handled internally.
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

// BatchWriter is kept for backwards compatibility.
// It is a stub that wraps a Backend for synchronous writes.
type BatchWriter struct {
	backend Backend
}

// NewBatchWriter creates a new BatchWriter with the given backend.
// The config is validated but ignored (batch writing is now synchronous).
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

	return &BatchWriter{backend: backend}, nil
}

// Write submits a write operation synchronously.
func (bw *BatchWriter) Write(hash Hash256, data []byte) <-chan error {
	result := make(chan error, 1)

	node := &Node{
		Type: NodeUnknown,
		Hash: hash,
		Data: make([]byte, len(data)),
	}
	copy(node.Data, data)

	status := bw.backend.Store(node)
	if status != OK {
		result <- fmt.Errorf("batch store failed: %s", status.String())
	} else {
		result <- nil
	}
	close(result)
	return result
}

// WriteSync submits a write operation and waits for completion.
func (bw *BatchWriter) WriteSync(hash Hash256, data []byte) error {
	return <-bw.Write(hash, data)
}

// WriteNode submits a node for writing.
func (bw *BatchWriter) WriteNode(node *Node) <-chan error {
	if node == nil {
		result := make(chan error, 1)
		result <- fmt.Errorf("node cannot be nil")
		close(result)
		return result
	}
	return bw.Write(node.Hash, node.Data)
}

// WriteNodeSync submits a node for writing and waits for completion.
func (bw *BatchWriter) WriteNodeSync(node *Node) error {
	return <-bw.WriteNode(node)
}

// Flush is a no-op since writes are synchronous.
func (bw *BatchWriter) Flush() error {
	return nil
}

// Close is a no-op.
func (bw *BatchWriter) Close() error {
	return nil
}

// PendingCount always returns 0 for synchronous writes.
func (bw *BatchWriter) PendingCount() int {
	return 0
}

// Stats returns statistics about the batch writer.
func (bw *BatchWriter) Stats() BatchWriterStats {
	return BatchWriterStats{}
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
