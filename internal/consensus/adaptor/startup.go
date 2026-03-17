package adaptor

import (
	"fmt"

	"github.com/LeJamon/goXRPLd/config"
	"github.com/LeJamon/goXRPLd/internal/consensus"
	"github.com/LeJamon/goXRPLd/internal/consensus/rcl"
	"github.com/LeJamon/goXRPLd/internal/ledger/service"
	"github.com/LeJamon/goXRPLd/internal/peermanagement"
)

// Components holds all the consensus/networking components created by NewFromConfig.
type Components struct {
	Overlay *peermanagement.Overlay
	Engine  consensus.Engine
	Adaptor *Adaptor
	Router  *Router
}

// NewFromConfig creates and wires all consensus/networking components from the app config.
// Returns nil Components if the node is in standalone mode.
func NewFromConfig(
	appCfg *config.Config,
	ledgerSvc *service.Service,
) (*Components, error) {
	// Build overlay options from app config
	overlayOpts := OverlayOptionsFromConfig(appCfg)

	overlay, err := peermanagement.New(overlayOpts...)
	if err != nil {
		return nil, fmt.Errorf("create overlay: %w", err)
	}

	// Create validator identity (nil if not a validator)
	var identity *ValidatorIdentity
	if appCfg.ValidationSeed != "" {
		identity, err = NewValidatorIdentity(appCfg.ValidationSeed)
		if err != nil {
			return nil, fmt.Errorf("create validator identity: %w", err)
		}
	}

	// Load UNL from config
	validators, err := ParseValidatorKeys(appCfg)
	if err != nil {
		return nil, fmt.Errorf("parse validators: %w", err)
	}

	// Create the sender wrapping the overlay
	sender := NewOverlaySender(overlay)

	// Create the adaptor
	adaptor := New(Config{
		LedgerService: ledgerSvc,
		Sender:        sender,
		Identity:      identity,
		Validators:    validators,
	})

	// Create the RCL consensus engine
	engine := rcl.NewEngine(adaptor, rcl.DefaultConfig())

	// Create the router
	router := NewRouter(engine, adaptor, overlay.Messages())

	return &Components{
		Overlay: overlay,
		Engine:  engine,
		Adaptor: adaptor,
		Router:  router,
	}, nil
}

// OverlayOptionsFromConfig maps app config fields to overlay options.
func OverlayOptionsFromConfig(appCfg *config.Config) []peermanagement.Option {
	var opts []peermanagement.Option

	// Network ID
	if networkID, err := appCfg.GetNetworkID(); err == nil {
		opts = append(opts, peermanagement.WithNetworkID(uint32(networkID)))
	}

	// Listen address from peer port config
	if _, peerPort, hasPeer := appCfg.GetPeerPort(); hasPeer {
		opts = append(opts, peermanagement.WithListenAddr(peerPort.GetBindAddress()))
	}

	// Bootstrap peers
	if len(appCfg.IPs) > 0 {
		opts = append(opts, peermanagement.WithBootstrapPeers(appCfg.IPs...))
	}

	// Fixed peers
	if len(appCfg.IPsFixed) > 0 {
		opts = append(opts, peermanagement.WithFixedPeers(appCfg.IPsFixed...))
	}

	// Max peers
	if appCfg.PeersMax > 0 {
		opts = append(opts, peermanagement.WithMaxPeers(appCfg.PeersMax))
	}

	// Private mode
	if appCfg.PeerPrivate > 0 {
		opts = append(opts, peermanagement.WithPrivateMode(true))
	}

	// Compression
	opts = append(opts, peermanagement.WithCompression(appCfg.Compression))

	return opts
}

// ParseValidatorKeys parses validator public keys from the config into NodeIDs.
func ParseValidatorKeys(appCfg *config.Config) ([]consensus.NodeID, error) {
	if len(appCfg.Validators.Validators) == 0 {
		return nil, nil
	}

	var validators []consensus.NodeID
	for _, key := range appCfg.Validators.Validators {
		nodeID, err := decodeValidatorKey(key)
		if err != nil {
			return nil, fmt.Errorf("invalid validator key %q: %w", key, err)
		}
		validators = append(validators, nodeID)
	}
	return validators, nil
}

// decodeValidatorKey decodes a base58-encoded validator public key (n-prefixed)
// into a consensus.NodeID (33 bytes).
func decodeValidatorKey(key string) (consensus.NodeID, error) {
	// Validator keys start with 'n' and are base58-encoded compressed public keys.
	// For now, we store the raw string bytes as a placeholder.
	// Full implementation would use addresscodec to decode the node public key.
	// TODO: decode base58 node public key to 33-byte compressed key
	var nodeID consensus.NodeID
	if len(key) < 33 {
		return nodeID, fmt.Errorf("key too short: %d", len(key))
	}
	copy(nodeID[:], []byte(key)[:33])
	return nodeID, nil
}
