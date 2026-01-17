// Package feature implements protocol feature negotiation for XRPL peers.
// Features are capabilities that peers can advertise and negotiate during handshake.
package feature

import (
	"strings"
	"sync"
)

// Feature represents a protocol feature.
type Feature int

const (
	// FeatureValidatorListPropagation enables validator list propagation.
	FeatureValidatorListPropagation Feature = iota
	// FeatureLedgerReplay enables ledger replay support.
	FeatureLedgerReplay
	// FeatureCompression enables message compression.
	FeatureCompression
	// FeatureReduceRelay enables reduce-relay optimization.
	FeatureReduceRelay
	// FeatureTransactionBatching enables transaction batching.
	FeatureTransactionBatching
)

// String returns the string representation of a feature.
func (f Feature) String() string {
	switch f {
	case FeatureValidatorListPropagation:
		return "validatorListPropagation"
	case FeatureLedgerReplay:
		return "ledgerReplay"
	case FeatureCompression:
		return "compression"
	case FeatureReduceRelay:
		return "reduceRelay"
	case FeatureTransactionBatching:
		return "transactionBatching"
	default:
		return "unknown"
	}
}

// ParseFeature parses a feature string.
func ParseFeature(s string) (Feature, bool) {
	switch strings.ToLower(s) {
	case "validatorlistpropagation":
		return FeatureValidatorListPropagation, true
	case "ledgerreplay":
		return FeatureLedgerReplay, true
	case "compression":
		return FeatureCompression, true
	case "reducerelay":
		return FeatureReduceRelay, true
	case "transactionbatching":
		return FeatureTransactionBatching, true
	default:
		return 0, false
	}
}

// ProtocolVersion represents a protocol version.
type ProtocolVersion struct {
	Major int
	Minor int
}

// String returns the string representation.
func (pv ProtocolVersion) String() string {
	return strings.Join([]string{
		"XRPL",
		string(rune('0' + pv.Major)),
		string(rune('0' + pv.Minor)),
	}, "/")
}

// ParseProtocolVersion parses a protocol version string.
// Accepts formats like "XRPL/2.2" or "RTXP/1.2"
func ParseProtocolVersion(s string) (ProtocolVersion, bool) {
	s = strings.TrimSpace(s)

	// Try XRPL/x.y format
	if strings.HasPrefix(s, "XRPL/") || strings.HasPrefix(s, "RTXP/") {
		parts := strings.Split(s, "/")
		if len(parts) >= 2 {
			version := parts[1]
			vparts := strings.Split(version, ".")
			if len(vparts) >= 2 {
				var major, minor int
				if _, err := stringToInt(vparts[0], &major); err == nil {
					if _, err := stringToInt(vparts[1], &minor); err == nil {
						return ProtocolVersion{Major: major, Minor: minor}, true
					}
				}
			}
		}
	}

	return ProtocolVersion{}, false
}

// stringToInt is a simple string to int conversion.
func stringToInt(s string, result *int) (bool, error) {
	*result = 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return false, nil
		}
		*result = *result*10 + int(c-'0')
	}
	return true, nil
}

// CurrentVersion is the current protocol version we support.
var CurrentVersion = ProtocolVersion{Major: 2, Minor: 2}

// MinVersion is the minimum protocol version we support.
var MinVersion = ProtocolVersion{Major: 2, Minor: 0}

// FeatureSet represents a set of supported features.
type FeatureSet struct {
	mu       sync.RWMutex
	features map[Feature]bool
}

// NewFeatureSet creates a new feature set.
func NewFeatureSet() *FeatureSet {
	return &FeatureSet{
		features: make(map[Feature]bool),
	}
}

// DefaultFeatureSet returns the default set of features we support.
func DefaultFeatureSet() *FeatureSet {
	fs := NewFeatureSet()
	fs.Enable(FeatureCompression)
	fs.Enable(FeatureReduceRelay)
	fs.Enable(FeatureValidatorListPropagation)
	return fs
}

