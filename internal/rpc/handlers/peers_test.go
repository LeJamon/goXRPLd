package handlers_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LeJamon/goXRPLd/internal/rpc/handlers"
	"github.com/LeJamon/goXRPLd/internal/rpc/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePeerSource struct {
	peers []map[string]any
}

func (f *fakePeerSource) PeersJSON() []map[string]any { return f.peers }

func TestPeersMethod_NilSourceReturnsEmptyList(t *testing.T) {
	m := &handlers.PeersMethod{}
	ctx := &types.RpcContext{Context: context.Background(), Role: types.RoleAdmin, IsAdmin: true}

	result, rpcErr := m.Handle(ctx, json.RawMessage(`{}`))
	require.Nil(t, rpcErr)

	resp, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []map[string]any{}, resp["peers"])
	assert.Equal(t, map[string]any{}, resp["cluster"],
		"rippled doPeers (Peers.cpp:59) always emits a cluster object")
}

func TestPeersMethod_PassesThroughSource(t *testing.T) {
	src := &fakePeerSource{peers: []map[string]any{
		{
			"address":         "192.0.2.1:51235",
			"public_key":      "nHB1...",
			"server_domain":   "validator.example.com",
			"ledger":          "ABCD",
			"previous_ledger": "0123",
			"remote_ip":       "203.0.113.7",
			"local_ip":        "198.51.100.42",
		},
	}}
	m := &handlers.PeersMethod{}
	ctx := &types.RpcContext{
		Context:    context.Background(),
		Role:       types.RoleAdmin,
		IsAdmin:    true,
		PeerSource: src,
	}

	result, rpcErr := m.Handle(ctx, json.RawMessage(`{}`))
	require.Nil(t, rpcErr)

	resp, ok := result.(map[string]any)
	require.True(t, ok)
	peers, ok := resp["peers"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, peers, 1)
	assert.Equal(t, "validator.example.com", peers[0]["server_domain"])
	assert.Equal(t, "ABCD", peers[0]["ledger"])
	assert.NotContains(t, peers[0], "closed_ledger", "rippled uses 'ledger' for the closed-ledger hash")
	assert.NotContains(t, peers[0], "inbound", "inbound is only emitted when true")
	assert.Equal(t, "203.0.113.7", peers[0]["remote_ip"])
	assert.Equal(t, map[string]any{}, resp["cluster"],
		"rippled doPeers (Peers.cpp:59) always emits a cluster object")
}
