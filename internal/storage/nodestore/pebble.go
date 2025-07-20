package nodestore

import (
	"encoding/binary"
	"fmt"
	"github.com/LeJamon/goXRPLd/internal/storage/nodestore/compression"
	"github.com/cockroachdb/pebble/bloom"
	"os"
	"sync"
	"time"

	"github.com/LeJamon/goXRPLd/internal/types"
	"github.com/cockroachdb/pebble"
)

// PebbleBackend implements a PebbleDB storage backend for production use.
type PebbleBackend struct {
	mu         sync.RWMutex
	db         *pebble.DB
	compressor compression.Compressor
	config     *Config
	open       bool
	deletePath bool
}

// NewPebbleBackend creates a new PebbleDB backend.
func NewPebbleBackend(config *Config) (Backend, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Get the compressor
	compressor, err := compression.Get(config.Compressor)
	if err != nil {
		return nil, fmt.Errorf("failed to get compressor %s: %w", config.Compressor, err)
	}

	return &PebbleBackend{
		compressor: compressor,
		config:     config,
		open:       false,
		deletePath: false,
	}, nil
}

// Name returns the name of this backend.
func (p *PebbleBackend) Name() string {
	return fmt.Sprintf("pebble(%s)", p.config.Path)
}

// Open opens the backend for use.
func (p *PebbleBackend) Open(createIfMissing bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.open {
		return fmt.Errorf("backend already open")
	}

	// Create directory if needed
	if createIfMissing {
		if err := os.MkdirAll(p.config.Path, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", p.config.Path, err)
		}
	}

	// Configure PebbleDB options
	opts := &pebble.Options{
		Cache:                    pebble.NewCache(64 << 20), // 64MB cache
		MaxOpenFiles:             1000,
		MemTableSize:             32 << 20, // 32MB
		MaxConcurrentCompactions: 4,
		L0CompactionThreshold:    2,
		L0StopWritesThreshold:    1000,
		LBaseMaxBytes:            64 << 20, // 64MB
		Levels: []pebble.LevelOptions{
			{TargetFileSize: 2 << 20, FilterPolicy: bloom.FilterPolicy(10)},
		},
	}

	// Open the database
	db, err := pebble.Open(p.config.Path, opts)
	if err != nil {
		return fmt.Errorf("failed to open PebbleDB at %s: %w", p.config.Path, err)
	}

	p.db = db
	p.open = true

	return nil
}

// Close closes the backend and releases resources.
func (p *PebbleBackend) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.open {
		return nil
	}

	var err error
	if p.db != nil {
		err = p.db.Close()
		p.db = nil
	}

	p.open = false

	// Delete path if requested
	if p.deletePath && p.config.Path != "" {
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
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.open
}

// Fetch retrieves a single object by key.
func (p *PebbleBackend) Fetch(key types.Hash256) (*Node, Status) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.open {
		return nil, BackendError
	}

	// Read from PebbleDB
	value, closer, err := p.db.Get(key[:])
	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, NotFound
		}
		return nil, BackendError
	}
	defer closer.Close()

	// Decode the stored data
	node, err := p.decodeNode(key, value)
	if err != nil {
		return nil, DataCorrupt
	}

	return node, OK
}

// FetchBatch retrieves multiple objects efficiently.
func (p *PebbleBackend) FetchBatch(keys []types.Hash256) ([]*Node, Status) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.open {
		return nil, BackendError
	}

	results := make([]*Node, len(keys))

	// Use iterator for batch reads (more efficient than individual Gets)
	for i, key := range keys {
		value, closer, err := p.db.Get(key[:])
		if err != nil {
			if err == pebble.ErrNotFound {
				results[i] = nil // Not found
				continue
			}
			closer.Close()
			return nil, BackendError
		}

		node, decodeErr := p.decodeNode(key, value)
		closer.Close()

		if decodeErr != nil {
			return nil, DataCorrupt
		}

		results[i] = node
	}

	return results, OK
}

// Store saves a single object.
func (p *PebbleBackend) Store(node *Node) Status {
	if node == nil {
		return BackendError
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.open {
		return BackendError
	}

	// Encode the node
	value, err := p.encodeNode(node)
	if err != nil {
		return BackendError
	}

	// Write to PebbleDB
	if err := p.db.Set(node.Hash[:], value, pebble.Sync); err != nil {
		return BackendError
	}

	return OK
}

// StoreBatch saves multiple objects efficiently.
func (p *PebbleBackend) StoreBatch(nodes []*Node) Status {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.open {
		return BackendError
	}

	// Use a batch for efficient bulk writes
	batch := p.db.NewBatch()
	defer batch.Close()

	for _, node := range nodes {
		if node == nil {
			continue
		}

		value, err := p.encodeNode(node)
		if err != nil {
			return BackendError
		}

		if err := batch.Set(node.Hash[:], value, nil); err != nil {
			return BackendError
		}
	}

	// Commit the batch
	if err := batch.Commit(pebble.Sync); err != nil {
		return BackendError
	}

	return OK
}

// Sync forces pending writes to be flushed.
func (p *PebbleBackend) Sync() Status {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.open {
		return BackendError
	}

	if err := p.db.Flush(); err != nil {
		return BackendError
	}

	return OK
}

// ForEach iterates over all objects in the backend.
func (p *PebbleBackend) ForEach(fn func(*Node) error) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.open {
		return ErrBackendClosed
	}

	iter := p.db.NewIter(nil)
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := iter.Key()
		value := iter.Value()

		// Convert key bytes to Hash256
		var hash types.Hash256
		if len(key) != 32 {
			continue // Skip invalid keys
		}
		copy(hash[:], key)

		// Decode the node
		node, err := p.decodeNode(hash, value)
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
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.open {
		return 0
	}

	// PebbleDB doesn't expose pending write count directly,
	// so we return 0 as an approximation
	return 0
}

