package nodestore

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// DatabaseImpl wraps a Backend to implement the Database interface.
type DatabaseImpl struct {
	backend Backend
	cache   *Cache
	stats   struct {
		reads       uint64
		cacheHits   uint64
		cacheMisses uint64
		writes      uint64
		readBytes   uint64
		writeBytes  uint64
	}
}

// NewDatabase creates a new Database from a Backend.
func NewDatabase(backend Backend, cacheSize int, cacheTTL time.Duration) *DatabaseImpl {
	var cache *Cache
	if cacheSize > 0 {
		cache = NewCache(cacheSize, cacheTTL)
	}
	return &DatabaseImpl{
		backend: backend,
		cache:   cache,
	}
}

// Store persists a node to the store.
func (d *DatabaseImpl) Store(ctx context.Context, node *Node) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	status := d.backend.Store(node)
	if status != OK {
		return errors.New("store failed: " + status.String())
	}

	atomic.AddUint64(&d.stats.writes, 1)
	atomic.AddUint64(&d.stats.writeBytes, uint64(len(node.Data)))

	// Update cache
	if d.cache != nil {
		d.cache.Put(node)
	}

	return nil
}

// Fetch retrieves a node by its hash.
func (d *DatabaseImpl) Fetch(ctx context.Context, hash Hash256) (*Node, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	atomic.AddUint64(&d.stats.reads, 1)

	// Check cache first
	if d.cache != nil {
		if node, found := d.cache.Get(hash); found {
			atomic.AddUint64(&d.stats.cacheHits, 1)
			return node, nil
		}
		atomic.AddUint64(&d.stats.cacheMisses, 1)
	}

	node, status := d.backend.Fetch(hash)
	if status == NotFound {
		return nil, nil
	}
	if status != OK {
		return nil, errors.New("fetch failed: " + status.String())
	}

	if node != nil {
		atomic.AddUint64(&d.stats.readBytes, uint64(len(node.Data)))
		// Update cache
		if d.cache != nil {
			d.cache.Put(node)
		}
	}

	return node, nil
}

// FetchBatch retrieves multiple nodes efficiently.
func (d *DatabaseImpl) FetchBatch(ctx context.Context, hashes []Hash256) ([]*Node, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	nodes, status := d.backend.FetchBatch(hashes)
	if status != OK && status != NotFound {
		return nil, errors.New("fetch batch failed: " + status.String())
	}

	return nodes, nil
}

// FetchAsync retrieves a node asynchronously.
func (d *DatabaseImpl) FetchAsync(ctx context.Context, hash Hash256) <-chan Result {
	resultCh := make(chan Result, 1)

	go func() {
		node, err := d.Fetch(ctx, hash)
		resultCh <- Result{Node: node, Err: err}
		close(resultCh)
	}()

	return resultCh
}

// StoreBatch stores multiple nodes efficiently.
func (d *DatabaseImpl) StoreBatch(ctx context.Context, nodes []*Node) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	status := d.backend.StoreBatch(nodes)
	if status != OK {
		return errors.New("store batch failed: " + status.String())
	}

	for _, node := range nodes {
		atomic.AddUint64(&d.stats.writes, 1)
		atomic.AddUint64(&d.stats.writeBytes, uint64(len(node.Data)))
		if d.cache != nil {
			d.cache.Put(node)
		}
	}

	return nil
}

// Sweep removes expired entries from caches.
func (d *DatabaseImpl) Sweep() error {
	if d.cache != nil {
		d.cache.Sweep()
	}
	return nil
}

// Stats returns performance statistics.
func (d *DatabaseImpl) Stats() Statistics {
	stats := Statistics{
		Reads:        atomic.LoadUint64(&d.stats.reads),
		CacheHits:    atomic.LoadUint64(&d.stats.cacheHits),
		CacheMisses:  atomic.LoadUint64(&d.stats.cacheMisses),
		ReadBytes:    atomic.LoadUint64(&d.stats.readBytes),
		Writes:       atomic.LoadUint64(&d.stats.writes),
		WriteBytes:   atomic.LoadUint64(&d.stats.writeBytes),
		BackendName:  d.backend.Name(),
	}

	if d.cache != nil {
		cacheStats := d.cache.Stats()
		stats.CacheSize = uint64(cacheStats.CurrentSize)
		stats.CacheMaxSize = uint64(cacheStats.MaxSize)
	}

	return stats
}

// Close gracefully closes the database.
func (d *DatabaseImpl) Close() error {
	return d.backend.Close()
}

// Sync forces pending writes to disk.
func (d *DatabaseImpl) Sync() error {
	status := d.backend.Sync()
	if status != OK {
		return errors.New("sync failed: " + status.String())
	}
	return nil
}
