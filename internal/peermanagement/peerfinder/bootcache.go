// Package peerfinder implements peer discovery and connection management for XRPL.
// It manages finding new peers, maintaining connections, and persisting known peers.
package peerfinder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// DefaultBootCacheFile is the default filename for the boot cache.
	DefaultBootCacheFile = "peerfinder.cache"

	// MaxCachedEndpoints is the maximum number of endpoints to cache.
	MaxCachedEndpoints = 1000

	// CacheEntryTTL is how long to keep an entry in the cache.
	CacheEntryTTL = 7 * 24 * time.Hour // 1 week
)

// CachedEndpoint represents a cached peer endpoint.
type CachedEndpoint struct {
	Address    string    `json:"address"`
	Port       uint16    `json:"port"`
	LastSeen   time.Time `json:"last_seen"`
	Valence    int       `json:"valence"` // How often this peer was successful
	FailCount  int       `json:"fail_count"`
	LastFailed time.Time `json:"last_failed,omitempty"`
}

// BootCache persists known peer addresses across restarts.
type BootCache struct {
	mu       sync.RWMutex
	cache    map[string]*CachedEndpoint
	filePath string
	dirty    bool
}

// NewBootCache creates a new boot cache.
func NewBootCache(dataDir string) *BootCache {
	return &BootCache{
		cache:    make(map[string]*CachedEndpoint),
		filePath: filepath.Join(dataDir, DefaultBootCacheFile),
	}
}

// Load loads the cache from disk.
func (bc *BootCache) Load() error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	data, err := os.ReadFile(bc.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No cache file yet
		}
		return err
	}

	var entries []*CachedEndpoint
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}

	bc.cache = make(map[string]*CachedEndpoint)
	now := time.Now()
	for _, entry := range entries {
		// Skip expired entries
		if now.Sub(entry.LastSeen) > CacheEntryTTL {
			continue
		}
		bc.cache[entry.Address] = entry
	}

	return nil
}

// Save writes the cache to disk.
func (bc *BootCache) Save() error {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	if !bc.dirty {
		return nil
	}

	entries := make([]*CachedEndpoint, 0, len(bc.cache))
	for _, entry := range bc.cache {
		entries = append(entries, entry)
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(bc.filePath), 0755); err != nil {
		return err
	}

	bc.dirty = false
	return os.WriteFile(bc.filePath, data, 0644)
}

// Insert adds or updates an endpoint in the cache.
func (bc *BootCache) Insert(address string, port uint16) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	entry, exists := bc.cache[address]
	if exists {
		entry.LastSeen = time.Now()
		entry.Valence++
	} else {
		bc.cache[address] = &CachedEndpoint{
			Address:  address,
			Port:     port,
			LastSeen: time.Now(),
			Valence:  1,
		}
	}

	bc.dirty = true
	bc.prune()
}

// MarkFailed records a connection failure for an endpoint.
func (bc *BootCache) MarkFailed(address string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	entry, exists := bc.cache[address]
	if exists {
		entry.FailCount++
		entry.LastFailed = time.Now()
		entry.Valence-- // Reduce valence on failure
		if entry.Valence < 0 {
			entry.Valence = 0
		}
		bc.dirty = true
	}
}

// MarkSuccess records a successful connection to an endpoint.
func (bc *BootCache) MarkSuccess(address string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	entry, exists := bc.cache[address]
	if exists {
		entry.LastSeen = time.Now()
		entry.Valence++
		entry.FailCount = 0 // Reset fail count on success
		bc.dirty = true
	}
}

// Remove removes an endpoint from the cache.
func (bc *BootCache) Remove(address string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if _, exists := bc.cache[address]; exists {
		delete(bc.cache, address)
		bc.dirty = true
	}
}

// GetEndpoints returns endpoints sorted by valence (best first).
func (bc *BootCache) GetEndpoints(limit int) []*CachedEndpoint {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	entries := make([]*CachedEndpoint, 0, len(bc.cache))
	for _, entry := range bc.cache {
		entries = append(entries, &CachedEndpoint{
			Address:    entry.Address,
			Port:       entry.Port,
			LastSeen:   entry.LastSeen,
			Valence:    entry.Valence,
			FailCount:  entry.FailCount,
			LastFailed: entry.LastFailed,
		})
	}

	// Sort by valence (descending)
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].Valence > entries[i].Valence {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	if limit > 0 && limit < len(entries) {
		entries = entries[:limit]
	}

	return entries
}

// Size returns the number of entries in the cache.
func (bc *BootCache) Size() int {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return len(bc.cache)
}

// Contains returns true if the address is in the cache.
func (bc *BootCache) Contains(address string) bool {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	_, exists := bc.cache[address]
	return exists
}

// prune removes old or low-valence entries if the cache is too large.
// Must be called with the lock held.
func (bc *BootCache) prune() {
	if len(bc.cache) <= MaxCachedEndpoints {
		return
	}

	// Build list sorted by valence
	type entry struct {
		address string
		valence int
	}
	entries := make([]entry, 0, len(bc.cache))
	for addr, e := range bc.cache {
		entries = append(entries, entry{addr, e.Valence})
	}

	// Sort by valence (ascending - worst first)
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].valence < entries[i].valence {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Remove worst entries
	toRemove := len(bc.cache) - MaxCachedEndpoints
	for i := 0; i < toRemove; i++ {
		delete(bc.cache, entries[i].address)
	}
}

// Clear removes all entries from the cache.
func (bc *BootCache) Clear() {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.cache = make(map[string]*CachedEndpoint)
	bc.dirty = true
}
