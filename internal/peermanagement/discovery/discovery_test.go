package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/LeJamon/goXRPLd/internal/peermanagement/message"
	"github.com/LeJamon/goXRPLd/internal/peermanagement/protocol"
)

func TestNewDiscovery(t *testing.T) {
	cfg := DefaultConfig()
	d := NewDiscovery(cfg)

	if d.config.MaxPeers != DefaultMaxPeers {
		t.Errorf("MaxPeers = %d, want %d", d.config.MaxPeers, DefaultMaxPeers)
	}
	if d.config.MinPeers != DefaultMinPeers {
		t.Errorf("MinPeers = %d, want %d", d.config.MinPeers, DefaultMinPeers)
	}
}

func TestAddPeer(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	d.AddPeer("192.168.1.1:51235", 0, 0)
	d.AddPeer("192.168.1.2:51235", 1, 1)

	if d.PeerCount() != 2 {
		t.Errorf("PeerCount = %d, want 2", d.PeerCount())
	}
}

func TestAddPeerUpdateHops(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	// Add with high hop count
	d.AddPeer("192.168.1.1:51235", 3, 1)

	info, exists := d.GetPeerInfo("192.168.1.1:51235")
	if !exists {
		t.Fatal("Peer should exist")
	}
	if info.Hops != 3 {
		t.Errorf("Hops = %d, want 3", info.Hops)
	}

	// Update with lower hop count
	d.AddPeer("192.168.1.1:51235", 1, 2)

	info, _ = d.GetPeerInfo("192.168.1.1:51235")
	if info.Hops != 1 {
		t.Errorf("Hops = %d, want 1 after update", info.Hops)
	}
}

func TestRemovePeer(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	d.AddPeer("192.168.1.1:51235", 0, 0)
	if d.PeerCount() != 1 {
		t.Fatal("Peer should be added")
	}

	d.RemovePeer("192.168.1.1:51235")
	if d.PeerCount() != 0 {
		t.Error("Peer should be removed")
	}
}

func TestMarkConnected(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	d.AddPeer("192.168.1.1:51235", 0, 0)
	d.MarkConnected("192.168.1.1:51235", protocol.PeerID(1))

	if d.ConnectedCount() != 1 {
		t.Errorf("ConnectedCount = %d, want 1", d.ConnectedCount())
	}

	info, _ := d.GetPeerInfo("192.168.1.1:51235")
	if !info.Connected {
		t.Error("Peer should be marked as connected")
	}
	if info.PeerID != protocol.PeerID(1) {
		t.Errorf("PeerID = %d, want 1", info.PeerID)
	}
}

func TestMarkDisconnected(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	d.AddPeer("192.168.1.1:51235", 0, 0)
	d.MarkConnected("192.168.1.1:51235", protocol.PeerID(1))

	disconnectCalled := false
	d.SetDisconnectCallback(func(peerID protocol.PeerID) {
		disconnectCalled = true
	})

	d.MarkDisconnected(protocol.PeerID(1))

	if d.ConnectedCount() != 0 {
		t.Errorf("ConnectedCount = %d, want 0", d.ConnectedCount())
	}
	if !disconnectCalled {
		t.Error("Disconnect callback should have been called")
	}

	info, _ := d.GetPeerInfo("192.168.1.1:51235")
	if info.Connected {
		t.Error("Peer should be marked as disconnected")
	}
}

func TestGetConnectedPeers(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	d.MarkConnected("192.168.1.1:51235", protocol.PeerID(1))
	d.MarkConnected("192.168.1.2:51235", protocol.PeerID(2))
	d.MarkConnected("192.168.1.3:51235", protocol.PeerID(3))

	peers := d.GetConnectedPeers()
	if len(peers) != 3 {
		t.Errorf("len(peers) = %d, want 3", len(peers))
	}
}

func TestNeedsMorePeers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinPeers = 3
	d := NewDiscovery(cfg)

	if !d.NeedsMorePeers() {
		t.Error("Should need more peers when none connected")
	}

	d.MarkConnected("192.168.1.1:51235", protocol.PeerID(1))
	d.MarkConnected("192.168.1.2:51235", protocol.PeerID(2))
	d.MarkConnected("192.168.1.3:51235", protocol.PeerID(3))

	if d.NeedsMorePeers() {
		t.Error("Should not need more peers when at minimum")
	}
}

