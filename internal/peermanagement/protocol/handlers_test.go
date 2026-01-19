package protocol

import (
	"context"
	"testing"
	"time"

	"github.com/LeJamon/goXRPLd/internal/peermanagement/message"
)

func TestPingHandler(t *testing.T) {
	h := NewPingHandler()

	// Track callback
	var receivedPing *message.Ping
	h.OnPing = func(ctx context.Context, peerID PeerID, ping *message.Ping) {
		receivedPing = ping
	}

	// Handle ping
	ping := &message.Ping{
		PType:    message.PingTypePing,
		Seq:      1,
		PingTime: uint64(time.Now().UnixMilli()),
	}

	err := h.HandleMessage(context.Background(), PeerID(1), ping)
	if err != nil {
		t.Errorf("HandleMessage error: %v", err)
	}

	if receivedPing == nil {
		t.Error("OnPing callback was not called")
	}

	lastPing := h.GetLastPing(PeerID(1))
	if lastPing.IsZero() {
		t.Error("LastPing should be set")
	}

	// Handle pong
	pong := &message.Ping{
		PType:    message.PingTypePong,
		Seq:      1,
		PingTime: uint64(time.Now().Add(-100 * time.Millisecond).UnixMilli()),
	}

	err = h.HandleMessage(context.Background(), PeerID(1), pong)
	if err != nil {
		t.Errorf("HandleMessage error: %v", err)
	}

	latency := h.GetLatency(PeerID(1))
	if latency < 90*time.Millisecond {
		t.Logf("Latency = %v (may vary based on timing)", latency)
	}
}

func TestPingHandlerWrongMessageType(t *testing.T) {
	h := NewPingHandler()

	// Handle wrong message type - should be ignored
	tx := &message.Transaction{RawTransaction: []byte{1, 2, 3}}
	err := h.HandleMessage(context.Background(), PeerID(1), tx)
	if err != nil {
		t.Errorf("Should ignore wrong message type, got error: %v", err)
	}
}

func TestEndpointsHandler(t *testing.T) {
	h := NewEndpointsHandler()

	callbackCount := 0
	h.OnEndpoint = func(ctx context.Context, peerID PeerID, ep message.Endpointv2) {
		callbackCount++
	}

	endpoints := &message.Endpoints{
		Version: 2,
		EndpointsV2: []message.Endpointv2{
			{Endpoint: "192.168.1.1:51235", Hops: 0},
			{Endpoint: "192.168.1.2:51235", Hops: 1},
			{Endpoint: "192.168.1.3:51235", Hops: 2},
		},
	}

	err := h.HandleMessage(context.Background(), PeerID(1), endpoints)
	if err != nil {
		t.Errorf("HandleMessage error: %v", err)
	}

	if callbackCount != 3 {
		t.Errorf("callbackCount = %d, want 3", callbackCount)
	}

	all := h.GetEndpoints()
	if len(all) != 3 {
		t.Errorf("len(endpoints) = %d, want 3", len(all))
	}
}

func TestEndpointsHandlerUpdateHops(t *testing.T) {
	h := NewEndpointsHandler()

	// First receive with hops = 2
	ep1 := &message.Endpoints{
		Version: 2,
		EndpointsV2: []message.Endpointv2{
			{Endpoint: "192.168.1.1:51235", Hops: 2},
		},
	}
	h.HandleMessage(context.Background(), PeerID(1), ep1)

	// Then receive same endpoint with lower hops
	ep2 := &message.Endpoints{
		Version: 2,
		EndpointsV2: []message.Endpointv2{
			{Endpoint: "192.168.1.1:51235", Hops: 1},
		},
	}
	h.HandleMessage(context.Background(), PeerID(2), ep2)

	all := h.GetEndpoints()
	if len(all) != 1 {
		t.Fatalf("len(endpoints) = %d, want 1", len(all))
	}

	// Should have lower hops
	if all[0].Hops != 1 {
		t.Errorf("Hops = %d, want 1", all[0].Hops)
	}
}