// Enable enables a feature.
func (fs *FeatureSet) Enable(f Feature) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.features[f] = true
}

// Disable disables a feature.
func (fs *FeatureSet) Disable(f Feature) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	delete(fs.features, f)
}

// Has returns true if the feature is enabled.
func (fs *FeatureSet) Has(f Feature) bool {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return fs.features[f]
}

// List returns all enabled features.
func (fs *FeatureSet) List() []Feature {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	var result []Feature
	for f := range fs.features {
		result = append(result, f)
	}
	return result
}

// Intersect returns features that are in both sets.
func (fs *FeatureSet) Intersect(other *FeatureSet) *FeatureSet {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	other.mu.RLock()
	defer other.mu.RUnlock()

	result := NewFeatureSet()
	for f := range fs.features {
		if other.features[f] {
			result.features[f] = true
		}
	}
	return result
}

// String returns a comma-separated list of features.
func (fs *FeatureSet) String() string {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	var names []string
	for f := range fs.features {
		names = append(names, f.String())
	}
	return strings.Join(names, ",")
}

// PeerCapabilities represents the negotiated capabilities of a peer.
type PeerCapabilities struct {
	mu sync.RWMutex

	// Protocol version
	Version ProtocolVersion

	// Supported features
	Features *FeatureSet

	// Compression algorithm
	CompressionAlgorithm string

	// Network ID
	NetworkID uint32

	// Listening port
	ListeningPort uint16

	// Whether the peer supports crawling
	SupportsCrawl bool

	// Whether the peer is a validator
	IsValidator bool
}

// NewPeerCapabilities creates new peer capabilities.
func NewPeerCapabilities() *PeerCapabilities {
	return &PeerCapabilities{
		Features: NewFeatureSet(),
	}
}

// HasFeature returns true if the peer has a feature.
func (pc *PeerCapabilities) HasFeature(f Feature) bool {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.Features.Has(f)
}

// SupportsCompression returns true if the peer supports compression.
func (pc *PeerCapabilities) SupportsCompression() bool {
	return pc.HasFeature(FeatureCompression)
}

// SupportsReduceRelay returns true if the peer supports reduce-relay.
func (pc *PeerCapabilities) SupportsReduceRelay() bool {
	return pc.HasFeature(FeatureReduceRelay)
}

// SupportsLedgerReplay returns true if the peer supports ledger replay.
func (pc *PeerCapabilities) SupportsLedgerReplay() bool {
	return pc.HasFeature(FeatureLedgerReplay)
}

// Negotiator handles feature negotiation during handshake.
type Negotiator struct {
	localFeatures *FeatureSet
	localVersion  ProtocolVersion
}

// NewNegotiator creates a new negotiator with our supported features.
func NewNegotiator() *Negotiator {
	return &Negotiator{
		localFeatures: DefaultFeatureSet(),
		localVersion:  CurrentVersion,
	}
}

// Negotiate performs feature negotiation with a peer.
// Returns the negotiated capabilities.
func (n *Negotiator) Negotiate(peerVersion ProtocolVersion, peerFeatures *FeatureSet) (*PeerCapabilities, error) {
	caps := NewPeerCapabilities()

	// Use the lower version
	caps.Version = peerVersion
	if n.localVersion.Major < peerVersion.Major ||
		(n.localVersion.Major == peerVersion.Major && n.localVersion.Minor < peerVersion.Minor) {
		caps.Version = n.localVersion
	}

	// Intersect features
	caps.Features = n.localFeatures.Intersect(peerFeatures)

	return caps, nil
}

// LocalFeatures returns our local feature set.
func (n *Negotiator) LocalFeatures() *FeatureSet {
	return n.localFeatures
}

// LocalVersion returns our local protocol version.
func (n *Negotiator) LocalVersion() ProtocolVersion {
	return n.localVersion
}
