package nodestore

import (
	"sync"
	"sync/atomic"
)

// MemoryBackend implements an in-memory Backend for testing purposes.
// It provides thread-safe operations and is useful for unit tests and development.
type MemoryBackend struct {
	mu   sync.RWMutex
	data map[Hash256]*Node

	open       int64 // atomic flag for open state
	deletePath int64 // atomic flag for delete on close

	// Statistics
	stats struct {
		reads        int64
		writes       int64
		bytesRead    int64
		bytesWritten int64
	}
}

// NewMemoryBackend creates a new in-memory backend.
func NewMemoryBackend() *MemoryBackend {
	return &MemoryBackend{
		data: make(map[Hash256]*Node),
	}
}

// NewMemoryBackendFromConfig creates a new in-memory backend from config.
// The config is ignored for memory backends but required for the BackendFactory signature.
func NewMemoryBackendFromConfig(config *Config) (Backend, error) {
	return NewMemoryBackend(), nil
}

// Name returns the name of this backend.
func (m *MemoryBackend) Name() string {
	return "memory"
}

// Open opens the backend for use.
func (m *MemoryBackend) Open(createIfMissing bool) error {
	if !atomic.CompareAndSwapInt64(&m.open, 0, 1) {
		return ErrBackendClosed // Already open, treat as error for consistency
	}
	return nil
}