func TestSelectPeersToConnect(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	// Add some peers
	d.AddPeer("192.168.1.1:51235", 0, 0)
	d.AddPeer("192.168.1.2:51235", 1, 0)
	d.AddPeer("192.168.1.3:51235", 2, 0)
	d.AddPeer("192.168.1.4:51235", 10, 0) // Too many hops

	// Mark one as connected
	d.MarkConnected("192.168.1.1:51235", protocol.PeerID(1))

	candidates := d.SelectPeersToConnect(3)

	// Should not include connected peer or high-hop peer
	for _, addr := range candidates {
		if addr == "192.168.1.1:51235" {
			t.Error("Should not select already connected peer")
		}
		if addr == "192.168.1.4:51235" {
			t.Error("Should not select peer with too many hops")
		}
	}
}

func TestGetEndpointsToShare(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ListenAddress = "10.0.0.1:51235"
	d := NewDiscovery(cfg)

	d.MarkConnected("192.168.1.1:51235", protocol.PeerID(1))
	d.MarkConnected("192.168.1.2:51235", protocol.PeerID(2))

	endpoints := d.GetEndpointsToShare()

	// Should include our address and connected peers
	if len(endpoints) < 1 {
		t.Error("Should have at least our own endpoint")
	}

	// Check our address is included with hops=0
	found := false
	for _, ep := range endpoints {
		if ep.Endpoint == "10.0.0.1:51235" && ep.Hops == 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("Our own endpoint should be included with hops=0")
	}
}

func TestHandleEndpointMessage(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	// Create endpoint message
	endpoints := &message.Endpoints{
		Version: 2,
		EndpointsV2: []message.Endpointv2{
			{Endpoint: "192.168.1.1:51235", Hops: 0},
			{Endpoint: "192.168.1.2:51235", Hops: 1},
		},
	}

	// Handle message through handler
	ctx := context.Background()
	err := d.Handler().HandleMessage(ctx, protocol.PeerID(1), endpoints)
	if err != nil {
		t.Errorf("HandleMessage error: %v", err)
	}

	// Check peers were added (with hops incremented)
	info1, exists := d.GetPeerInfo("192.168.1.1:51235")
	if !exists {
		t.Error("First peer should be added")
	}
	if info1.Hops != 1 { // 0 + 1
		t.Errorf("First peer hops = %d, want 1", info1.Hops)
	}

	info2, exists := d.GetPeerInfo("192.168.1.2:51235")
	if !exists {
		t.Error("Second peer should be added")
	}
	if info2.Hops != 2 { // 1 + 1
		t.Errorf("Second peer hops = %d, want 2", info2.Hops)
	}
}

func TestHandleEndpointMessageMaxHops(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	// Create endpoint with max hops (should be rejected when incremented)
	endpoints := &message.Endpoints{
		Version: 2,
		EndpointsV2: []message.Endpointv2{
			{Endpoint: "192.168.1.1:51235", Hops: MaxHops}, // Will become MaxHops+1
		},
	}

	ctx := context.Background()
	d.Handler().HandleMessage(ctx, protocol.PeerID(1), endpoints)

	// Peer should not be added (too many hops)
	_, exists := d.GetPeerInfo("192.168.1.1:51235")
	if exists {
		t.Error("Peer with too many hops should not be added")
	}
}

func TestBootstrapPeers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BootstrapPeers = []string{
		"192.168.1.1:51235",
		"192.168.1.2:51235",
	}
	d := NewDiscovery(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := d.Start(ctx)
	if err != nil {
		t.Fatalf("Start error: %v", err)
	}

	// Bootstrap peers should be added
	if d.PeerCount() != 2 {
		t.Errorf("PeerCount = %d, want 2", d.PeerCount())
	}

	d.Stop()
}

func TestGetAllPeers(t *testing.T) {
	d := NewDiscovery(DefaultConfig())

	d.AddPeer("192.168.1.1:51235", 0, 0)
	d.AddPeer("192.168.1.2:51235", 1, 0)
	d.AddPeer("192.168.1.3:51235", 2, 0)

	peers := d.GetAllPeers()
	if len(peers) != 3 {
		t.Errorf("len(peers) = %d, want 3", len(peers))
	}

	// Verify it's a copy
	peers[0].Address = "modified"
	info, _ := d.GetPeerInfo("192.168.1.1:51235")
	if info.Address == "modified" {
		t.Error("GetAllPeers should return copies")
	}
}

func TestConnectCallback(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinPeers = 1
	d := NewDiscovery(cfg)

	connectCalled := false
	d.SetConnectCallback(func(address string) error {
		connectCalled = true
		return nil
	})

	// Add a peer but don't connect
	d.AddPeer("192.168.1.1:51235", 0, 0)

	// Manually trigger connect attempt
	d.tryConnectMore()

	// Wait a bit for goroutine
	time.Sleep(100 * time.Millisecond)

	if !connectCalled {
		t.Error("Connect callback should have been called")
	}
}
