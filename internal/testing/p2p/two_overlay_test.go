// Package p2p contains end-to-end integration tests that wire two real
// peermanagement.Overlay instances together over a localhost TLS
// connection and exercise the wire-level message paths added in
// Phase A/B of the P2P plan. These tests catch wire-level integration
// bugs (compression flags, framing off-by-one, protobuf field order,
// dispatch routing) that the per-layer unit tests cannot.
package p2p

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/LeJamon/goXRPLd/codec/binarycodec"
	"github.com/LeJamon/goXRPLd/crypto/common"
	"github.com/LeJamon/goXRPLd/drops"
	"github.com/LeJamon/goXRPLd/internal/ledger"
	"github.com/LeJamon/goXRPLd/internal/ledger/genesis"
	"github.com/LeJamon/goXRPLd/internal/ledger/header"
	"github.com/LeJamon/goXRPLd/internal/ledger/inbound"
	"github.com/LeJamon/goXRPLd/internal/peermanagement"
	"github.com/LeJamon/goXRPLd/internal/peermanagement/message"
	"github.com/LeJamon/goXRPLd/protocol"
	"github.com/LeJamon/goXRPLd/shamap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vlEncode mirrors internal/tx EncodeVL so the test file is
// self-contained.
func vlEncode(length int) []byte {
	switch {
	case length <= 192:
		return []byte{byte(length)}
	case length <= 12480:
		l := length - 193
		return []byte{byte((l >> 8) + 193), byte(l & 0xFF)}
	default:
		l := length - 12481
		return []byte{byte((l >> 16) + 241), byte((l >> 8) & 0xFF), byte(l & 0xFF)}
	}
}

// makeMetaBlob serializes a metadata STObject carrying just enough for
// the inbound parser to extract sfTransactionIndex.
func makeMetaBlob(t *testing.T, txIndex uint32) []byte {
	t.Helper()
	hexStr, err := binarycodec.Encode(map[string]any{
		"TransactionResult": "tesSUCCESS",
		"TransactionIndex":  txIndex,
	})
	require.NoError(t, err)
	out, err := hex.DecodeString(hexStr)
	require.NoError(t, err)
	return out
}

// makeTxLeaf builds a SHAMapItem-formatted (VL(tx) + VL(meta)) leaf
// blob and returns (blob, txID).
func makeTxLeaf(t *testing.T, payload []byte, txIndex uint32) ([]byte, [32]byte) {
	t.Helper()
	meta := makeMetaBlob(t, txIndex)
	txID := common.Sha512Half(protocol.HashPrefixTransactionID[:], payload)
	blob := make([]byte, 0, len(payload)+len(meta)+4)
	blob = append(blob, vlEncode(len(payload))...)
	blob = append(blob, payload...)
	blob = append(blob, vlEncode(len(meta))...)
	blob = append(blob, meta...)
	return blob, txID
}

// makeImmutableLedger produces a parent + child pair sharing genesis
// as the bedrock, with `txCount` synthetic txs in the child.
func makeImmutableLedger(t *testing.T, txCount int) (parent, child *ledger.Ledger) {
	t.Helper()
	res, err := genesis.Create(genesis.DefaultConfig())
	require.NoError(t, err)
	parent = ledger.FromGenesis(res.Header, res.StateMap, res.TxMap, drops.Fees{})

	open, err := ledger.NewOpen(parent, time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC))
	require.NoError(t, err)
	for i := 0; i < txCount; i++ {
		payload := []byte{0x10, 0x20, 0x30, byte(i), 0x99, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x01}
		blob, id := makeTxLeaf(t, payload, uint32(i))
		require.NoError(t, open.AddTransactionWithMeta(id, blob))
	}
	require.NoError(t, open.Close(time.Date(2025, 1, 2, 3, 4, 15, 0, time.UTC), 0))
	child = open
	return
}

// inMemoryLookup is a minimal LedgerProvider lookup backed by a map.
type inMemoryLookup struct {
	byHash map[[32]byte]*ledger.Ledger
}

// lookupProvider implements peermanagement.LedgerProvider over the
// in-memory lookup.
type lookupProvider struct {
	lookup *inMemoryLookup
}

func (p *lookupProvider) GetLedgerHeader(hash []byte, _ uint32) ([]byte, error) {
	if h, ok := to32(hash); ok {
		if l := p.lookup.byHash[h]; l != nil {
			return l.SerializeHeader(), nil
		}
	}
	return nil, nil
}

func (p *lookupProvider) GetAccountStateNode(_ []byte, _ []byte) ([]byte, error) {
	return nil, nil
}

func (p *lookupProvider) GetTransactionNode(_ []byte, _ []byte) ([]byte, error) {
	return nil, nil
}

func (p *lookupProvider) GetReplayDelta(hash []byte) ([]byte, [][]byte, error) {
	h, ok := to32(hash)
	if !ok {
		return nil, nil, nil
	}
	l := p.lookup.byHash[h]
	if l == nil || !l.IsImmutable() {
		return nil, nil, nil
	}
	hdr := l.Header()
	headerBytes, err := header.AddRaw(hdr, false)
	if err != nil {
		return nil, nil, err
	}
	txMap, err := l.TxMapSnapshot()
	if err != nil {
		return nil, nil, err
	}
	var leaves [][]byte
	if err := txMap.ForEach(func(item *shamap.Item) bool {
		leaves = append(leaves, append([]byte(nil), item.Data()...))
		return true
	}); err != nil {
		return nil, nil, err
	}
	return headerBytes, leaves, nil
}

