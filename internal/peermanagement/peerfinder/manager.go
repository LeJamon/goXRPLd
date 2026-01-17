package peerfinder

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/LeJamon/goXRPLd/internal/peermanagement/slot"
	"github.com/LeJamon/goXRPLd/internal/peermanagement/token"
)

const (
	// DefaultMaxPeers is the default maximum number of peers.
	DefaultMaxPeers = 21

	// DefaultOutPeers is the default number of outbound peers to maintain.
	DefaultOutPeers = 10

	// DefaultInPeers is the default number of inbound peer slots.
	DefaultInPeers = 11

	// DefaultConnectTimeout is the default connection timeout.
	DefaultConnectTimeout = 30 * time.Second

	// DefaultEndpointCooldown is the cooldown between endpoint messages from a peer.
	DefaultEndpointCooldown = 1 * time.Second

	// MaintenanceInterval is how often to run maintenance tasks.
	MaintenanceInterval = 30 * time.Second

	// CacheSaveInterval is how often to save the boot cache.
	CacheSaveInterval = 5 * time.Minute
)

// Config holds PeerFinder configuration.
type Config struct {
	// MaxPeers is the maximum total number of peers.
	MaxPeers int

	// OutPeers is the target number of outbound peers.
	OutPeers int

	// InPeers is the maximum number of inbound peers.
	InPeers int

	// WantIncoming indicates whether we accept incoming connections.
	WantIncoming bool

	// AutoConnect enables automatic connection to peers.
	AutoConnect bool

	// FixedPeers are addresses to always maintain connections to.
	FixedPeers []string

	// DataDir is where to store the boot cache.
	DataDir string

	// ConnectTimeout is the connection timeout.
	ConnectTimeout time.Duration
}

// DefaultConfig returns the default PeerFinder configuration.
func DefaultConfig() Config {
	return Config{
		MaxPeers:       DefaultMaxPeers,
		OutPeers:       DefaultOutPeers,
		InPeers:        DefaultInPeers,
		WantIncoming:   true,
		AutoConnect:    true,
		ConnectTimeout: DefaultConnectTimeout,
	}
}

// PeerCallback is called when a peer event occurs.
type PeerCallback func(slot *slot.Slot)

// ConnectCallback is called to establish a new connection.
type ConnectCallback func(address string) error

// Manager manages peer discovery and connection maintenance.
type Manager struct {
	mu sync.RWMutex

	config Config

	// Slots for active connections
	slots map[string]*slot.Slot // address -> slot

	// Boot cache for persistent storage
	bootCache *BootCache

	// Fixed peers that should always be connected
	fixedPeers map[string]bool

	// Endpoints received from peers
	endpoints map[string]*Endpoint

	// Callbacks
	onConnect    ConnectCallback
	onActivate   PeerCallback
	onDeactivate PeerCallback

	// Lifecycle
	closeCh chan struct{}
	wg      sync.WaitGroup
}

// Endpoint represents a discovered peer endpoint.
type Endpoint struct {
	Address   string
	Hops      uint32
	LastSeen  time.Time
	Source    string // Address of the peer that told us about this
	Attempts  int
	LastError error
}

// NewManager creates a new PeerFinder manager.
func NewManager(config Config) *Manager {
	m := &Manager{
		config:     config,
		slots:      make(map[string]*slot.Slot),
		fixedPeers: make(map[string]bool),
		endpoints:  make(map[string]*Endpoint),
		closeCh:    make(chan struct{}),
	}

	// Set up fixed peers
	for _, addr := range config.FixedPeers {
		m.fixedPeers[addr] = true
	}

	// Set up boot cache
	if config.DataDir != "" {
		m.bootCache = NewBootCache(config.DataDir)
	}

	return m
}

// SetConnectCallback sets the callback for establishing connections.
func (m *Manager) SetConnectCallback(cb ConnectCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onConnect = cb
}

// SetActivateCallback sets the callback for when a peer becomes active.
func (m *Manager) SetActivateCallback(cb PeerCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onActivate = cb
}

// SetDeactivateCallback sets the callback for when a peer is deactivated.
func (m *Manager) SetDeactivateCallback(cb PeerCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onDeactivate = cb
}

// Start starts the PeerFinder.
func (m *Manager) Start(ctx context.Context) error {
	// Load boot cache
	if m.bootCache != nil {
		if err := m.bootCache.Load(); err != nil {
			// Log but don't fail
			fmt.Printf("Failed to load boot cache: %v\n", err)
		}
	}

	// Start maintenance loop
	m.wg.Add(1)
	go m.maintenanceLoop(ctx)

	// Start cache save loop
	if m.bootCache != nil {
		m.wg.Add(1)
		go m.cacheSaveLoop(ctx)
	}

	return nil
}