func TestEndpointsHandlerPruneOld(t *testing.T) {
	h := NewEndpointsHandler()

	endpoints := &message.Endpoints{
		Version: 2,
		EndpointsV2: []message.Endpointv2{
			{Endpoint: "192.168.1.1:51235", Hops: 0},
		},
	}
	h.HandleMessage(context.Background(), PeerID(1), endpoints)

	// Manually set LastSeen to be old
	h.mu.Lock()
	ep := h.endpoints["192.168.1.1:51235"]
	ep.LastSeen = time.Now().Add(-2 * time.Hour)
	h.endpoints["192.168.1.1:51235"] = ep
	h.mu.Unlock()

	removed := h.PruneOld(1 * time.Hour)
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}

	all := h.GetEndpoints()
	if len(all) != 0 {
		t.Errorf("len(endpoints) = %d, want 0", len(all))
	}
}

func TestTransactionHandler(t *testing.T) {
	h := NewTransactionHandler()

	var receivedTx *message.Transaction
	h.OnTransaction = func(ctx context.Context, peerID PeerID, tx *message.Transaction) {
		receivedTx = tx
	}

	tx := &message.Transaction{
		RawTransaction: []byte{1, 2, 3, 4, 5},
		Status:         message.TxStatusNew,
	}

	err := h.HandleMessage(context.Background(), PeerID(1), tx)
	if err != nil {
		t.Errorf("HandleMessage error: %v", err)
	}

	if receivedTx == nil {
		t.Error("OnTransaction callback was not called")
	}
}

func TestTransactionHandlerSeenTracking(t *testing.T) {
	h := NewTransactionHandler()

	hash := "abc123"

	if h.HasSeen(hash) {
		t.Error("Hash should not be seen initially")
	}

	h.MarkSeen(hash)

	if !h.HasSeen(hash) {
		t.Error("Hash should be seen after marking")
	}

	// Mark again - should be idempotent
	h.MarkSeen(hash)
	if !h.HasSeen(hash) {
		t.Error("Hash should still be seen")
	}
}

func TestTransactionHandlerPruneOld(t *testing.T) {
	h := NewTransactionHandler()

	h.MarkSeen("hash1")
	h.MarkSeen("hash2")

	// Manually set old timestamp
	h.mu.Lock()
	h.seen["hash1"] = time.Now().Add(-2 * time.Hour)
	h.mu.Unlock()

	removed := h.PruneOld(1 * time.Hour)
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}

	if h.HasSeen("hash1") {
		t.Error("hash1 should have been pruned")
	}
	if !h.HasSeen("hash2") {
		t.Error("hash2 should still exist")
	}
}

func TestValidationHandler(t *testing.T) {
	h := NewValidationHandler()

	var receivedVal *message.Validation
	h.OnValidation = func(ctx context.Context, peerID PeerID, val *message.Validation) {
		receivedVal = val
	}

	val := &message.Validation{
		Validation: []byte{1, 2, 3, 4, 5},
	}

	err := h.HandleMessage(context.Background(), PeerID(1), val)
	if err != nil {
		t.Errorf("HandleMessage error: %v", err)
	}

	if receivedVal == nil {
		t.Error("OnValidation callback was not called")
	}
}

func TestSquelchHandler(t *testing.T) {
	h := NewSquelchHandler()

	pubKey := []byte{1, 2, 3, 4, 5}

	// Initially not squelched
	if h.IsSquelched(pubKey) {
		t.Error("Should not be squelched initially")
	}

	// Squelch
	squelch := &message.Squelch{
		Squelch:         true,
		ValidatorPubKey: pubKey,
		SquelchDuration: 300, // 5 minutes
	}

	err := h.HandleMessage(context.Background(), PeerID(1), squelch)
	if err != nil {
		t.Errorf("HandleMessage error: %v", err)
	}

	if !h.IsSquelched(pubKey) {
		t.Error("Should be squelched after squelch message")
	}

	// Unsquelch
	unsquelch := &message.Squelch{
		Squelch:         false,
		ValidatorPubKey: pubKey,
	}

	err = h.HandleMessage(context.Background(), PeerID(1), unsquelch)
	if err != nil {
		t.Errorf("HandleMessage error: %v", err)
	}

	if h.IsSquelched(pubKey) {
		t.Error("Should not be squelched after unsquelch message")
	}
}

