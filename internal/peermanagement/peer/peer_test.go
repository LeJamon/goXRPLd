package peer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LeJamon/goXRPLd/internal/peermanagement/identity"
)

// TestNew tests creating a new peer
func TestNew(t *testing.T) {
	id, err := identity.NewIdentity()
	require.NoError(t, err)

	cfg := DefaultConfig()
	p := New(id, cfg)

	require.NotNil(t, p)
	assert.Equal(t, StateDisconnected, p.State())
	assert.Empty(t, p.Address())
	assert.Nil(t, p.RemotePublicKey())
	assert.Empty(t, p.RemoteVersion())
}

// TestDefaultConfig tests the default configuration
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, ConnectTimeout, cfg.Timeout)
	assert.NotNil(t, cfg.TLSConfig)
	assert.True(t, cfg.TLSConfig.InsecureSkipVerify) // XRPL uses self-signed certs
}

// TestState_String tests the State String method
func TestState_String(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateDisconnected, "disconnected"},
		{StateConnecting, "connecting"},
		{StateConnected, "connected"},
		{StateClosing, "closing"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.String())
		})
	}
}

// TestNormalizeAddress tests address normalization
func TestNormalizeAddress(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"localhost", "localhost:51235"},
		{"192.168.1.1", "192.168.1.1:51235"},
		{"example.com", "example.com:51235"},
		{"localhost:8080", "localhost:8080"},
		{"192.168.1.1:6006", "192.168.1.1:6006"},
		{"[::1]", "[::1]:51235"},
		{"[::1]:9999", "[::1]:9999"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeAddress(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestPeer_Close tests closing a disconnected peer
func TestPeer_Close(t *testing.T) {
	id, err := identity.NewIdentity()
	require.NoError(t, err)

	p := New(id, DefaultConfig())

	// Close should succeed even when disconnected
	err = p.Close()
	assert.NoError(t, err)
	assert.Equal(t, StateDisconnected, p.State())

	// Multiple closes should be safe
	err = p.Close()
	assert.NoError(t, err)
}

// TestPeer_ConnectAlreadyConnecting tests connecting when already connecting
func TestPeer_ConnectAlreadyConnecting(t *testing.T) {
	id, err := identity.NewIdentity()
	require.NoError(t, err)

	p := New(id, DefaultConfig())

	// Manually set state to connecting
	p.setState(StateConnecting)

	// Try to connect should fail
	ctx := context.Background()
	err = p.Connect(ctx, "localhost:51235")
	assert.Error(t, err)
}

// TestPeer_ReadWriteClosed tests read/write on closed connection
func TestPeer_ReadWriteClosed(t *testing.T) {
	id, err := identity.NewIdentity()
	require.NoError(t, err)

	p := New(id, DefaultConfig())

	// Read on disconnected peer
	buf := make([]byte, 100)
	_, err = p.Read(buf)
	assert.ErrorIs(t, err, ErrClosed)

	// Write on disconnected peer
	_, err = p.Write([]byte("test"))
	assert.ErrorIs(t, err, ErrClosed)
}

// TestPeer_StateTransitions tests state transitions
func TestPeer_StateTransitions(t *testing.T) {
	id, err := identity.NewIdentity()
	require.NoError(t, err)

	p := New(id, DefaultConfig())

	// Initial state
	assert.Equal(t, StateDisconnected, p.State())

	// Simulate state transitions
	p.setState(StateConnecting)
	assert.Equal(t, StateConnecting, p.State())

	p.setState(StateConnected)
	assert.Equal(t, StateConnected, p.State())

	p.setState(StateClosing)
	assert.Equal(t, StateClosing, p.State())

	p.setState(StateDisconnected)
	assert.Equal(t, StateDisconnected, p.State())
}

// TestPeer_ConnectInvalidAddress tests connecting to invalid addresses
// Note: This test attempts actual connections, so it may be slow
func TestPeer_ConnectInvalidAddress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping connection test in short mode")
	}

	id, err := identity.NewIdentity()
	require.NoError(t, err)

	cfg := DefaultConfig()
	cfg.Timeout = 1 * time.Second // Short timeout for test

	p := New(id, cfg)

	// Try to connect to non-routable address
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = p.Connect(ctx, "192.0.2.1:51235") // TEST-NET-1, should fail
	assert.Error(t, err)
	assert.Equal(t, StateDisconnected, p.State())
}

// TestPeer_Accessors tests getter methods
func TestPeer_Accessors(t *testing.T) {
	id, err := identity.NewIdentity()
	require.NoError(t, err)

	p := New(id, DefaultConfig())
	p.address = "test.example.com:51235"

	assert.Equal(t, "test.example.com:51235", p.Address())
	assert.Nil(t, p.Connection())
}

// TestConstants tests that constants have expected values
func TestConstants(t *testing.T) {
	assert.Equal(t, 51235, DefaultPort)
	assert.Equal(t, 30*time.Second, ConnectTimeout)
	assert.Equal(t, 10*time.Second, HandshakeTimeout)
}