// Stop stops the PeerFinder.
func (m *Manager) Stop() {
	close(m.closeCh)
	m.wg.Wait()

	// Save boot cache
	if m.bootCache != nil {
		m.bootCache.Save()
	}
}

// NewInboundSlot creates a slot for an incoming connection.
func (m *Manager) NewInboundSlot(localAddr, remoteAddr net.Addr) (*slot.Slot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if we have room for inbound connections
	inboundCount := 0
	for _, s := range m.slots {
		if s.Inbound() {
			inboundCount++
		}
	}

	if inboundCount >= m.config.InPeers {
		return nil, fmt.Errorf("too many inbound connections")
	}

	// Check for fixed peer
	addrStr := remoteAddr.String()
	fixed := m.fixedPeers[addrStr]

	s := slot.NewInboundSlot(localAddr, remoteAddr, fixed)
	m.slots[addrStr] = s

	return s, nil
}

// NewOutboundSlot creates a slot for an outgoing connection.
func (m *Manager) NewOutboundSlot(remoteAddr net.Addr) (*slot.Slot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if we have room for outbound connections
	outboundCount := 0
	for _, s := range m.slots {
		if !s.Inbound() {
			outboundCount++
		}
	}

	if outboundCount >= m.config.OutPeers {
		return nil, fmt.Errorf("too many outbound connections")
	}

	addrStr := remoteAddr.String()
	fixed := m.fixedPeers[addrStr]

	s := slot.NewOutboundSlot(remoteAddr, fixed)
	m.slots[addrStr] = s

	return s, nil
}

// OnConnected is called when a connection is established.
func (m *Manager) OnConnected(s *slot.Slot) {
	s.SetState(slot.StateConnected)
}

// OnHandshakeComplete is called when the handshake is complete.
func (m *Manager) OnHandshakeComplete(s *slot.Slot, pubKey *token.PublicKey) {
	m.mu.Lock()
	onActivate := m.onActivate
	m.mu.Unlock()

	s.SetPublicKey(pubKey)
	s.Activate()

	// Add to boot cache
	if m.bootCache != nil && s.RemoteEndpoint() != nil {
		addr := s.RemoteEndpoint().String()
		m.bootCache.MarkSuccess(addr)
	}

	if onActivate != nil {
		onActivate(s)
	}
}

// OnDisconnect is called when a peer disconnects.
func (m *Manager) OnDisconnect(s *slot.Slot) {
	m.mu.Lock()
	onDeactivate := m.onDeactivate
	addr := ""
	if s.RemoteEndpoint() != nil {
		addr = s.RemoteEndpoint().String()
		delete(m.slots, addr)
	}
	m.mu.Unlock()

	if onDeactivate != nil {
		onDeactivate(s)
	}
}

// OnConnectionFailed is called when a connection attempt fails.
func (m *Manager) OnConnectionFailed(addr string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.slots, addr)

	if m.bootCache != nil {
		m.bootCache.MarkFailed(addr)
	}

	// Update endpoint info
	if ep, exists := m.endpoints[addr]; exists {
		ep.Attempts++
		ep.LastError = err
	}
}

// AddEndpoint adds a discovered endpoint.
func (m *Manager) AddEndpoint(address string, hops uint32, source string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.endpoints[address]
	if exists {
		// Update if this path is shorter
		if hops < existing.Hops {
			existing.Hops = hops
			existing.Source = source
		}
		existing.LastSeen = time.Now()
		return
	}

	m.endpoints[address] = &Endpoint{
		Address:  address,
		Hops:     hops,
		LastSeen: time.Now(),
		Source:   source,
	}

	// Also add to boot cache
	if m.bootCache != nil {
		host, portStr, err := net.SplitHostPort(address)
		if err == nil {
			var port uint16
			fmt.Sscanf(portStr, "%d", &port)
			m.bootCache.Insert(host+":"+portStr, port)
		}
	}
}

// GetEndpointsToShare returns endpoints suitable for sharing with a peer.
func (m *Manager) GetEndpointsToShare(slot *slot.Slot, limit int) []*Endpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Endpoint
	recent := slot.RecentEndpoints()

	for _, ep := range m.endpoints {
		// Don't send back endpoints the peer recently gave us
		if recent != nil && recent.Filter(ep.Address, ep.Hops) {
			continue
		}

		result = append(result, &Endpoint{
			Address:  ep.Address,
			Hops:     ep.Hops + 1, // Increment hops when forwarding
			LastSeen: ep.LastSeen,
		})

		if limit > 0 && len(result) >= limit {
			break
		}
	}

	return result
}