func TestSquelchHandlerExpiry(t *testing.T) {
	h := NewSquelchHandler()

	pubKey := []byte{1, 2, 3}

	// Squelch with short duration
	squelch := &message.Squelch{
		Squelch:         true,
		ValidatorPubKey: pubKey,
		SquelchDuration: 1, // 1 second
	}
	h.HandleMessage(context.Background(), PeerID(1), squelch)

	// Manually expire
	h.mu.Lock()
	info := h.squelched[string(pubKey)]
	info.ExpiresAt = time.Now().Add(-1 * time.Second)
	h.squelched[string(pubKey)] = info
	h.mu.Unlock()

	// Should no longer be squelched (expired)
	if h.IsSquelched(pubKey) {
		t.Error("Should not be squelched after expiry")
	}
}

func TestSquelchHandlerPruneExpired(t *testing.T) {
	h := NewSquelchHandler()

	squelch := &message.Squelch{
		Squelch:         true,
		ValidatorPubKey: []byte{1},
		SquelchDuration: 300,
	}
	h.HandleMessage(context.Background(), PeerID(1), squelch)

	// Manually expire
	h.mu.Lock()
	info := h.squelched[string([]byte{1})]
	info.ExpiresAt = time.Now().Add(-1 * time.Second)
	h.squelched[string([]byte{1})] = info
	h.mu.Unlock()

	removed := h.PruneExpired()
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
}

func TestStatusChangeHandler(t *testing.T) {
	h := NewStatusChangeHandler()

	var receivedStatus *message.StatusChange
	h.OnStatusChange = func(ctx context.Context, peerID PeerID, status *message.StatusChange) {
		receivedStatus = status
	}

	sc := &message.StatusChange{
		NewStatus:  message.NodeStatusValidating,
		NewEvent:   message.NodeEventAcceptedLedger,
		LedgerSeq:  1000000,
		LedgerHash: []byte{0xAB, 0xCD},
	}

	err := h.HandleMessage(context.Background(), PeerID(1), sc)
	if err != nil {
		t.Errorf("HandleMessage error: %v", err)
	}

	if receivedStatus == nil {
		t.Error("OnStatusChange callback was not called")
	}

	status, exists := h.GetStatus(PeerID(1))
	if !exists {
		t.Error("Status should exist")
	}
	if status.Status != message.NodeStatusValidating {
		t.Errorf("Status = %d, want %d", status.Status, message.NodeStatusValidating)
	}
	if status.LedgerSeq != 1000000 {
		t.Errorf("LedgerSeq = %d, want 1000000", status.LedgerSeq)
	}
}

func TestStatusChangeHandlerGetAllStatuses(t *testing.T) {
	h := NewStatusChangeHandler()

	h.HandleMessage(context.Background(), PeerID(1), &message.StatusChange{NewStatus: message.NodeStatusConnected})
	h.HandleMessage(context.Background(), PeerID(2), &message.StatusChange{NewStatus: message.NodeStatusValidating})

	all := h.GetAllStatuses()
	if len(all) != 2 {
		t.Errorf("len(statuses) = %d, want 2", len(all))
	}

	// Verify it's a copy
	all[PeerID(1)] = PeerStatus{Status: message.NodeStatusShutting}
	original, _ := h.GetStatus(PeerID(1))
	if original.Status == message.NodeStatusShutting {
		t.Error("GetAllStatuses should return a copy")
	}
}

func TestStatusChangeHandlerRemovePeer(t *testing.T) {
	h := NewStatusChangeHandler()

	h.HandleMessage(context.Background(), PeerID(1), &message.StatusChange{NewStatus: message.NodeStatusConnected})

	_, exists := h.GetStatus(PeerID(1))
	if !exists {
		t.Error("Status should exist before remove")
	}

	h.RemovePeer(PeerID(1))

	_, exists = h.GetStatus(PeerID(1))
	if exists {
		t.Error("Status should not exist after remove")
	}
}

func TestManifestsHandler(t *testing.T) {
	h := NewManifestsHandler()

	callbackCount := 0
	h.OnManifest = func(ctx context.Context, peerID PeerID, manifest message.Manifest) {
		callbackCount++
	}

	manifests := &message.Manifests{
		List: []message.Manifest{
			{STObject: []byte{1, 2, 3}},
			{STObject: []byte{4, 5, 6}},
		},
		History: true,
	}

	err := h.HandleMessage(context.Background(), PeerID(1), manifests)
	if err != nil {
		t.Errorf("HandleMessage error: %v", err)
	}

	if callbackCount != 2 {
		t.Errorf("callbackCount = %d, want 2", callbackCount)
	}
}
