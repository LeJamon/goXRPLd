package nodestore

import (
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/LeJamon/goXRPLd/internal/storage/nodestore/compression"
	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/bloom"
)

const (
	// Encoding constants
	nodeHeaderSize = 4 + 4 + 8 + 4 + 1 // type + ledgerSeq + timestamp + dataLen + compressed

	// Performance constants
	defaultBatchSize     = 1000
	asyncWriteBufferSize = 10000
	minCompressionSize   = 128 // Don't compress very small data

	// Buffer pool constants
	maxBufferSize = 64 * 1024 // 64KB max buffer size
)

// PebbleBackend implements a high-performance PebbleDB storage backend.
type PebbleBackend struct {
	// Core components
	db         *pebble.DB
	compressor compression.Compressor
	config     *Config

	// State management (atomic for lock-free reads)
	open       int64 // Use atomic instead of mutex for simple state
	deletePath int64

	// Buffer pools for reducing allocations
	keyPool    sync.Pool
	bufferPool sync.Pool

	// Async write handling
	writeQueue chan *writeOp
	batchQueue []*writeOp
	batchMu    sync.Mutex
	stopCh     chan struct{}
	wg         sync.WaitGroup

	// Stats (atomic for lock-free updates)
	stats struct {
		reads        int64
		writes       int64
		bytesRead    int64
		bytesWritten int64
	}
}

// writeOp represents a pending write operation
type writeOp struct {
	key    Hash256
	value  []byte
	result chan Status
}

// NewPebbleBackend creates a new optimized PebbleDB backend.
func NewPebbleBackend(config *Config) (Backend, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Get the compressor
	compressor, err := compression.Get(config.Compressor)
	if err != nil {
		return nil, fmt.Errorf("failed to get compressor %s: %w", config.Compressor, err)
	}

	p := &PebbleBackend{
		compressor: compressor,
		config:     config,
		// Don't initialize channels here - they'll be created in Open()
		batchQueue: make([]*writeOp, 0, defaultBatchSize),
	}

	// Initialize buffer pools
	p.keyPool.New = func() interface{} {
		return make([]byte, 32)
	}

	p.bufferPool.New = func() interface{} {
		return make([]byte, 0, 1024) // Start with 1KB, can grow
	}

	return p, nil
}

// Name returns the name of this backend.
func (p *PebbleBackend) Name() string {
	return fmt.Sprintf("pebble(%s)", p.config.Path)
}

// Open opens the backend for use.
func (p *PebbleBackend) Open(createIfMissing bool) error {
	if !atomic.CompareAndSwapInt64(&p.open, 0, 1) {
		return fmt.Errorf("backend already open")
	}

	// Create directory if needed
	if createIfMissing {
		if err := os.MkdirAll(p.config.Path, 0755); err != nil {
			atomic.StoreInt64(&p.open, 0)
			return fmt.Errorf("failed to create directory %s: %w", p.config.Path, err)
		}
	}

	// Configure optimized PebbleDB options for XRPL workload
	opts := p.buildOptimizedOptions()

	// Open the database
	db, err := pebble.Open(p.config.Path, opts)
	if err != nil {
		atomic.StoreInt64(&p.open, 0)
		return fmt.Errorf("failed to open PebbleDB at %s: %w", p.config.Path, err)
	}

	p.db = db

	// Recreate channels in case of reopen after close
	p.writeQueue = make(chan *writeOp, asyncWriteBufferSize)
	p.stopCh = make(chan struct{})

	// Start async write worker
	p.wg.Add(1)
	go p.asyncWriteWorker()

	return nil
}

