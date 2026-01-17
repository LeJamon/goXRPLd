package slot

import (
	"sync"
	"time"
)

const (
	// RecentEndpointTTL is how long to remember an endpoint.
	RecentEndpointTTL = 5 * time.Minute
)

// EndpointEntry represents a recently seen endpoint.
type EndpointEntry struct {
	Hops     uint32
	LastSeen time.Time
}

// RecentEndpoints tracks recently seen endpoints from a peer.
// This is used to avoid sending a peer the same addresses they gave us.
type RecentEndpoints struct {
	mu    sync.RWMutex
	cache map[string]*EndpointEntry
}

// NewRecentEndpoints creates a new RecentEndpoints tracker.
func NewRecentEndpoints() *RecentEndpoints {
	return &RecentEndpoints{
		cache: make(map[string]*EndpointEntry),
	}
}

// Insert records an endpoint as recently seen.
func (r *RecentEndpoints) Insert(endpoint string, hops uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cache[endpoint] = &EndpointEntry{
		Hops:     hops,
		LastSeen: time.Now(),
	}
}

// Filter returns true if we should NOT send this endpoint to the peer.
// Returns true if the endpoint was recently seen from this peer with
// the same or fewer hops.
func (r *RecentEndpoints) Filter(endpoint string, hops uint32) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.cache[endpoint]
	if !exists {
		return false
	}

	// Filter if the entry is still fresh and has same or fewer hops
	if time.Since(entry.LastSeen) < RecentEndpointTTL && entry.Hops <= hops {
		return true
	}

	return false
}

// Expire removes old entries from the cache.
func (r *RecentEndpoints) Expire() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for endpoint, entry := range r.cache {
		if now.Sub(entry.LastSeen) > RecentEndpointTTL {
			delete(r.cache, endpoint)
		}
	}
}

// Size returns the number of entries in the cache.
func (r *RecentEndpoints) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.cache)
}

// Clear removes all entries from the cache.
func (r *RecentEndpoints) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = make(map[string]*EndpointEntry)
}
