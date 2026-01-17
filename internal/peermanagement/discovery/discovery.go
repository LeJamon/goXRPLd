// Package discovery implements peer discovery for the XRPL network.
// It manages known peers, handles endpoint announcements, and maintains
// a healthy set of connections.
package discovery

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/LeJamon/goXRPLd/internal/peermanagement/message"
	"github.com/LeJamon/goXRPLd/internal/peermanagement/protocol"
)

const (
	// DefaultMaxPeers is the default maximum number of peer connections.
	DefaultMaxPeers = 21

	// DefaultMinPeers is the default minimum number of peer connections to maintain.
	DefaultMinPeers = 10

	// DefaultEndpointBroadcastInterval is how often we broadcast our endpoints.
	DefaultEndpointBroadcastInterval = 5 * time.Minute

	// DefaultEndpointMaxAge is how long to keep an endpoint before pruning.
	DefaultEndpointMaxAge = 1 * time.Hour

	// DefaultConnectTimeout is the timeout for connecting to a peer.
	DefaultConnectTimeout = 30 * time.Second

	// MaxHops is the maximum hop count for relayed endpoints.
	MaxHops = 3
)

// PeerInfo stores information about a discovered peer.
type PeerInfo struct {
	Address   string
	Hops      uint32
	LastSeen  time.Time
	Connected bool
	PeerID    protocol.PeerID
	Source    protocol.PeerID
}

// Config holds discovery configuration.
type Config struct {
	// MaxPeers is the maximum number of peer connections.
	MaxPeers int

	// MinPeers is the minimum number of peer connections to maintain.
	MinPeers int

	// EndpointBroadcastInterval is how often we broadcast our endpoints.
	EndpointBroadcastInterval time.Duration

	// EndpointMaxAge is how long to keep an endpoint before pruning.
	EndpointMaxAge time.Duration

	// ConnectTimeout is the timeout for connecting to a peer.
	ConnectTimeout time.Duration

	// BootstrapPeers are initial peers to connect to.
	BootstrapPeers []string

	// ListenAddress is our advertised listen address.
	ListenAddress string
}

// DefaultConfig returns the default discovery configuration.
func DefaultConfig() Config {
	return Config{
		MaxPeers:                  DefaultMaxPeers,
		MinPeers:                  DefaultMinPeers,
		EndpointBroadcastInterval: DefaultEndpointBroadcastInterval,
		EndpointMaxAge:            DefaultEndpointMaxAge,
		ConnectTimeout:            DefaultConnectTimeout,
	}
}

// Discovery manages peer discovery and connection maintenance.
type Discovery struct {
	mu sync.RWMutex

	config Config

	// peers maps peer address to peer info
	peers map[string]*PeerInfo

	// connected tracks currently connected peer IDs
	connected map[protocol.PeerID]*PeerInfo

	// handlers for endpoint messages
	endpointHandler *protocol.EndpointsHandler

	// callbacks
	onConnect    func(address string) error
	onDisconnect func(peerID protocol.PeerID)

	// close channel
	closeCh chan struct{}
	wg      sync.WaitGroup
}

// NewDiscovery creates a new Discovery instance.
func NewDiscovery(cfg Config) *Discovery {
	if cfg.MaxPeers <= 0 {
		cfg.MaxPeers = DefaultMaxPeers
	}
	if cfg.MinPeers <= 0 {
		cfg.MinPeers = DefaultMinPeers
	}

	d := &Discovery{
		config:          cfg,
		peers:           make(map[string]*PeerInfo),
		connected:       make(map[protocol.PeerID]*PeerInfo),
		endpointHandler: protocol.NewEndpointsHandler(),
		closeCh:         make(chan struct{}),
	}

	// Set up endpoint handler callback
	d.endpointHandler.OnEndpoint = d.handleEndpoint

	return d
}

// SetConnectCallback sets the callback for connecting to a peer.
func (d *Discovery) SetConnectCallback(fn func(address string) error) {
	d.mu.Lock()
	d.onConnect = fn
	d.mu.Unlock()
}

// SetDisconnectCallback sets the callback for when a peer disconnects.
func (d *Discovery) SetDisconnectCallback(fn func(peerID protocol.PeerID)) {
	d.mu.Lock()
	d.onDisconnect = fn
	d.mu.Unlock()
}

// Handler returns the endpoint message handler.
func (d *Discovery) Handler() protocol.Handler {
	return d.endpointHandler
}

// Start starts the discovery background tasks.
func (d *Discovery) Start(ctx context.Context) error {
	// Add bootstrap peers
	for _, addr := range d.config.BootstrapPeers {
		d.AddPeer(addr, 0, 0)
	}

	// Start maintenance loop
	d.wg.Add(1)
	go d.maintenanceLoop(ctx)

	return nil
}

// Stop stops the discovery service.
func (d *Discovery) Stop() {
	close(d.closeCh)
	d.wg.Wait()
}

// AddPeer adds a peer to the discovery list.
func (d *Discovery) AddPeer(address string, hops uint32, source protocol.PeerID) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if we already know this peer
	existing, exists := d.peers[address]
	if exists {
		// Update if we have a shorter path
		if hops < existing.Hops {
			existing.Hops = hops
			existing.Source = source
		}
		existing.LastSeen = time.Now()
		return
	}

	// Add new peer
	d.peers[address] = &PeerInfo{
		Address:  address,
		Hops:     hops,
		LastSeen: time.Now(),
		Source:   source,
	}
}

// RemovePeer removes a peer from the discovery list.
func (d *Discovery) RemovePeer(address string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.peers, address)
}