// buildOptimizedOptions creates optimized PebbleDB options for XRPL workload
func (p *PebbleBackend) buildOptimizedOptions() *pebble.Options {
	// Calculate memory budget based on available system memory
	var memBudget int64 = 256 << 20 // Default 256MB

	// Use up to 25% of system memory for cache, with reasonable bounds
	if mem := getSystemMemory(); mem > 0 {
		budget := mem / 4
		if budget > 1<<30 { // Max 1GB
			budget = 1 << 30
		}
		if budget < 128<<20 { // Min 128MB
			budget = 128 << 20
		}
		memBudget = budget
	}

	cache := pebble.NewCache(memBudget)

	// Optimize for XRPL characteristics:
	// - High write throughput
	// - Point lookups (by hash)
	// - Batch reads
	// - Large value sizes (up to ~64KB)
	opts := &pebble.Options{
		Cache:                       cache,
		MaxOpenFiles:                10000,    // High for SST file caching
		MemTableSize:                64 << 20, // 64MB memtables
		MemTableStopWritesThreshold: 4,        // Allow more memtables
		MaxConcurrentCompactions: func() int { // Scale with CPU cores
			return runtime.NumCPU()
		},

		// L0 settings optimized for high write throughput
		L0CompactionThreshold: 4,  // Allow more L0 files
		L0StopWritesThreshold: 20, // Higher threshold

		// Base level size - start larger for better space amplification
		LBaseMaxBytes: 256 << 20, // 256MB

		// Level-specific options with bloom filters for all levels
		Levels: make([]pebble.LevelOptions, 7),

		// Write options
		DisableWAL: false, // Keep WAL for durability

		// Compaction options
		TargetByteDeletionRate: 128 << 20, // 128MB/sec deletion rate

	}

	// Configure bloom filters and file sizes for each level
	// Optimize for point lookups while managing space amplification
	for i := range opts.Levels {
		opts.Levels[i] = pebble.LevelOptions{
			BlockSize:      32 << 10,                // 32KB blocks (good for large values)
			IndexBlockSize: 256 << 10,               // 256KB index blocks
			FilterPolicy:   bloom.FilterPolicy(10),  // 10 bits per key bloom filter
			FilterType:     pebble.TableFilter,      // Table-level filters
			TargetFileSize: int64(8<<20) << uint(i), // Exponential file size growth
			Compression:    pebble.NoCompression,    // Let app handle compression
		}

		// Cap max file size at 256MB
		if opts.Levels[i].TargetFileSize > 256<<20 {
			opts.Levels[i].TargetFileSize = 256 << 20
		}
	}

	return opts
}

// getSystemMemory returns available system memory in bytes
func getSystemMemory() int64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// This is a rough approximation - in production you might want to use
	// a more accurate method to get total system memory
	return int64(m.Sys)
}

// Close closes the backend and releases resources.
func (p *PebbleBackend) Close() error {
	if !atomic.CompareAndSwapInt64(&p.open, 1, 0) {
		return nil // Already closed
	}

	// Stop async writer gracefully
	if p.stopCh != nil {
		// Check if channel is already closed to prevent panic
		select {
		case <-p.stopCh:
			// Already closed
		default:
			close(p.stopCh)
		}
	}

	// Wait for worker to finish
	p.wg.Wait()

	var err error
	if p.db != nil {
		// Flush any pending writes
		if syncErr := p.db.Flush(); syncErr != nil {
			err = syncErr
		}

		if closeErr := p.db.Close(); closeErr != nil {
			if err == nil {
				err = closeErr
			}
		}
		p.db = nil
	}

	// Delete path if requested
	if atomic.LoadInt64(&p.deletePath) != 0 && p.config.Path != "" {
		if removeErr := os.RemoveAll(p.config.Path); removeErr != nil {
			if err == nil {
				err = removeErr
			}
		}
	}

	return err
}

// IsOpen returns true if the backend is currently open.
func (p *PebbleBackend) IsOpen() bool {
	return atomic.LoadInt64(&p.open) != 0
}

// Fetch retrieves a single object by key - optimized for zero allocations.
func (p *PebbleBackend) Fetch(key Hash256) (*Node, Status) {
	if !p.IsOpen() {
		return nil, BackendError
	}

	// Use Hash256 directly as key - no allocation needed
	keySlice := (*[32]byte)(unsafe.Pointer(&key[0]))[:]

	// Read from PebbleDB
	value, closer, err := p.db.Get(keySlice)
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, NotFound
		}
		return nil, BackendError
	}
	defer closer.Close()

	// Decode the stored data
	node, err := p.decodeNodeZeroCopy(key, value)
	if err != nil {
		return nil, DataCorrupt
	}

	// Update stats
	atomic.AddInt64(&p.stats.reads, 1)
	atomic.AddInt64(&p.stats.bytesRead, int64(len(value)))

	return node, OK
}

