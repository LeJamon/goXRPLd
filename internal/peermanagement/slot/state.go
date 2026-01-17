// Package slot implements connection slot state management for XRPL peers.
// It provides the state machine for managing peer connection lifecycles.
package slot

import (
	"net"
	"sync"
	"time"

	"github.com/LeJamon/goXRPLd/internal/peermanagement/token"
)

// State represents the connection state of a peer slot.
type State int

const (
	// StateAccept means we're accepting an inbound connection.
	StateAccept State = iota
	// StateConnect means we're attempting an outbound connection.
	StateConnect
	// StateConnected means the TCP connection is established.
	StateConnected
	// StateActive means the handshake is complete and the peer is active.
	StateActive
	// StateClosing means the connection is being closed.
	StateClosing
)

// String returns the string representation of the state.
func (s State) String() string {
	switch s {
	case StateAccept:
		return "accept"
	case StateConnect:
		return "connect"
	case StateConnected:
		return "connected"
	case StateActive:
		return "active"
	case StateClosing:
		return "closing"
	default:
		return "unknown"
	}
}

// Slot represents a peer connection slot with its state and properties.
type Slot struct {
	mu sync.RWMutex

	// Connection properties
	inbound        bool
	fixed          bool
	reserved       bool
	state          State
	remoteEndpoint net.Addr
	localEndpoint  net.Addr
	publicKey      *token.PublicKey
	listeningPort  uint16

	// Connectivity check
	checked                     bool
	canAccept                   bool
	connectivityCheckInProgress bool

	// Endpoint handling
	whenAcceptEndpoints time.Time

	// Recent endpoints tracking
	recentEndpoints *RecentEndpoints

	// Timestamps
	createdAt   time.Time
	activatedAt time.Time
}

// NewInboundSlot creates a new slot for an inbound connection.
func NewInboundSlot(localEndpoint, remoteEndpoint net.Addr, fixed bool) *Slot {
	return &Slot{
		inbound:         true,
		fixed:           fixed,
		reserved:        false,
		state:           StateAccept,
		remoteEndpoint:  remoteEndpoint,
		localEndpoint:   localEndpoint,
		recentEndpoints: NewRecentEndpoints(),
		createdAt:       time.Now(),
	}
}

// NewOutboundSlot creates a new slot for an outbound connection.
func NewOutboundSlot(remoteEndpoint net.Addr, fixed bool) *Slot {
	return &Slot{
		inbound:         false,
		fixed:           fixed,
		reserved:        false,
		state:           StateConnect,
		remoteEndpoint:  remoteEndpoint,
		checked:         true, // Outbound connections are always considered checked
		recentEndpoints: NewRecentEndpoints(),
		createdAt:       time.Now(),
	}
}

// Inbound returns true if this is an inbound connection.
func (s *Slot) Inbound() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inbound
}

// Fixed returns true if this is a fixed connection.
func (s *Slot) Fixed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fixed
}

// Reserved returns true if this is a reserved connection.
func (s *Slot) Reserved() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reserved
}

// SetReserved sets the reserved flag.
func (s *Slot) SetReserved(reserved bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reserved = reserved
}

// State returns the current connection state.
func (s *Slot) State() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// SetState updates the connection state.
func (s *Slot) SetState(state State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
}

// RemoteEndpoint returns the remote endpoint.
func (s *Slot) RemoteEndpoint() net.Addr {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.remoteEndpoint
}

// SetRemoteEndpoint sets the remote endpoint.
func (s *Slot) SetRemoteEndpoint(endpoint net.Addr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.remoteEndpoint = endpoint
}

// LocalEndpoint returns the local endpoint.
func (s *Slot) LocalEndpoint() net.Addr {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.localEndpoint
}

// SetLocalEndpoint sets the local endpoint.
func (s *Slot) SetLocalEndpoint(endpoint net.Addr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.localEndpoint = endpoint
}

// PublicKey returns the peer's public key.
func (s *Slot) PublicKey() *token.PublicKey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.publicKey
}

// SetPublicKey sets the peer's public key.
func (s *Slot) SetPublicKey(key *token.PublicKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publicKey = key
}

// ListeningPort returns the peer's listening port.
func (s *Slot) ListeningPort() uint16 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listeningPort
}

// SetListeningPort sets the peer's listening port.
func (s *Slot) SetListeningPort(port uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeningPort = port
}

// Activate transitions the slot to the active state.
func (s *Slot) Activate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = StateActive
	s.activatedAt = time.Now()
}

// IsActive returns true if the slot is in the active state.
func (s *Slot) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state == StateActive
}

// CanAcceptEndpoints returns true if we can accept endpoints from this peer.
func (s *Slot) CanAcceptEndpoints() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Now().After(s.whenAcceptEndpoints)
}

// SetEndpointCooldown sets a cooldown period before accepting more endpoints.
func (s *Slot) SetEndpointCooldown(duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.whenAcceptEndpoints = time.Now().Add(duration)
}

// RecentEndpoints returns the recent endpoints tracker.
func (s *Slot) RecentEndpoints() *RecentEndpoints {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.recentEndpoints
}

// Expire expires old entries in the recent endpoints cache.
func (s *Slot) Expire() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recentEndpoints != nil {
		s.recentEndpoints.Expire()
	}
}

// Duration returns the duration since the slot was created.
func (s *Slot) Duration() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.createdAt)
}

// ActiveDuration returns the duration since the slot became active.
func (s *Slot) ActiveDuration() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.activatedAt.IsZero() {
		return 0
	}
	return time.Since(s.activatedAt)
}
