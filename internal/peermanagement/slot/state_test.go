package slot

import (
	"net"
	"testing"
	"time"
)

func TestNewInboundSlot(t *testing.T) {
	localAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 51235}
	remoteAddr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 51235}

	slot := NewInboundSlot(localAddr, remoteAddr, false)

	if !slot.Inbound() {
		t.Error("Slot should be inbound")
	}

	if slot.Fixed() {
		t.Error("Slot should not be fixed")
	}

	if slot.State() != StateAccept {
		t.Errorf("Expected StateAccept, got %v", slot.State())
	}

	if slot.RemoteEndpoint().String() != remoteAddr.String() {
		t.Errorf("Expected remote endpoint %s, got %s", remoteAddr, slot.RemoteEndpoint())
	}
}

func TestNewOutboundSlot(t *testing.T) {
	remoteAddr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 51235}

	slot := NewOutboundSlot(remoteAddr, true)

	if slot.Inbound() {
		t.Error("Slot should not be inbound")
	}

	if !slot.Fixed() {
		t.Error("Slot should be fixed")
	}

	if slot.State() != StateConnect {
		t.Errorf("Expected StateConnect, got %v", slot.State())
	}
}

func TestSlotStateTransitions(t *testing.T) {
	remoteAddr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 51235}
	slot := NewOutboundSlot(remoteAddr, false)

	// Initial state
	if slot.State() != StateConnect {
		t.Errorf("Expected StateConnect, got %v", slot.State())
	}

	// Transition to connected
	slot.SetState(StateConnected)
	if slot.State() != StateConnected {
		t.Errorf("Expected StateConnected, got %v", slot.State())
	}

	// Activate
	slot.Activate()
	if slot.State() != StateActive {
		t.Errorf("Expected StateActive, got %v", slot.State())
	}

	if !slot.IsActive() {
		t.Error("Slot should be active")
	}

	// Transition to closing
	slot.SetState(StateClosing)
	if slot.State() != StateClosing {
		t.Errorf("Expected StateClosing, got %v", slot.State())
	}
}

func TestSlotDuration(t *testing.T) {
	remoteAddr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 51235}
	slot := NewOutboundSlot(remoteAddr, false)

	// Duration should be small initially
	if slot.Duration() > 1*time.Second {
		t.Error("Duration should be very small initially")
	}

	// Active duration should be 0 until activated
	if slot.ActiveDuration() != 0 {
		t.Error("Active duration should be 0 before activation")
	}

	slot.Activate()

	// Now active duration should be tracked
	time.Sleep(10 * time.Millisecond)
	if slot.ActiveDuration() < 10*time.Millisecond {
		t.Error("Active duration should be at least 10ms")
	}
}

func TestSlotReserved(t *testing.T) {
	remoteAddr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 51235}
	slot := NewOutboundSlot(remoteAddr, false)

	if slot.Reserved() {
		t.Error("Slot should not be reserved initially")
	}

	slot.SetReserved(true)

	if !slot.Reserved() {
		t.Error("Slot should be reserved after SetReserved(true)")
	}
}

func TestSlotEndpointCooldown(t *testing.T) {
	remoteAddr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 51235}
	slot := NewOutboundSlot(remoteAddr, false)

	// Should be able to accept endpoints initially
	if !slot.CanAcceptEndpoints() {
		t.Error("Should be able to accept endpoints initially")
	}

	// Set cooldown
	slot.SetEndpointCooldown(100 * time.Millisecond)

	// Should not be able to accept endpoints during cooldown
	if slot.CanAcceptEndpoints() {
		t.Error("Should not accept endpoints during cooldown")
	}

	// Wait for cooldown to expire
	time.Sleep(110 * time.Millisecond)

	// Should be able to accept endpoints again
	if !slot.CanAcceptEndpoints() {
		t.Error("Should accept endpoints after cooldown expires")
	}
}

func TestSlotStateString(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateAccept, "accept"},
		{StateConnect, "connect"},
		{StateConnected, "connected"},
		{StateActive, "active"},
		{StateClosing, "closing"},
		{State(99), "unknown"},
	}

	for _, tc := range tests {
		if tc.state.String() != tc.expected {
			t.Errorf("Expected %s for state %d, got %s", tc.expected, tc.state, tc.state.String())
		}
	}
}

func TestRecentEndpoints(t *testing.T) {
	recent := NewRecentEndpoints()

	// Insert an endpoint
	recent.Insert("192.168.1.1:51235", 1)

	// Should filter the same endpoint with same or higher hops
	if !recent.Filter("192.168.1.1:51235", 1) {
		t.Error("Should filter endpoint with same hops")
	}

	if !recent.Filter("192.168.1.1:51235", 2) {
		t.Error("Should filter endpoint with higher hops")
	}

	// Should not filter with lower hops
	if recent.Filter("192.168.1.1:51235", 0) {
		t.Error("Should not filter endpoint with lower hops")
	}

	// Should not filter unknown endpoint
	if recent.Filter("192.168.1.2:51235", 1) {
		t.Error("Should not filter unknown endpoint")
	}

	// Size should be 1
	if recent.Size() != 1 {
		t.Errorf("Expected size 1, got %d", recent.Size())
	}

	// Clear should remove all entries
	recent.Clear()
	if recent.Size() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", recent.Size())
	}
}
