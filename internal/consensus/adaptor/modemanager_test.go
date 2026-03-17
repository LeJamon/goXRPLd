package adaptor

import (
	"testing"

	"github.com/LeJamon/goXRPLd/internal/consensus"
	"github.com/stretchr/testify/assert"
)

func newTestModeManager(t *testing.T) *ModeManager {
	t.Helper()
	a := newTestAdaptor(t)
	return NewModeManager(a)
}

func TestModeManagerInitialState(t *testing.T) {
	mm := newTestModeManager(t)
	assert.Equal(t, consensus.OpModeDisconnected, mm.Mode())
}

func TestModeManagerPeerConnected(t *testing.T) {
	mm := newTestModeManager(t)

	mm.OnPeerConnected()
	assert.Equal(t, consensus.OpModeConnected, mm.Mode())
}

func TestModeManagerAllPeersDisconnected(t *testing.T) {
	mm := newTestModeManager(t)

	mm.OnPeerConnected()
	mm.OnPeerConnected()
	assert.Equal(t, consensus.OpModeConnected, mm.Mode())

	mm.OnPeerDisconnected()
	assert.Equal(t, consensus.OpModeConnected, mm.Mode()) // still 1 peer

	mm.OnPeerDisconnected()
	assert.Equal(t, consensus.OpModeDisconnected, mm.Mode()) // 0 peers
}

func TestModeManagerFullTransitionPath(t *testing.T) {
	mm := newTestModeManager(t)

	// Disconnected → Connected
	mm.OnPeerConnected()
	assert.Equal(t, consensus.OpModeConnected, mm.Mode())

	// Connected → Syncing
	mm.OnLCLMismatch()
	assert.Equal(t, consensus.OpModeSyncing, mm.Mode())

	// Syncing → Tracking
	mm.OnLCLAcquired()
	assert.Equal(t, consensus.OpModeTracking, mm.Mode())

	// Tracking → Full
	mm.OnValidationsReceived()
	assert.Equal(t, consensus.OpModeFull, mm.Mode())
}

func TestModeManagerWrongLedgerRecovery(t *testing.T) {
	mm := newTestModeManager(t)

	// Get to Full mode
	mm.OnPeerConnected()
	mm.OnLCLMismatch()
	mm.OnLCLAcquired()
	mm.OnValidationsReceived()
	assert.Equal(t, consensus.OpModeFull, mm.Mode())

	// Full → Syncing (wrong ledger)
	mm.OnWrongLedger()
	assert.Equal(t, consensus.OpModeSyncing, mm.Mode())

	// Recover: Syncing → Tracking → Full
	mm.OnLCLAcquired()
	assert.Equal(t, consensus.OpModeTracking, mm.Mode())
	mm.OnValidationsReceived()
	assert.Equal(t, consensus.OpModeFull, mm.Mode())
}

func TestModeManagerDisconnectFromAnyState(t *testing.T) {
	mm := newTestModeManager(t)

	// Get to Full mode
	mm.OnPeerConnected()
	mm.OnLCLMismatch()
	mm.OnLCLAcquired()
	mm.OnValidationsReceived()
	assert.Equal(t, consensus.OpModeFull, mm.Mode())

	// Losing all peers → Disconnected
	mm.OnPeerDisconnected()
	assert.Equal(t, consensus.OpModeDisconnected, mm.Mode())
}

func TestModeManagerNoopTransitions(t *testing.T) {
	mm := newTestModeManager(t)

	// These should be no-ops in Disconnected state
	mm.OnLCLMismatch()
	assert.Equal(t, consensus.OpModeDisconnected, mm.Mode())

	mm.OnLCLAcquired()
	assert.Equal(t, consensus.OpModeDisconnected, mm.Mode())

	mm.OnValidationsReceived()
	assert.Equal(t, consensus.OpModeDisconnected, mm.Mode())
}

func TestModeManagerCallback(t *testing.T) {
	mm := newTestModeManager(t)

	var transitions []struct {
		from, to consensus.OperatingMode
	}

	mm.SetOnModeChange(func(old, new consensus.OperatingMode) {
		transitions = append(transitions, struct {
			from, to consensus.OperatingMode
		}{old, new})
	})

	mm.OnPeerConnected()
	mm.OnLCLMismatch()

	assert.Len(t, transitions, 2)
	assert.Equal(t, consensus.OpModeDisconnected, transitions[0].from)
	assert.Equal(t, consensus.OpModeConnected, transitions[0].to)
	assert.Equal(t, consensus.OpModeConnected, transitions[1].from)
	assert.Equal(t, consensus.OpModeSyncing, transitions[1].to)
}

func TestModeManagerPeerCountUnderflow(t *testing.T) {
	mm := newTestModeManager(t)

	// Disconnecting with 0 peers should not underflow
	mm.OnPeerDisconnected()
	assert.Equal(t, consensus.OpModeDisconnected, mm.Mode())
}

func TestModeManagerForceSetMode(t *testing.T) {
	mm := newTestModeManager(t)

	mm.SetMode(consensus.OpModeFull)
	assert.Equal(t, consensus.OpModeFull, mm.Mode())
}
