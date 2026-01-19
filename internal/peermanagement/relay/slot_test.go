package relay

import (
	"sync"
	"testing"
	"time"

	"github.com/LeJamon/goXRPLd/internal/peermanagement/protocol"
)

// mockSquelchHandler tracks squelch/unsquelch calls.
type mockSquelchHandler struct {
	mu         sync.Mutex
	squelched  map[protocol.PeerID]time.Duration
	unsquelched []protocol.PeerID
}

func newMockSquelchHandler() *mockSquelchHandler {
	return &mockSquelchHandler{
		squelched: make(map[protocol.PeerID]time.Duration),
	}
}

func (m *mockSquelchHandler) Squelch(validator []byte, peerID protocol.PeerID, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.squelched[peerID] = duration
}

func (m *mockSquelchHandler) Unsquelch(validator []byte, peerID protocol.PeerID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.unsquelched = append(m.unsquelched, peerID)
}

func TestSlotUpdate(t *testing.T) {
	handler := newMockSquelchHandler()
	slot := NewSlot(handler, 3)

	validator := []byte("test-validator")

	// Add first peer
	slot.Update(validator, protocol.PeerID(1), nil)
	if slot.GetState() != SlotStateCounting {
		t.Errorf("Expected SlotStateCounting, got %v", slot.GetState())
	}

	// Verify peer was added
	peers := slot.GetPeers()
	if len(peers) != 1 {
		t.Errorf("Expected 1 peer, got %d", len(peers))
	}
	if peers[protocol.PeerID(1)].State != PeerStateCounting {
		t.Errorf("Expected PeerStateCounting, got %v", peers[protocol.PeerID(1)].State)
	}
}

func TestSlotDeletePeer(t *testing.T) {
	handler := newMockSquelchHandler()
	slot := NewSlot(handler, 3)

	validator := []byte("test-validator")

	// Add peers
	for i := 1; i <= 5; i++ {
		slot.Update(validator, protocol.PeerID(i), nil)
	}

	// Delete a peer
	slot.DeletePeer(validator, protocol.PeerID(1), true)

	peers := slot.GetPeers()
	if len(peers) != 4 {
		t.Errorf("Expected 4 peers after delete, got %d", len(peers))
	}

	if _, exists := peers[protocol.PeerID(1)]; exists {
		t.Error("Peer 1 should have been deleted")
	}
}

func TestSlotInState(t *testing.T) {
	handler := newMockSquelchHandler()
	slot := NewSlot(handler, 3)

	validator := []byte("test-validator")

	// Add peers
	for i := 1; i <= 5; i++ {
		slot.Update(validator, protocol.PeerID(i), nil)
	}

	// All should be in counting state
	if count := slot.InState(PeerStateCounting); count != 5 {
		t.Errorf("Expected 5 peers in counting state, got %d", count)
	}

	if count := slot.InState(PeerStateSquelched); count != 0 {
		t.Errorf("Expected 0 peers in squelched state, got %d", count)
	}
}

func TestSquelchAddAndExpire(t *testing.T) {
	squelch := NewSquelch()

	validator := []byte("test-validator")

	// Add squelch with valid duration
	if !squelch.AddSquelch(validator, 5*time.Minute) {
		t.Error("AddSquelch should succeed with valid duration")
	}

	// Check it's squelched
	if !squelch.IsSquelched(validator) {
		t.Error("Validator should be squelched")
	}

	// Check count
	if squelch.GetSquelchedCount() != 1 {
		t.Errorf("Expected 1 squelched validator, got %d", squelch.GetSquelchedCount())
	}

	// Remove squelch
	squelch.RemoveSquelch(validator)

	if squelch.IsSquelched(validator) {
		t.Error("Validator should not be squelched after removal")
	}
}

func TestSquelchInvalidDuration(t *testing.T) {
	squelch := NewSquelch()

	validator := []byte("test-validator")

	// Invalid duration (too short)
	if squelch.AddSquelch(validator, 1*time.Second) {
		t.Error("AddSquelch should fail with invalid duration")
	}

	// Invalid duration (too long)
	if squelch.AddSquelch(validator, 2*time.Hour) {
		t.Error("AddSquelch should fail with duration too long")
	}
}

func TestSlotsContainer(t *testing.T) {
	handler := newMockSquelchHandler()
	config := DefaultSlotsConfig()
	slots := NewSlots(handler, config)

	validator1 := []byte("validator-1")
	validator2 := []byte("validator-2")

	// Update slots for different validators
	slots.UpdateSlotAndSquelch([]byte("msg1"), validator1, protocol.PeerID(1), nil)
	slots.UpdateSlotAndSquelch([]byte("msg2"), validator2, protocol.PeerID(2), nil)

	if slots.SlotCount() != 2 {
		t.Errorf("Expected 2 slots, got %d", slots.SlotCount())
	}
}

func TestMessageTracker(t *testing.T) {
	tracker := NewMessageTracker()

	// First message from peer should succeed
	if !tracker.Add([]byte("msg1"), protocol.PeerID(1)) {
		t.Error("First message should be added")
	}

	// Same message from same peer should fail
	if tracker.Add([]byte("msg1"), protocol.PeerID(1)) {
		t.Error("Duplicate message should not be added")
	}

	// Same message from different peer should succeed
	if !tracker.Add([]byte("msg1"), protocol.PeerID(2)) {
		t.Error("Message from different peer should be added")
	}

	// Different message from same peer should succeed
	if !tracker.Add([]byte("msg2"), protocol.PeerID(1)) {
		t.Error("Different message should be added")
	}
}
