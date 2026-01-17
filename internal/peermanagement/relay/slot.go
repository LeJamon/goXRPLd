package relay

import (
	"math/rand"
	"sync"
	"time"

	"github.com/LeJamon/goXRPLd/internal/peermanagement/protocol"
)

// PeerState represents the state of a peer in the reduce-relay system.
type PeerState int

const (
	// PeerStateCounting means the peer is being evaluated for message relay.
	PeerStateCounting PeerState = iota
	// PeerStateSelected means the peer is selected to relay messages.
	PeerStateSelected
	// PeerStateSquelched means the peer should not relay messages.
	PeerStateSquelched
)

// String returns the string representation of PeerState.
func (s PeerState) String() string {
	switch s {
	case PeerStateCounting:
		return "counting"
	case PeerStateSelected:
		return "selected"
	case PeerStateSquelched:
		return "squelched"
	default:
		return "unknown"
	}
}

// SlotState represents the state of a validator slot.
type SlotState int

const (
	// SlotStateCounting means we're counting messages to select peers.
	SlotStateCounting SlotState = iota
	// SlotStateSelected means peers have been selected.
	SlotStateSelected
)

// String returns the string representation of SlotState.
func (s SlotState) String() string {
	switch s {
	case SlotStateCounting:
		return "counting"
	case SlotStateSelected:
		return "selected"
	default:
		return "unknown"
	}
}

// PeerInfo holds information about a peer in the reduce-relay system.
type PeerInfo struct {
	State       PeerState
	Count       int
	Expire      time.Time
	LastMessage time.Time
}

// SquelchHandler defines callbacks for squelch/unsquelch events.
type SquelchHandler interface {
	// Squelch tells a peer to stop relaying messages from a validator.
	Squelch(validator []byte, peerID protocol.PeerID, duration time.Duration)
	// Unsquelch tells a peer to resume relaying messages from a validator.
	Unsquelch(validator []byte, peerID protocol.PeerID)
}

// Slot is associated with a specific validator and manages peer selection
// for message relay optimization.
type Slot struct {
	mu sync.RWMutex

	// peers tracks information about each peer
	peers map[protocol.PeerID]*PeerInfo

	// considered is the pool of peers being considered for selection
	considered map[protocol.PeerID]struct{}

	// reachedThreshold is the number of peers that reached MaxMessageThreshold
	reachedThreshold int

	// lastSelected is the time of the last peer selection round
	lastSelected time.Time

	// state is the current slot state
	state SlotState

	// handler for squelch/unsquelch callbacks
	handler SquelchHandler

	// maxSelectedPeers is the maximum number of peers to select
	maxSelectedPeers int
}

// NewSlot creates a new reduce-relay slot for a validator.
func NewSlot(handler SquelchHandler, maxSelectedPeers int) *Slot {
	if maxSelectedPeers <= 0 {
		maxSelectedPeers = MaxSelectedPeers
	}
	return &Slot{
		peers:            make(map[protocol.PeerID]*PeerInfo),
		considered:       make(map[protocol.PeerID]struct{}),
		reachedThreshold: 0,
		lastSelected:     time.Now(),
		state:            SlotStateCounting,
		handler:          handler,
		maxSelectedPeers: maxSelectedPeers,
	}
}

// Update processes a message from a peer and updates the peer selection state.
func (s *Slot) Update(validator []byte, peerID protocol.PeerID, onIgnoredSquelch func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	peer, exists := s.peers[peerID]

	// First message from this peer
	if !exists {
		s.peers[peerID] = &PeerInfo{
			State:       PeerStateCounting,
			Count:       0,
			Expire:      now,
			LastMessage: now,
		}
		s.initCounting()
		return
	}

	// Message from a peer with expired squelch
	if peer.State == PeerStateSquelched && now.After(peer.Expire) {
		peer.State = PeerStateCounting
		peer.LastMessage = now
		s.initCounting()
		return
	}

	peer.LastMessage = now

	// Report if we received a message from a squelched peer
	if peer.State == PeerStateSquelched && onIgnoredSquelch != nil {
		onIgnoredSquelch()
	}

	if s.state != SlotStateCounting || peer.State == PeerStateSquelched {
		return
	}

	peer.Count++
	if peer.Count > MinMessageThreshold {
		s.considered[peerID] = struct{}{}
	}
	if peer.Count == MaxMessageThreshold+1 {
		s.reachedThreshold++
	}

	// Reset if inactive for too long
	if time.Since(s.lastSelected) > 2*MaxUnsquelchExpireDefault {
		s.initCounting()
		return
	}

	// Select peers when threshold is reached
	if s.reachedThreshold == s.maxSelectedPeers {
		s.selectPeers(validator, now)
	}
}

