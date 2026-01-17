package relay

import (
	"sync"
	"time"

	"github.com/LeJamon/goXRPLd/internal/peermanagement/protocol"
)

// Slots manages reduce-relay slots for multiple validators.
type Slots struct {
	mu sync.RWMutex

	// slots maps validator public key to their slot
	slots map[string]*Slot

	// handler for squelch/unsquelch callbacks
	handler SquelchHandler

	// peersWithMessage tracks which peers have sent which messages
	peersWithMessage *MessageTracker

	// configuration
	baseSquelchEnabled bool
	maxSelectedPeers   int

	// reduceRelayReady indicates if reduce-relay is enabled
	reduceRelayReady bool
	startTime        time.Time
}

// SlotsConfig holds configuration for the Slots container.
type SlotsConfig struct {
	BaseSquelchEnabled bool
	MaxSelectedPeers   int
}

// DefaultSlotsConfig returns the default configuration.
func DefaultSlotsConfig() SlotsConfig {
	return SlotsConfig{
		BaseSquelchEnabled: true,
		MaxSelectedPeers:   MaxSelectedPeers,
	}
}

// NewSlots creates a new Slots container.
func NewSlots(handler SquelchHandler, config SlotsConfig) *Slots {
	return &Slots{
		slots:              make(map[string]*Slot),
		handler:            handler,
		peersWithMessage:   NewMessageTracker(),
		baseSquelchEnabled: config.BaseSquelchEnabled,
		maxSelectedPeers:   config.MaxSelectedPeers,
		startTime:          time.Now(),
	}
}

// BaseSquelchReady returns true if base squelching is enabled and ready.
func (s *Slots) BaseSquelchReady() bool {
	return s.baseSquelchEnabled && s.ReduceRelayReady()
}

// ReduceRelayReady returns true if reduce-relay is ready.
func (s *Slots) ReduceRelayReady() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.reduceRelayReady {
		s.reduceRelayReady = time.Since(s.startTime) > WaitOnBootup
	}
	return s.reduceRelayReady
}

// UpdateSlotAndSquelch updates the slot for a validator message.
func (s *Slots) UpdateSlotAndSquelch(
	messageHash []byte,
	validator []byte,
	peerID protocol.PeerID,
	onIgnoredSquelch func(),
) {
	// Check for duplicate message from same peer
	if !s.peersWithMessage.Add(messageHash, peerID) {
		return
	}

	s.mu.Lock()
	validatorKey := string(validator)
	slot, exists := s.slots[validatorKey]
	if !exists {
		slot = NewSlot(s.handler, s.maxSelectedPeers)
		s.slots[validatorKey] = slot
	}
	s.mu.Unlock()

	slot.Update(validator, peerID, onIgnoredSquelch)
}

// DeleteIdlePeers checks all slots for idle peers and removes them.
func (s *Slots) DeleteIdlePeers() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for validatorKey, slot := range s.slots {
		slot.DeleteIdlePeer([]byte(validatorKey))

		// Remove idle slots
		if time.Since(slot.GetLastSelected()) > MaxUnsquelchExpireDefault {
			delete(s.slots, validatorKey)
		}
	}
	_ = now
}

// DeletePeer removes a peer from all slots.
func (s *Slots) DeletePeer(peerID protocol.PeerID, erase bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for validatorKey, slot := range s.slots {
		slot.DeletePeer([]byte(validatorKey), peerID, erase)
	}
}

// InState returns the count of peers in the given state for a validator.
func (s *Slots) InState(validator []byte, state PeerState) (int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	slot, exists := s.slots[string(validator)]
	if !exists {
		return 0, false
	}
	return slot.InState(state), true
}

// NotInState returns the count of peers not in the given state for a validator.
func (s *Slots) NotInState(validator []byte, state PeerState) (int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	slot, exists := s.slots[string(validator)]
	if !exists {
		return 0, false
	}
	return slot.NotInState(state), true
}

// GetSlotState returns the state of a validator's slot.
func (s *Slots) GetSlotState(validator []byte) (SlotState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	slot, exists := s.slots[string(validator)]
	if !exists {
		return SlotStateCounting, false
	}
	return slot.GetState(), true
}

// GetSelected returns the selected peers for a validator.
func (s *Slots) GetSelected(validator []byte) []protocol.PeerID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	slot, exists := s.slots[string(validator)]
	if !exists {
		return nil
	}
	return slot.GetSelected()
}

// GetPeers returns peer information for a validator.
func (s *Slots) GetPeers(validator []byte) map[protocol.PeerID]*PeerInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	slot, exists := s.slots[string(validator)]
	if !exists {
		return nil
	}
	return slot.GetPeers()
}

// SlotCount returns the number of validator slots.
func (s *Slots) SlotCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.slots)
}

// MessageTracker tracks which peers have sent which messages.
type MessageTracker struct {
	mu      sync.Mutex
	entries map[string]map[protocol.PeerID]time.Time
}

// NewMessageTracker creates a new MessageTracker.
func NewMessageTracker() *MessageTracker {
	return &MessageTracker{
		entries: make(map[string]map[protocol.PeerID]time.Time),
	}
}

// Add records that a peer sent a message. Returns false if duplicate.
func (t *MessageTracker) Add(messageHash []byte, peerID protocol.PeerID) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.expire()

	key := string(messageHash)
	if key == "" {
		return true // Allow empty hashes
	}

	peers, exists := t.entries[key]
	if !exists {
		t.entries[key] = map[protocol.PeerID]time.Time{peerID: time.Now()}
		return true
	}

	if _, seen := peers[peerID]; seen {
		return false // Duplicate
	}

	peers[peerID] = time.Now()
	return true
}

// expire removes old entries.
func (t *MessageTracker) expire() {
	now := time.Now()
	for hash, peers := range t.entries {
		for peerID, timestamp := range peers {
			if now.Sub(timestamp) > Idled {
				delete(peers, peerID)
			}
		}
		if len(peers) == 0 {
			delete(t.entries, hash)
		}
	}
}