// MarkConnected marks a peer as connected.
func (d *Discovery) MarkConnected(address string, peerID protocol.PeerID) {
	d.mu.Lock()
	defer d.mu.Unlock()

	peer, exists := d.peers[address]
	if !exists {
		peer = &PeerInfo{
			Address:  address,
			LastSeen: time.Now(),
		}
		d.peers[address] = peer
	}

	peer.Connected = true
	peer.PeerID = peerID
	d.connected[peerID] = peer
}

// MarkDisconnected marks a peer as disconnected.
func (d *Discovery) MarkDisconnected(peerID protocol.PeerID) {
	d.mu.Lock()
	defer d.mu.Unlock()

	peer, exists := d.connected[peerID]
	if exists {
		peer.Connected = false
		peer.PeerID = 0
		delete(d.connected, peerID)
	}

	if d.onDisconnect != nil {
		d.onDisconnect(peerID)
	}
}

// ConnectedCount returns the number of connected peers.
func (d *Discovery) ConnectedCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.connected)
}

// PeerCount returns the total number of known peers.
func (d *Discovery) PeerCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.peers)
}

// GetConnectedPeers returns all connected peer IDs.
func (d *Discovery) GetConnectedPeers() []protocol.PeerID {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]protocol.PeerID, 0, len(d.connected))
	for peerID := range d.connected {
		result = append(result, peerID)
	}
	return result
}

// GetEndpointsToShare returns endpoints to share with peers.
// This includes our own address and endpoints from direct connections.
func (d *Discovery) GetEndpointsToShare() []message.Endpointv2 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	endpoints := make([]message.Endpointv2, 0)

	// Add our own address if configured
	if d.config.ListenAddress != "" {
		endpoints = append(endpoints, message.Endpointv2{
			Endpoint: d.config.ListenAddress,
			Hops:     0,
		})
	}

	// Add directly connected peers (hops = 1 from us)
	for _, peer := range d.connected {
		if peer.Address != "" {
			endpoints = append(endpoints, message.Endpointv2{
				Endpoint: peer.Address,
				Hops:     1,
			})
		}
	}

	// Limit the number of endpoints we share
	if len(endpoints) > 10 {
		endpoints = endpoints[:10]
	}

	return endpoints
}

// NeedsMorePeers returns true if we should try to connect to more peers.
func (d *Discovery) NeedsMorePeers() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.connected) < d.config.MinPeers
}

// SelectPeersToConnect selects candidate peers to connect to.
func (d *Discovery) SelectPeersToConnect(count int) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Build list of unconnected peers
	candidates := make([]*PeerInfo, 0)
	for _, peer := range d.peers {
		if !peer.Connected {
			candidates = append(candidates, peer)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Sort by hops (prefer closer peers)
	// Using simple selection for now
	selected := make([]string, 0, count)

	// Shuffle candidates
	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	// Select up to count peers, preferring lower hop counts
	for _, peer := range candidates {
		if len(selected) >= count {
			break
		}
		if peer.Hops <= MaxHops {
			selected = append(selected, peer.Address)
		}
	}

	return selected
}

// handleEndpoint handles incoming endpoint messages.
func (d *Discovery) handleEndpoint(ctx context.Context, peerID protocol.PeerID, ep message.Endpointv2) {
	// Increment hops since this came from a peer
	hops := ep.Hops + 1
	if hops > MaxHops {
		return // Too far away
	}

	d.AddPeer(ep.Endpoint, hops, peerID)
}

// maintenanceLoop runs periodic maintenance tasks.
func (d *Discovery) maintenanceLoop(ctx context.Context) {
	defer d.wg.Done()

	pruneTicker := time.NewTicker(d.config.EndpointMaxAge / 4)
	defer pruneTicker.Stop()

	connectTicker := time.NewTicker(30 * time.Second)
	defer connectTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.closeCh:
			return
		case <-pruneTicker.C:
			d.pruneOldPeers()
		case <-connectTicker.C:
			d.tryConnectMore()
		}
	}
}

// pruneOldPeers removes peers that haven't been seen recently.
func (d *Discovery) pruneOldPeers() {
	d.mu.Lock()
	defer d.mu.Unlock()

	cutoff := time.Now().Add(-d.config.EndpointMaxAge)
	for addr, peer := range d.peers {
		if !peer.Connected && peer.LastSeen.Before(cutoff) {
			delete(d.peers, addr)
		}
	}

	// Also prune the endpoint handler's cache
	d.endpointHandler.PruneOld(d.config.EndpointMaxAge)
}

// tryConnectMore attempts to connect to more peers if needed.
func (d *Discovery) tryConnectMore() {
	if !d.NeedsMorePeers() {
		return
	}

	d.mu.RLock()
	onConnect := d.onConnect
	d.mu.RUnlock()

	if onConnect == nil {
		return
	}

	// Try to connect to a few peers
	toConnect := d.config.MinPeers - d.ConnectedCount()
	if toConnect > 3 {
		toConnect = 3 // Don't try too many at once
	}

	candidates := d.SelectPeersToConnect(toConnect)
	for _, addr := range candidates {
		go func(address string) {
			if err := onConnect(address); err != nil {
				// Failed to connect, peer might be offline
				// Keep it in the list for retry later
			}
		}(addr)
	}
}

// GetPeerInfo returns information about a specific peer.
func (d *Discovery) GetPeerInfo(address string) (*PeerInfo, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	peer, exists := d.peers[address]
	if !exists {
		return nil, false
	}

	// Return a copy
	info := *peer
	return &info, true
}

// GetAllPeers returns information about all known peers.
func (d *Discovery) GetAllPeers() []*PeerInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]*PeerInfo, 0, len(d.peers))
	for _, peer := range d.peers {
		info := *peer
		result = append(result, &info)
	}
	return result
}