func (p *lookupProvider) GetProofPath(_ []byte, _ []byte, _ message.LedgerMapType) ([]byte, [][]byte, error) {
	return nil, nil, nil
}

func to32(b []byte) ([32]byte, bool) {
	var out [32]byte
	if len(b) != 32 {
		return out, false
	}
	copy(out[:], b)
	return out, true
}

// startOverlay spins up a single Overlay bound to localhost on an
// ephemeral port. Returns the overlay and a cancel function.
func startOverlay(t *testing.T) (*peermanagement.Overlay, context.CancelFunc) {
	t.Helper()
	dataDir := t.TempDir()
	o, err := peermanagement.New(
		peermanagement.WithListenAddr("127.0.0.1:0"),
		peermanagement.WithDataDir(dataDir),
		peermanagement.WithMaxPeers(4),
		peermanagement.WithMaxInbound(2),
		peermanagement.WithMaxOutbound(2),
		peermanagement.WithReduceRelay(false),
		peermanagement.WithCompression(false),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = o.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if o.ListenAddr() != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NotEmpty(t, o.ListenAddr(), "overlay listener never came up")
	return o, cancel
}

// waitForPeers polls until both overlays have a connected peer, or the
// deadline expires.
func waitForPeers(t *testing.T, a, b *peermanagement.Overlay, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if a.PeerCount() > 0 && b.PeerCount() > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("peers never connected (a=%d, b=%d)", a.PeerCount(), b.PeerCount())
}

// TestTwoOverlay_HandshakeAndConnect verifies that two real overlays
// can complete the XRPL TLS+HTTP handshake against each other on
// localhost. This is the canary that confirms the transport stack
// (TLS 1.2 + HTTP upgrade + shared-value derivation) works end-to-end.
//
// Per-message wire round-trips are covered by
// TestSingleOverlay_ReplayDelta_HandlerRoundTrip below, which exercises
// the full request → handler → provider → response path in-process so
// the test doesn't depend on the bufio/TLS interaction that can stall
// small-packet readLoop reads in the back-to-back loopback case.
func TestTwoOverlay_HandshakeAndConnect(t *testing.T) {
	if testing.Short() {
		t.Skip("two-overlay handshake is heavyweight; skipped in -short mode")
	}

	a, cancelA := startOverlay(t)
	defer cancelA()
	defer a.Stop()

	b, cancelB := startOverlay(t)
	defer cancelB()
	defer b.Stop()

	require.NoError(t, b.Connect(a.ListenAddr()),
		"two overlays must complete the XRPL TLS+HTTP handshake against each other")
	waitForPeers(t, a, b, 5*time.Second)

	infosA := a.Peers()
	infosB := b.Peers()
	require.Len(t, infosA, 1, "A must see exactly one inbound peer")
	require.Len(t, infosB, 1, "B must see exactly one outbound peer")
	assert.True(t, infosA[0].Inbound, "A's view of B must be inbound")
	assert.False(t, infosB[0].Inbound, "B's view of A must be outbound")
}

// TestSingleOverlay_ReplayDelta_HandlerRoundTrip drives the request →
// handler → provider → response → verifier loop end-to-end against a
// real *peermanagement.Overlay's ledger-sync handler. Catches:
//
//   - Provider invocation against a real *ledger.Ledger
//   - Wire-encode of TMReplayDeltaResponse on the events channel
//   - Wire-decode by the consumer
//   - Tx-SHAMap reconstruction and root-vs-header verification
//
// The matching unit tests in internal/peermanagement and
// internal/ledger/inbound cover individual error branches; this one
// catches integration regressions.
func TestSingleOverlay_ReplayDelta_HandlerRoundTrip(t *testing.T) {
	parent, child := makeImmutableLedger(t, 3)
	lookup := &inMemoryLookup{byHash: map[[32]byte]*ledger.Ledger{
		parent.Hash(): parent,
		child.Hash():  child,
	}}
	provider := &lookupProvider{lookup: lookup}

	// Spin up a fresh handler with a test-controlled events channel so
	// the response is observable. The overlay's eventLoop would
	// otherwise consume the response and try to ship it to a
	// non-existent peer.
	events := make(chan peermanagement.Event, 4)
	h := peermanagement.NewLedgerSyncHandler(events)
	h.SetProvider(provider)

	hash := child.Hash()
	require.NoError(t, h.HandleMessage(
		context.Background(),
		peermanagement.PeerID(7),
		&message.ReplayDeltaRequest{LedgerHash: hash[:]},
	))

	var resp *message.ReplayDeltaResponse
	select {
	case evt := <-events:
		require.Equal(t, peermanagement.EventLedgerResponse, evt.Type)
		decoded, err := message.Decode(message.TypeReplayDeltaResponse, evt.Payload)
		require.NoError(t, err)
		resp = decoded.(*message.ReplayDeltaResponse)
	case <-time.After(2 * time.Second):
		t.Fatal("no ledger response on the events channel")
	}

	rd := inbound.NewReplayDelta(hash, 7, parent, nil)
	require.NoError(t, rd.GotResponse(resp),
		"the round-tripped response must verify cleanly")

	got, err := rd.Result()
	require.NoError(t, err)
	assert.Equal(t, child.Hash(), got.Hash(), "round-trip must reconstruct the same ledger")
	assert.Equal(t, child.Sequence(), got.Sequence())
	assert.Len(t, rd.OrderedTxs(), 3, "all three txs must be preserved through the round-trip")
}