// SelectPeersToConnect returns addresses to attempt connections to.
func (m *Manager) SelectPeersToConnect() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// First, check fixed peers
	var needed []string
	for addr := range m.fixedPeers {
		if _, connected := m.slots[addr]; !connected {
			needed = append(needed, addr)
		}
	}

	// Check how many more outbound connections we need
	outboundCount := 0
	for _, s := range m.slots {
		if !s.Inbound() {
			outboundCount++
		}
	}

	remaining := m.config.OutPeers - outboundCount - len(needed)
	if remaining <= 0 {
		return needed
	}

	// Collect candidates from endpoints and boot cache
	candidates := make(map[string]int) // address -> priority

	// Endpoints have priority based on hops
	for addr, ep := range m.endpoints {
		if _, connected := m.slots[addr]; !connected {
			candidates[addr] = int(100 - ep.Hops*10) // Lower hops = higher priority
		}
	}

	// Boot cache entries
	if m.bootCache != nil {
		for _, entry := range m.bootCache.GetEndpoints(50) {
			addr := entry.Address
			if _, connected := m.slots[addr]; !connected {
				if _, exists := candidates[addr]; !exists {
					candidates[addr] = entry.Valence
				}
			}
		}
	}

	// Sort by priority and select
	type candidate struct {
		addr     string
		priority int
	}
	sorted := make([]candidate, 0, len(candidates))
	for addr, priority := range candidates {
		sorted = append(sorted, candidate{addr, priority})
	}

	// Sort descending by priority
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].priority > sorted[i].priority {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Add some randomness - shuffle top candidates
	topN := remaining * 2
	if topN > len(sorted) {
		topN = len(sorted)
	}
	if topN > 0 {
		rand.Shuffle(topN, func(i, j int) {
			sorted[i], sorted[j] = sorted[j], sorted[i]
		})
	}

	for i := 0; i < remaining && i < len(sorted); i++ {
		needed = append(needed, sorted[i].addr)
	}

	return needed
}

// GetSlot returns the slot for an address.
func (m *Manager) GetSlot(addr string) *slot.Slot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.slots[addr]
}

// ActiveCount returns the number of active connections.
func (m *Manager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, s := range m.slots {
		if s.IsActive() {
			count++
		}
	}
	return count
}

// OutboundCount returns the number of outbound connections.
func (m *Manager) OutboundCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, s := range m.slots {
		if !s.Inbound() {
			count++
		}
	}
	return count
}

// InboundCount returns the number of inbound connections.
func (m *Manager) InboundCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, s := range m.slots {
		if s.Inbound() {
			count++
		}
	}
	return count
}

// maintenanceLoop runs periodic maintenance tasks.
func (m *Manager) maintenanceLoop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(MaintenanceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.closeCh:
			return
		case <-ticker.C:
			m.maintenance()
		}
	}
}

// maintenance performs periodic maintenance.
func (m *Manager) maintenance() {
	// Prune old endpoints
	m.pruneEndpoints()

	// Expire old slot entries
	m.expireSlotCaches()

	// Try to connect to more peers if needed
	if m.config.AutoConnect {
		m.tryConnect()
	}
}

// pruneEndpoints removes old endpoints.
func (m *Manager) pruneEndpoints() {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-1 * time.Hour)
	for addr, ep := range m.endpoints {
		if ep.LastSeen.Before(cutoff) {
			delete(m.endpoints, addr)
		}
	}
}

// expireSlotCaches expires old entries in slot caches.
func (m *Manager) expireSlotCaches() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, s := range m.slots {
		s.Expire()
	}
}

// tryConnect attempts to establish new connections.
func (m *Manager) tryConnect() {
	m.mu.RLock()
	onConnect := m.onConnect
	m.mu.RUnlock()

	if onConnect == nil {
		return
	}

	addresses := m.SelectPeersToConnect()
	for _, addr := range addresses {
		go func(address string) {
			if err := onConnect(address); err != nil {
				m.OnConnectionFailed(address, err)
			}
		}(addr)
	}
}

// cacheSaveLoop periodically saves the boot cache.
func (m *Manager) cacheSaveLoop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(CacheSaveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.closeCh:
			return
		case <-ticker.C:
			if m.bootCache != nil {
				m.bootCache.Save()
			}
		}
	}
}