// selectPeers randomly selects peers from the considered pool.
func (s *Slot) selectPeers(validator []byte, now time.Time) {
	// Build list of non-idle candidates
	candidates := make([]protocol.PeerID, 0, len(s.considered))
	for peerID := range s.considered {
		peer, exists := s.peers[peerID]
		if exists && time.Since(peer.LastMessage) < Idled {
			candidates = append(candidates, peerID)
		}
	}

	// If we don't have enough candidates, reset
	if len(candidates) < s.maxSelectedPeers {
		s.initCounting()
		return
	}

	// Randomly select maxSelectedPeers
	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	selected := make(map[protocol.PeerID]struct{})
	for i := 0; i < s.maxSelectedPeers && i < len(candidates); i++ {
		selected[candidates[i]] = struct{}{}
	}

	s.lastSelected = now

	// Squelch non-selected peers
	squelchablePeers := len(s.peers) - s.maxSelectedPeers
	for peerID, peer := range s.peers {
		peer.Count = 0

		if _, isSelected := selected[peerID]; isSelected {
			peer.State = PeerStateSelected
		} else if peer.State != PeerStateSquelched {
			peer.State = PeerStateSquelched
			duration := s.getSquelchDuration(squelchablePeers)
			peer.Expire = now.Add(duration)
			if s.handler != nil {
				s.handler.Squelch(validator, peerID, duration)
			}
		}
	}

	s.considered = make(map[protocol.PeerID]struct{})
	s.reachedThreshold = 0
	s.state = SlotStateSelected
}

// DeletePeer handles peer disconnection.
func (s *Slot) DeletePeer(validator []byte, peerID protocol.PeerID, erase bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	peer, exists := s.peers[peerID]
	if !exists {
		return
	}

	var toUnsquelch []protocol.PeerID
	now := time.Now()

	// If the deleted peer was selected, unsquelch all squelched peers
	if peer.State == PeerStateSelected {
		for id, p := range s.peers {
			if p.State == PeerStateSquelched {
				toUnsquelch = append(toUnsquelch, id)
			}
			p.State = PeerStateCounting
			p.Count = 0
			p.Expire = now
		}

		s.considered = make(map[protocol.PeerID]struct{})
		s.reachedThreshold = 0
		s.state = SlotStateCounting
	} else if _, inConsidered := s.considered[peerID]; inConsidered {
		if peer.Count > MaxMessageThreshold {
			s.reachedThreshold--
		}
		delete(s.considered, peerID)
	}

	peer.LastMessage = now
	peer.Count = 0

	if erase {
		delete(s.peers, peerID)
	}

	// Unsquelch after releasing lock would be safer, but for simplicity do it here
	for _, id := range toUnsquelch {
		if s.handler != nil {
			s.handler.Unsquelch(validator, id)
		}
	}
}

// DeleteIdlePeer removes peers that have stopped relaying messages.
func (s *Slot) DeleteIdlePeer(validator []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for peerID, peer := range s.peers {
		if time.Since(peer.LastMessage) > Idled {
			// Use a copy to avoid issues with the unlock/lock
			s.mu.Unlock()
			s.DeletePeer(validator, peerID, false)
			s.mu.Lock()
		}
	}
	_ = now // silence unused warning
}

// GetLastSelected returns the time of the last peer selection.
func (s *Slot) GetLastSelected() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSelected
}

// GetState returns the slot state.
func (s *Slot) GetState() SlotState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// GetSelected returns the set of selected peer IDs.
func (s *Slot) GetSelected() []protocol.PeerID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []protocol.PeerID
	for peerID, peer := range s.peers {
		if peer.State == PeerStateSelected {
			result = append(result, peerID)
		}
	}
	return result
}

// InState returns the count of peers in the given state.
func (s *Slot) InState(state PeerState) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, peer := range s.peers {
		if peer.State == state {
			count++
		}
	}
	return count
}

// NotInState returns the count of peers not in the given state.
func (s *Slot) NotInState(state PeerState) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, peer := range s.peers {
		if peer.State != state {
			count++
		}
	}
	return count
}

// GetPeers returns information about all peers.
func (s *Slot) GetPeers() map[protocol.PeerID]*PeerInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[protocol.PeerID]*PeerInfo)
	for peerID, peer := range s.peers {
		result[peerID] = &PeerInfo{
			State:       peer.State,
			Count:       peer.Count,
			Expire:      peer.Expire,
			LastMessage: peer.LastMessage,
		}
	}
	return result
}

// initCounting resets the slot to counting state.
func (s *Slot) initCounting() {
	s.state = SlotStateCounting
	s.considered = make(map[protocol.PeerID]struct{})
	s.reachedThreshold = 0
	s.resetCounts()
}

// resetCounts resets message counts for all non-squelched peers.
func (s *Slot) resetCounts() {
	for _, peer := range s.peers {
		peer.Count = 0
	}
}

// getSquelchDuration calculates the squelch duration based on peer count.
func (s *Slot) getSquelchDuration(numPeers int) time.Duration {
	maxDuration := MaxUnsquelchExpireDefault
	if time.Duration(numPeers)*SquelchPerPeer > maxDuration {
		maxDuration = time.Duration(numPeers) * SquelchPerPeer
	}
	if maxDuration > MaxUnsquelchExpirePeers {
		maxDuration = MaxUnsquelchExpirePeers
	}

	minSecs := int(MinUnsquelchExpire.Seconds())
	maxSecs := int(maxDuration.Seconds())

	return time.Duration(minSecs+rand.Intn(maxSecs-minSecs+1)) * time.Second
}
