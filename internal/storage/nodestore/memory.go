package nodestore

import (
	"fmt"
	"sync"

	"github.com/LeJamon/goXRPLd/internal/types"
)

// MemoryBackend implements an in-memory storage backend for testing and development.
type MemoryBackend struct {
	mu     sync.RWMutex
	data   map[types.Hash256]*Node
	open   bool
	config *Config
}

// NewMemoryBackend creates a new memory backend.
func NewMemoryBackend(config *Config) (Backend, error) {
	if config == nil {
		config = DefaultConfig()
	}

	return &MemoryBackend{
		data:   make(map[types.Hash256]*Node),
		open:   false,
		config: config,
	}, nil
}

// Name returns the name of this backend.
func (m *MemoryBackend) Name() string {
	return "memory"
}

// Open opens the backend for use.
func (m *MemoryBackend) Open(createIfMissing bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.open {
		return fmt.Errorf("backend already open")
	}

	m.open = true
	return nil
}

// Close closes the backend and releases resources.
func (m *MemoryBackend) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.open {
		return nil
	}

	m.open = false

	// Clear data to help with garbage collection
	m.data = make(map[types.Hash256]*Node)

	return nil
}

// IsOpen returns true if the backend is currently open.
func (m *MemoryBackend) IsOpen() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.open
}

// Fetch retrieves a single object by key.
func (m *MemoryBackend) Fetch(key types.Hash256) (*Node, Status) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.open {
		return nil, BackendError
	}

	node, exists := m.data[key]
	if !exists {
		return nil, NotFound
	}

	// Return a copy to prevent external modifications
	return m.copyNode(node), OK
}

// FetchBatch retrieves multiple objects efficiently.
func (m *MemoryBackend) FetchBatch(keys []types.Hash256) ([]*Node, Status) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.open {
		return nil, BackendError
	}

	results := make([]*Node, len(keys))
	for i, key := range keys {
		if node, exists := m.data[key]; exists {
			results[i] = m.copyNode(node)
		}
		// nil for not found items
	}

	return results, OK
}

// Store saves a single object.
func (m *MemoryBackend) Store(node *Node) Status {
	if node == nil {
		return BackendError
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.open {
		return BackendError
	}

	// Store a copy to prevent external modifications
	m.data[node.Hash] = m.copyNode(node)
	return OK
}

// StoreBatch saves multiple objects efficiently.
func (m *MemoryBackend) StoreBatch(nodes []*Node) Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.open {
		return BackendError
	}

	for _, node := range nodes {
		if node != nil {
			m.data[node.Hash] = m.copyNode(node)
		}
	}

	return OK
}

// Sync forces pending writes to be flushed (no-op for memory backend).
func (m *MemoryBackend) Sync() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.open {
		return BackendError
	}

	return OK
}

// ForEach iterates over all objects in the backend.
func (m *MemoryBackend) ForEach(fn func(*Node) error) error {
	m.mu.RLock()
	// Take a snapshot of all nodes while holding the lock
	nodes := make([]*Node, 0, len(m.data))
	for _, node := range m.data {
		nodes = append(nodes, m.copyNode(node))
	}
	m.mu.RUnlock()

	// Iterate over the snapshot without holding the lock
	for _, node := range nodes {
		if err := fn(node); err != nil {
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
	// No-op for memory backend
}

// FdRequired returns the number of file descriptors needed (0 for memory backend).
func (m *MemoryBackend) FdRequired() int {
	return 0
}

// Info returns information about this backend.
func (m *MemoryBackend) Info() BackendInfo {
	return BackendInfo{
		Name:            "memory",
		Description:     "In-memory storage backend for testing and development",
		FileDescriptors: 0,
		Persistent:      false,
		Compression:     false,
	}
}

// Size returns the number of stored objects.
func (m *MemoryBackend) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.data)
}

// ByteSize returns the total size of stored data in bytes.
func (m *MemoryBackend) ByteSize() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := 0
	for _, node := range m.data {
		total += node.Size()
	}
	return total
}

// Clear removes all stored objects.
func (m *MemoryBackend) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.open {
		return fmt.Errorf("backend not open")
	}

	m.data = make(map[types.Hash256]*Node)
	return nil
}

// copyNode creates a deep copy of a node.
func (m *MemoryBackend) copyNode(node *Node) *Node {
	if node == nil {
		return nil
	}

	// Create a copy of the data slice
	dataCopy := make(types.Blob, len(node.Data))
	copy(dataCopy, node.Data)

	return &Node{
		Type:      node.Type,
		Hash:      node.Hash, // Hash256 is a value type, so this is safe
		Data:      dataCopy,
		LedgerSeq: node.LedgerSeq,
		CreatedAt: node.CreatedAt,
	}
}

// Keys returns all keys stored in the backend.
func (m *MemoryBackend) Keys() []types.Hash256 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]types.Hash256, 0, len(m.data))
	for key := range m.data {
		keys = append(keys, key)
	}
	return keys
}

// Contains checks if a key exists in the backend.
func (m *MemoryBackend) Contains(key types.Hash256) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.data[key]
	return exists
}

// Delete removes a single object by key.
func (m *MemoryBackend) Delete(key types.Hash256) Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.open {
		return BackendError
	}

	if _, exists := m.data[key]; !exists {
		return NotFound
	}

	delete(m.data, key)
	return OK
}

// DeleteBatch removes multiple objects by key.
func (m *MemoryBackend) DeleteBatch(keys []types.Hash256) Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.open {
		return BackendError
	}

	for _, key := range keys {
		delete(m.data, key)
	}

	return OK
}