// Close closes the backend and clears all data.
func (m *MemoryBackend) Close() error {
	if !atomic.CompareAndSwapInt64(&m.open, 1, 0) {
		return nil // Already closed
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Clear all data
	m.data = make(map[Hash256]*Node)

	return nil
}

// IsOpen returns true if the backend is currently open.
func (m *MemoryBackend) IsOpen() bool {
	return atomic.LoadInt64(&m.open) != 0
}

// Fetch retrieves a single object by key.
func (m *MemoryBackend) Fetch(key Hash256) (*Node, Status) {
	if !m.IsOpen() {
		return nil, BackendError
	}

	m.mu.RLock()
	node, found := m.data[key]
	m.mu.RUnlock()

	if !found {
		return nil, NotFound
	}

	// Update stats
	atomic.AddInt64(&m.stats.reads, 1)
	atomic.AddInt64(&m.stats.bytesRead, int64(len(node.Data)))

	// Return a copy to prevent mutation
	nodeCopy := &Node{
		Type:      node.Type,
		Hash:      node.Hash,
		Data:      make(Blob, len(node.Data)),
		LedgerSeq: node.LedgerSeq,
		CreatedAt: node.CreatedAt,
	}
	copy(nodeCopy.Data, node.Data)

	return nodeCopy, OK
}

// FetchBatch retrieves multiple objects efficiently.
func (m *MemoryBackend) FetchBatch(keys []Hash256) ([]*Node, Status) {
	if !m.IsOpen() {
		return nil, BackendError
	}

	results := make([]*Node, len(keys))

	m.mu.RLock()
	defer m.mu.RUnlock()

	for i, key := range keys {
		node, found := m.data[key]
		if found {
			// Return a copy to prevent mutation
			nodeCopy := &Node{
				Type:      node.Type,
				Hash:      node.Hash,
				Data:      make(Blob, len(node.Data)),
				LedgerSeq: node.LedgerSeq,
				CreatedAt: node.CreatedAt,
			}
			copy(nodeCopy.Data, node.Data)
			results[i] = nodeCopy

			atomic.AddInt64(&m.stats.reads, 1)
			atomic.AddInt64(&m.stats.bytesRead, int64(len(node.Data)))
		}
	}

	return results, OK
}

// Store saves a single object.
func (m *MemoryBackend) Store(node *Node) Status {
	if node == nil {
		return BackendError
	}

	if !m.IsOpen() {
		return BackendError
	}

	// Create a copy to prevent external mutation
	nodeCopy := &Node{
		Type:      node.Type,
		Hash:      node.Hash,
		Data:      make(Blob, len(node.Data)),
		LedgerSeq: node.LedgerSeq,
		CreatedAt: node.CreatedAt,
	}
	copy(nodeCopy.Data, node.Data)

	m.mu.Lock()
	m.data[node.Hash] = nodeCopy
	m.mu.Unlock()

	// Update stats
	atomic.AddInt64(&m.stats.writes, 1)
	atomic.AddInt64(&m.stats.bytesWritten, int64(len(node.Data)))

	return OK
}

// StoreBatch saves multiple objects efficiently.
func (m *MemoryBackend) StoreBatch(nodes []*Node) Status {
	if !m.IsOpen() {
		return BackendError
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var totalBytes int64

	for _, node := range nodes {
		if node == nil {
			continue
		}

		// Create a copy to prevent external mutation
		nodeCopy := &Node{
			Type:      node.Type,
			Hash:      node.Hash,
			Data:      make(Blob, len(node.Data)),
			LedgerSeq: node.LedgerSeq,
			CreatedAt: node.CreatedAt,
		}
		copy(nodeCopy.Data, node.Data)

		m.data[node.Hash] = nodeCopy
		totalBytes += int64(len(node.Data))
	}

	// Update stats
	atomic.AddInt64(&m.stats.writes, int64(len(nodes)))
	atomic.AddInt64(&m.stats.bytesWritten, totalBytes)

	return OK
}

// Sync forces pending writes to be flushed (no-op for memory backend).
func (m *MemoryBackend) Sync() Status {
	if !m.IsOpen() {
		return BackendError
	}
	return OK
}

// ForEach iterates over all objects in the backend.
func (m *MemoryBackend) ForEach(fn func(*Node) error) error {
	if !m.IsOpen() {
		return ErrBackendClosed
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, node := range m.data {
		// Create a copy to prevent mutation during iteration
		nodeCopy := &Node{
			Type:      node.Type,
			Hash:      node.Hash,
			Data:      make(Blob, len(node.Data)),
			LedgerSeq: node.LedgerSeq,
			CreatedAt: node.CreatedAt,
		}
		copy(nodeCopy.Data, node.Data)

		if err := fn(nodeCopy); err != nil {
			return err
		}
	}

	return nil
}

// GetWriteLoad returns an estimate of pending write operations (always 0 for memory backend).
func (m *MemoryBackend) GetWriteLoad() int {
	return 0
}

// SetDeletePath marks the backend for deletion when closed (no-op for memory backend).
func (m *MemoryBackend) SetDeletePath() {
	atomic.StoreInt64(&m.deletePath, 1)
}

// FdRequired returns the number of file descriptors needed (0 for memory backend).
func (m *MemoryBackend) FdRequired() int {
	return 0
}

// HasNode checks if a node with the given hash exists.
func (m *MemoryBackend) HasNode(hash Hash256) bool {
	if !m.IsOpen() {
		return false
	}

	m.mu.RLock()
	_, found := m.data[hash]
	m.mu.RUnlock()

	return found
}

// Delete removes a node by its hash.
func (m *MemoryBackend) Delete(hash Hash256) Status {
	if !m.IsOpen() {
		return BackendError
	}

	m.mu.Lock()
	delete(m.data, hash)
	m.mu.Unlock()

	return OK
}

// Stats returns performance statistics.
func (m *MemoryBackend) Stats() BackendStats {
	return BackendStats{
		Reads:        atomic.LoadInt64(&m.stats.reads),
		Writes:       atomic.LoadInt64(&m.stats.writes),
		BytesRead:    atomic.LoadInt64(&m.stats.bytesRead),
		BytesWritten: atomic.LoadInt64(&m.stats.bytesWritten),
		NodeCount:    int64(m.Size()),
	}
}

// Size returns the number of nodes stored in the backend.
func (m *MemoryBackend) Size() int {
	m.mu.RLock()
	size := len(m.data)
	m.mu.RUnlock()
	return size
}

// Info returns information about this backend.
func (m *MemoryBackend) Info() BackendInfo {
	return BackendInfo{
		Name:            "memory",
		Description:     "In-memory storage backend for testing",
		FileDescriptors: 0,
		Persistent:      false,
		Compression:     false,
	}
}

// BackendStats holds statistics for a backend.
type BackendStats struct {
	Reads        int64 // Number of read operations
	Writes       int64 // Number of write operations
	BytesRead    int64 // Total bytes read
	BytesWritten int64 // Total bytes written
	NodeCount    int64 // Number of nodes stored
}

// Clear removes all nodes from the backend without closing it.
func (m *MemoryBackend) Clear() {
	m.mu.Lock()
	m.data = make(map[Hash256]*Node)
	m.mu.Unlock()
}

func init() {
	RegisterBackend("memory", NewMemoryBackendFromConfig)
}