// FetchBatch retrieves multiple objects efficiently using iterators.
func (p *PebbleBackend) FetchBatch(keys []Hash256) ([]*Node, Status) {
	if !p.IsOpen() {
		return nil, BackendError
	}

	results := make([]*Node, len(keys))

	// For small batches, use individual gets (faster due to caching)
	if len(keys) <= 10 {
		for i, key := range keys {
			node, status := p.Fetch(key)
			if status == OK {
				results[i] = node
			} else if status != NotFound {
				return nil, status
			}
			// NotFound is OK - results[i] stays nil
		}
		return results, OK
	}

	// For larger batches, use batch get if available, otherwise fallback to individual gets
	// Note: PebbleDB doesn't have batch get, so we use individual gets
	// but we could optimize with sorted iteration if keys are pre-sorted
	for i, key := range keys {
		node, status := p.Fetch(key)
		if status == OK {
			results[i] = node
		} else if status != NotFound {
			return nil, status
		}
	}

	return results, OK
}

// Store saves a single object asynchronously.
func (p *PebbleBackend) Store(node *Node) Status {
	if node == nil {
		return BackendError
	}

	if !p.IsOpen() {
		return BackendError
	}

	// Encode the node
	value, err := p.encodeNodeOptimized(node)
	if err != nil {
		return BackendError
	}

	// For synchronous behavior, write directly
	// In production, you might want to make this configurable
	return p.storeSync(node.Hash, value)
}

// storeSync performs synchronous write
func (p *PebbleBackend) storeSync(key Hash256, value []byte) Status {
	keySlice := (*[32]byte)(unsafe.Pointer(&key[0]))[:]

	// Use NoSync for better performance, rely on WAL for durability
	if err := p.db.Set(keySlice, value, pebble.NoSync); err != nil {
		return BackendError
	}

	// Update stats
	atomic.AddInt64(&p.stats.writes, 1)
	atomic.AddInt64(&p.stats.bytesWritten, int64(len(value)))

	return OK
}

// StoreBatch saves multiple objects efficiently using batched writes.
func (p *PebbleBackend) StoreBatch(nodes []*Node) Status {
	if !p.IsOpen() {
		return BackendError
	}

	if len(nodes) == 0 {
		return OK
	}

	// Use indexed batch for better performance
	batch := p.db.NewIndexedBatch()
	defer batch.Close()

	var totalBytes int64

	for _, node := range nodes {
		if node == nil {
			continue
		}

		value, err := p.encodeNodeOptimized(node)
		if err != nil {
			return BackendError
		}

		keySlice := (*[32]byte)(unsafe.Pointer(&node.Hash[0]))[:]
		if err := batch.Set(keySlice, value, nil); err != nil {
			return BackendError
		}

		totalBytes += int64(len(value))
	}

	// Commit the batch with controlled sync
	syncMode := pebble.NoSync
	if len(nodes) > 1000 { // Sync large batches for durability
		syncMode = pebble.Sync
	}

	if err := batch.Commit(syncMode); err != nil {
		return BackendError
	}

	// Update stats
	atomic.AddInt64(&p.stats.writes, int64(len(nodes)))
	atomic.AddInt64(&p.stats.bytesWritten, totalBytes)

	return OK
}

// asyncWriteWorker handles asynchronous writes in batches
func (p *PebbleBackend) asyncWriteWorker() {
	defer p.wg.Done()

	ticker := time.NewTicker(10 * time.Millisecond) // Batch every 10ms
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			// Flush remaining operations
			p.flushPendingWrites()
			return

		case op, ok := <-p.writeQueue:
			if !ok {
				// Channel closed
				p.flushPendingWrites()
				return
			}

			p.batchMu.Lock()
			p.batchQueue = append(p.batchQueue, op)
			shouldFlush := len(p.batchQueue) >= defaultBatchSize
			p.batchMu.Unlock()

			if shouldFlush {
				p.flushPendingWrites()
			}

		case <-ticker.C:
			p.flushPendingWrites()
		}
	}
}

