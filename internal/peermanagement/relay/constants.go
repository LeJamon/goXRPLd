// Package relay implements the reduce-relay optimization for XRPL peer messages.
// This optimization reduces bandwidth by selecting a subset of peers to relay
// validator messages (validations and proposals) instead of flooding to all peers.
package relay

import "time"

const (
	// MinUnsquelchExpire is the minimum squelch duration.
	MinUnsquelchExpire = 300 * time.Second

	// MaxUnsquelchExpireDefault is the default maximum squelch duration.
	MaxUnsquelchExpireDefault = 600 * time.Second

	// SquelchPerPeer adds this much duration per peer when calculating squelch time.
	SquelchPerPeer = 10 * time.Second

	// MaxUnsquelchExpirePeers is the absolute maximum squelch duration.
	MaxUnsquelchExpirePeers = 3600 * time.Second

	// Idled is the threshold before identifying a peer as idle.
	Idled = 8 * time.Second

	// MinMessageThreshold is the message count to start considering a peer.
	MinMessageThreshold = 19

	// MaxMessageThreshold is the message count threshold for peer selection.
	MaxMessageThreshold = 20

	// MaxSelectedPeers is the maximum number of peers to select as message source.
	MaxSelectedPeers = 5

	// WaitOnBootup is how long to wait after startup before enabling reduce-relay.
	WaitOnBootup = 10 * time.Minute

	// MaxTxQueueSize is the maximum size of the aggregated transaction hashes per peer.
	MaxTxQueueSize = 10000
)