// SetDeletePath marks the backend for deletion when closed.
func (p *PebbleBackend) SetDeletePath() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deletePath = true
}

// FdRequired returns the number of file descriptors needed.
func (p *PebbleBackend) FdRequired() int {
	// PebbleDB typically needs several file descriptors for:
	// - WAL files
	// - SST files
	// - Manifest files
	// We return a conservative estimate
	return 100
}

// Info returns information about this backend.
func (p *PebbleBackend) Info() BackendInfo {
	return BackendInfo{
		Name:            "pebble",
		Description:     "High-performance LSM-tree database backend",
		FileDescriptors: p.FdRequired(),
		Persistent:      true,
		Compression:     true,
	}
}

// encodeNode serializes a node for storage.
func (p *PebbleBackend) encodeNode(node *Node) ([]byte, error) {
	// Node storage format:
	// [4 bytes: node type][4 bytes: ledger seq][8 bytes: created timestamp][4 bytes: data length][data][compressed flag]

	dataToCompress := node.Data

	// Compress the data if compressor is not "none"
	compressed := false
	if p.compressor.Name() != "none" {
		compressedData, err := p.compressor.Compress(node.Data, p.config.CompressionLevel)
		if err == nil && len(compressedData) < len(node.Data) {
			// Only use compressed data if it's actually smaller
			dataToCompress = compressedData
			compressed = true
		}
	}

	// Calculate total size
	totalSize := 4 + 4 + 8 + 4 + len(dataToCompress) + 1
	result := make([]byte, totalSize)

	offset := 0

	// Write node type
	binary.LittleEndian.PutUint32(result[offset:], uint32(node.Type))
	offset += 4

	// Write ledger sequence
	binary.LittleEndian.PutUint32(result[offset:], node.LedgerSeq)
	offset += 4

	// Write created timestamp (nanoseconds since Unix epoch)
	binary.LittleEndian.PutUint64(result[offset:], uint64(node.CreatedAt.UnixNano()))
	offset += 8

	// Write data length
	binary.LittleEndian.PutUint32(result[offset:], uint32(len(dataToCompress)))
	offset += 4

	// Write data
	copy(result[offset:], dataToCompress)
	offset += len(dataToCompress)

	// Write compression flag
	if compressed {
		result[offset] = 1
	} else {
		result[offset] = 0
	}

	return result, nil
}

// decodeNode deserializes a node from storage.
func (p *PebbleBackend) decodeNode(hash types.Hash256, data []byte) (*Node, error) {
	if len(data) < 21 { // Minimum size: 4+4+8+4+1
		return nil, fmt.Errorf("invalid data size: %d", len(data))
	}

	offset := 0

	// Read node type
	nodeType := NodeType(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4

	// Read ledger sequence
	ledgerSeq := binary.LittleEndian.Uint32(data[offset:])
	offset += 4

	// Read created timestamp
	createdNanos := int64(binary.LittleEndian.Uint64(data[offset:]))
	createdAt := time.Unix(0, createdNanos)
	offset += 8

	// Read data length
	dataLength := int(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4

	if offset+dataLength+1 > len(data) {
		return nil, fmt.Errorf("invalid data length: %d", dataLength)
	}

	// Read data
	nodeData := data[offset : offset+dataLength]
	offset += dataLength

	// Read compression flag
	compressed := data[offset] == 1

	// Decompress if necessary
	if compressed {
		decompressed, err := p.compressor.Decompress(nodeData)
		if err != nil {
			return nil, fmt.Errorf("decompression failed: %w", err)
		}
		nodeData = decompressed
	}

	// Create and return the node
	node := &Node{
		Type:      nodeType,
		Hash:      hash,
		Data:      make(types.Blob, len(nodeData)),
		LedgerSeq: ledgerSeq,
		CreatedAt: createdAt,
	}
	copy(node.Data, nodeData)

	return node, nil
}

// Compact triggers manual compaction of the database.
func (p *PebbleBackend) Compact() error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.open {
		return ErrBackendClosed
	}

	// Compact the entire key range
	return p.db.Compact(nil, nil, true)
}

// Stats returns database statistics.
func (p *PebbleBackend) Stats() (pebble.Metrics, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.open {
		return pebble.Metrics{}, ErrBackendClosed
	}

	return p.db.Metrics(), nil
}

// EstimateSize returns an estimate of the total size of data in the given range.
func (p *PebbleBackend) EstimateSize(start, end types.Hash256) (uint64, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if !p.open {
		return 0, ErrBackendClosed
	}

	size, err := p.db.EstimateDiskUsage(start[:], end[:])
	return size, err
}