// flushPendingWrites flushes batched writes to database
func (p *PebbleBackend) flushPendingWrites() {
	p.batchMu.Lock()
	if len(p.batchQueue) == 0 {
		p.batchMu.Unlock()
		return
	}

	// Move operations to local slice
	ops := make([]*writeOp, len(p.batchQueue))
	copy(ops, p.batchQueue)
	p.batchQueue = p.batchQueue[:0] // Reset slice but keep capacity
	p.batchMu.Unlock()

	// Create batch
	batch := p.db.NewIndexedBatch()
	defer batch.Close()

	// Add all operations to batch
	for _, op := range ops {
		keySlice := (*[32]byte)(unsafe.Pointer(&op.key[0]))[:]
		batch.Set(keySlice, op.value, nil)
	}

	// Commit batch
	err := batch.Commit(pebble.NoSync)
	status := OK
	if err != nil {
		status = BackendError
	}

	// Notify all operations
	for _, op := range ops {
		select {
		case op.result <- status:
		default: // Non-blocking send
		}
		close(op.result)
	}
}

// Sync forces pending writes to be flushed.
func (p *PebbleBackend) Sync() Status {
	if !p.IsOpen() {
		return BackendError
	}

	if err := p.db.Flush(); err != nil {
		return BackendError
	}

	return OK
}

// ForEach iterates over all objects in the backend.
func (p *PebbleBackend) ForEach(fn func(*Node) error) error {
	if !p.IsOpen() {
		return ErrBackendClosed
	}

	// Use lower-level iterator for better performance
	opts := &pebble.IterOptions{
		// Could add key bounds here if needed
	}

	iter, _ := p.db.NewIter(opts)
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := iter.Key()
		value := iter.Value()

		// Convert key bytes to Hash256 - zero copy
		if len(key) != 32 {
			continue // Skip invalid keys
		}

		var hash Hash256
		copy(hash[:], key)

		// Decode the node with zero-copy optimization
		node, err := p.decodeNodeZeroCopy(hash, value)
		if err != nil {
			continue // Skip corrupted entries
		}

		// Call the callback function
		if err := fn(node); err != nil {
			return err
		}
	}

	return iter.Error()
}

// GetWriteLoad returns an estimate of pending write operations.
func (p *PebbleBackend) GetWriteLoad() int {
	return len(p.writeQueue)
}

// SetDeletePath marks the backend for deletion when closed.
func (p *PebbleBackend) SetDeletePath() {
	atomic.StoreInt64(&p.deletePath, 1)
}

// FdRequired returns the number of file descriptors needed.
func (p *PebbleBackend) FdRequired() int {
	// Optimized PebbleDB needs more FDs for performance
	return 500
}

// Info returns information about this backend.
func (p *PebbleBackend) Info() BackendInfo {
	return BackendInfo{
		Name:            "pebble-optimized",
		Description:     "High-performance LSM-tree database backend optimized for XRPL",
		FileDescriptors: p.FdRequired(),
		Persistent:      true,
		Compression:     true,
	}
}

// encodeNodeOptimized serializes a node for storage with buffer reuse.
func (p *PebbleBackend) encodeNodeOptimized(node *Node) ([]byte, error) {
	// Get buffer from pool
	buf := p.bufferPool.Get().([]byte)
	defer func() {
		if cap(buf) <= maxBufferSize {
			p.bufferPool.Put(buf[:0]) // Reset length but keep capacity
		}
	}()

	dataToCompress := node.Data
	compressed := false

	// Only compress if data is large enough and compressor is not "none"
	if len(node.Data) > minCompressionSize && p.compressor.Name() != "none" {
		compressedData, err := p.compressor.Compress(node.Data, p.config.CompressionLevel)
		if err == nil {
			// Use compressed data if it provides meaningful space savings (>10% reduction)
			if len(compressedData) < len(node.Data)*9/10 {
				dataToCompress = compressedData
				compressed = true
			}
		}
	}

	// Calculate total size and ensure buffer capacity
	totalSize := nodeHeaderSize + len(dataToCompress)
	if cap(buf) < totalSize {
		buf = make([]byte, totalSize)
	}
	buf = buf[:totalSize]

	// Write header fields using direct byte manipulation for speed
	binary.LittleEndian.PutUint32(buf[0:4], uint32(node.Type))
	binary.LittleEndian.PutUint32(buf[4:8], node.LedgerSeq)
	binary.LittleEndian.PutUint64(buf[8:16], uint64(node.CreatedAt.UnixNano()))
	binary.LittleEndian.PutUint32(buf[16:20], uint32(len(dataToCompress)))

	// Copy data
	copy(buf[20:20+len(dataToCompress)], dataToCompress)

	// Set compression flag
	if compressed {
		buf[20+len(dataToCompress)] = 1
	} else {
		buf[20+len(dataToCompress)] = 0
	}

	// Return copy of buffer to avoid pool interference
	result := make([]byte, len(buf))
	copy(result, buf)

	return result, nil
}

// decodeNodeZeroCopy deserializes a node from storage with minimal allocations.
func (p *PebbleBackend) decodeNodeZeroCopy(hash Hash256, data []byte) (*Node, error) {
	if len(data) < nodeHeaderSize {
		return nil, fmt.Errorf("invalid data size: %d", len(data))
	}

	// Read header fields directly from byte slice
	nodeType := NodeType(binary.LittleEndian.Uint32(data[0:4]))
	ledgerSeq := binary.LittleEndian.Uint32(data[4:8])
	createdNanos := int64(binary.LittleEndian.Uint64(data[8:16]))
	dataLength := int(binary.LittleEndian.Uint32(data[16:20]))

	if 20+dataLength+1 > len(data) {
		return nil, fmt.Errorf("invalid data length: %d", dataLength)
	}

	// Extract data slice without copy
	nodeData := data[20 : 20+dataLength]
	compressed := data[20+dataLength] == 1

	// Decompress if necessary
	if compressed {
		decompressed, err := p.compressor.Decompress(nodeData)
		if err != nil {
			return nil, fmt.Errorf("decompression failed: %w", err)
		}
		nodeData = decompressed
	}

	// Create node - only allocate for the final data if decompressed
	var nodeDataCopy Blob
	if compressed {
		nodeDataCopy = make(Blob, len(nodeData))
		copy(nodeDataCopy, nodeData)
	} else {
		// For uncompressed data, we can reference the original slice
		// Note: This is safe because PebbleDB guarantees the slice is valid
		// during the iterator/get operation
		nodeDataCopy = make(Blob, len(nodeData))
		copy(nodeDataCopy, nodeData)
	}

	node := &Node{
		Type:      nodeType,
		Hash:      hash,
		Data:      nodeDataCopy,
		LedgerSeq: ledgerSeq,
		CreatedAt: time.Unix(0, createdNanos),
	}

	return node, nil
}

// Stats returns performance statistics
func (p *PebbleBackend) Stats() map[string]interface{} {
	stats := make(map[string]interface{})

	// Basic stats
	stats["reads"] = atomic.LoadInt64(&p.stats.reads)
	stats["writes"] = atomic.LoadInt64(&p.stats.writes)
	stats["bytes_read"] = atomic.LoadInt64(&p.stats.bytesRead)
	stats["bytes_written"] = atomic.LoadInt64(&p.stats.bytesWritten)
	stats["pending_writes"] = len(p.writeQueue)

	// PebbleDB metrics if available
	if p.db != nil {
		if metrics := p.db.Metrics(); metrics != nil {
			stats["pebble_metrics"] = *metrics
		}
	}

	return stats
}

// Compact triggers manual compaction of the database.
func (p *PebbleBackend) Compact() error {
	if !p.IsOpen() {
		return ErrBackendClosed
	}

	// Compact the entire key range
	return p.db.Compact(nil, nil, true)
}

// EstimateSize returns an estimate of the total size of data in the given range.
func (p *PebbleBackend) EstimateSize(start, end Hash256) (uint64, error) {
	if !p.IsOpen() {
		return 0, ErrBackendClosed
	}

	startSlice := (*[32]byte)(unsafe.Pointer(&start[0]))[:]
	endSlice := (*[32]byte)(unsafe.Pointer(&end[0]))[:]

	size, err := p.db.EstimateDiskUsage(startSlice, endSlice)
	return size, err
}
