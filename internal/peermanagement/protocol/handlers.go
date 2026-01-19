// Package protocol implements concrete message handlers for the XRPL peer protocol.
package protocol

import (
	"context"
	"sync"
	"time"

	"github.com/LeJamon/goXRPLd/internal/peermanagement/message"
)

// PingHandler handles ping/pong messages for keepalive.
type PingHandler struct {
	mu       sync.RWMutex
	lastPing map[PeerID]time.Time
	latency  map[PeerID]time.Duration

	// OnPing is called when a ping is received
	OnPing func(ctx context.Context, peerID PeerID, ping *message.Ping)
}

// NewPingHandler creates a new PingHandler.
func NewPingHandler() *PingHandler {
	return &PingHandler{
		lastPing: make(map[PeerID]time.Time),
		latency:  make(map[PeerID]time.Duration),
	}
}

// HandleMessage handles ping/pong messages.
func (h *PingHandler) HandleMessage(ctx context.Context, peerID PeerID, msg message.Message) error {
	ping, ok := msg.(*message.Ping)
	if !ok {
		return nil
	}

	h.mu.Lock()
	if ping.PType == message.PingTypePing {
		h.lastPing[peerID] = time.Now()
	} else if ping.PType == message.PingTypePong && ping.PingTime > 0 {
		// Calculate latency
		sentTime := time.UnixMilli(int64(ping.PingTime))
		h.latency[peerID] = time.Since(sentTime)
	}
	h.mu.Unlock()

	if h.OnPing != nil {
		h.OnPing(ctx, peerID, ping)
	}

	return nil
}

// GetLatency returns the last measured latency for a peer.
func (h *PingHandler) GetLatency(peerID PeerID) time.Duration {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.latency[peerID]
}

// GetLastPing returns the time of the last ping from a peer.
func (h *PingHandler) GetLastPing(peerID PeerID) time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastPing[peerID]
}

// ManifestsHandler handles validator manifest messages.
type ManifestsHandler struct {
	mu        sync.RWMutex
	manifests map[string][]byte // keyed by public key

	// OnManifest is called when a manifest is received
	OnManifest func(ctx context.Context, peerID PeerID, manifest message.Manifest)
}

// NewManifestsHandler creates a new ManifestsHandler.
func NewManifestsHandler() *ManifestsHandler {
	return &ManifestsHandler{
		manifests: make(map[string][]byte),
	}
}

// HandleMessage handles manifest messages.
func (h *ManifestsHandler) HandleMessage(ctx context.Context, peerID PeerID, msg message.Message) error {
	manifests, ok := msg.(*message.Manifests)
	if !ok {
		return nil
	}

	for _, manifest := range manifests.List {
		// Store manifest
		if len(manifest.STObject) > 0 {
			h.mu.Lock()
			// In a full implementation, we would parse the STObject to get the public key
			h.mu.Unlock()

			if h.OnManifest != nil {
				h.OnManifest(ctx, peerID, manifest)
			}
		}
	}

	return nil
}

// EndpointsHandler handles peer endpoint messages for discovery.
type EndpointsHandler struct {
	mu        sync.RWMutex
	endpoints map[string]EndpointInfo // keyed by address

	// OnEndpoint is called when a new endpoint is discovered
	OnEndpoint func(ctx context.Context, peerID PeerID, endpoint message.Endpointv2)
}

// EndpointInfo stores information about a discovered endpoint.
type EndpointInfo struct {
	Address   string
	Hops      uint32
	LastSeen  time.Time
	SourceID  PeerID
}

// NewEndpointsHandler creates a new EndpointsHandler.
func NewEndpointsHandler() *EndpointsHandler {
	return &EndpointsHandler{
		endpoints: make(map[string]EndpointInfo),
	}
}

// HandleMessage handles endpoint messages.
func (h *EndpointsHandler) HandleMessage(ctx context.Context, peerID PeerID, msg message.Message) error {
	endpoints, ok := msg.(*message.Endpoints)
	if !ok {
		return nil
	}

	now := time.Now()
	for _, ep := range endpoints.EndpointsV2 {
		h.mu.Lock()
		existing, exists := h.endpoints[ep.Endpoint]
		if !exists || ep.Hops < existing.Hops {
			h.endpoints[ep.Endpoint] = EndpointInfo{
				Address:  ep.Endpoint,
				Hops:     ep.Hops,
				LastSeen: now,
				SourceID: peerID,
			}
		} else {
			existing.LastSeen = now
			h.endpoints[ep.Endpoint] = existing
		}
		h.mu.Unlock()

		if h.OnEndpoint != nil {
			h.OnEndpoint(ctx, peerID, ep)
		}
	}

	return nil
}

// GetEndpoints returns all known endpoints.
func (h *EndpointsHandler) GetEndpoints() []EndpointInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]EndpointInfo, 0, len(h.endpoints))
	for _, ep := range h.endpoints {
		result = append(result, ep)
	}
	return result
}

// PruneOld removes endpoints not seen within the given duration.
func (h *EndpointsHandler) PruneOld(maxAge time.Duration) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for addr, ep := range h.endpoints {
		if ep.LastSeen.Before(cutoff) {
			delete(h.endpoints, addr)
			removed++
		}
	}
	return removed
}

// TransactionHandler handles transaction relay messages.
type TransactionHandler struct {
	mu   sync.RWMutex
	seen map[string]time.Time // hash -> first seen time

	// OnTransaction is called when a transaction is received
	OnTransaction func(ctx context.Context, peerID PeerID, tx *message.Transaction)
}

// NewTransactionHandler creates a new TransactionHandler.
func NewTransactionHandler() *TransactionHandler {
	return &TransactionHandler{
		seen: make(map[string]time.Time),
	}
}

// HandleMessage handles transaction messages.
func (h *TransactionHandler) HandleMessage(ctx context.Context, peerID PeerID, msg message.Message) error {
	tx, ok := msg.(*message.Transaction)
	if !ok {
		return nil
	}

	if h.OnTransaction != nil {
		h.OnTransaction(ctx, peerID, tx)
	}

	return nil
}

// MarkSeen marks a transaction hash as seen.
func (h *TransactionHandler) MarkSeen(hash string) {
	h.mu.Lock()
	if _, exists := h.seen[hash]; !exists {
		h.seen[hash] = time.Now()
	}
	h.mu.Unlock()
}

// HasSeen returns true if the transaction hash has been seen.
func (h *TransactionHandler) HasSeen(hash string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, exists := h.seen[hash]
	return exists
}

// PruneOld removes seen transactions older than the given duration.
func (h *TransactionHandler) PruneOld(maxAge time.Duration) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for hash, seen := range h.seen {
		if seen.Before(cutoff) {
			delete(h.seen, hash)
			removed++
		}
	}
	return removed
}

// ValidationHandler handles validation relay messages.
type ValidationHandler struct {
	mu   sync.RWMutex
	seen map[string]time.Time // hash -> first seen time

	// OnValidation is called when a validation is received
	OnValidation func(ctx context.Context, peerID PeerID, val *message.Validation)
}

// NewValidationHandler creates a new ValidationHandler.
func NewValidationHandler() *ValidationHandler {
	return &ValidationHandler{
		seen: make(map[string]time.Time),
	}
}

// HandleMessage handles validation messages.
func (h *ValidationHandler) HandleMessage(ctx context.Context, peerID PeerID, msg message.Message) error {
	val, ok := msg.(*message.Validation)
	if !ok {
		return nil
	}

	if h.OnValidation != nil {
		h.OnValidation(ctx, peerID, val)
	}

	return nil
}

// SquelchHandler handles squelch messages for reduce-relay.
type SquelchHandler struct {
	mu       sync.RWMutex
	squelched map[string]SquelchInfo // validator pubkey -> squelch info

	// OnSquelch is called when a squelch is received
	OnSquelch func(ctx context.Context, peerID PeerID, squelch *message.Squelch)
}

// SquelchInfo stores squelch state for a validator.
type SquelchInfo struct {
	ValidatorPubKey []byte
	Squelched       bool
	ExpiresAt       time.Time
	SourceID        PeerID
}

// NewSquelchHandler creates a new SquelchHandler.
func NewSquelchHandler() *SquelchHandler {
	return &SquelchHandler{
		squelched: make(map[string]SquelchInfo),
	}
}

// HandleMessage handles squelch messages.
func (h *SquelchHandler) HandleMessage(ctx context.Context, peerID PeerID, msg message.Message) error {
	squelch, ok := msg.(*message.Squelch)
	if !ok {
		return nil
	}

	key := string(squelch.ValidatorPubKey)

	h.mu.Lock()
	if squelch.Squelch {
		expiresAt := time.Now().Add(time.Duration(squelch.SquelchDuration) * time.Second)
		h.squelched[key] = SquelchInfo{
			ValidatorPubKey: squelch.ValidatorPubKey,
			Squelched:       true,
			ExpiresAt:       expiresAt,
			SourceID:        peerID,
		}
	} else {
		delete(h.squelched, key)
	}
	h.mu.Unlock()

	if h.OnSquelch != nil {
		h.OnSquelch(ctx, peerID, squelch)
	}

	return nil
}

// IsSquelched returns true if the validator is currently squelched.
func (h *SquelchHandler) IsSquelched(validatorPubKey []byte) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	info, exists := h.squelched[string(validatorPubKey)]
	if !exists {
		return false
	}
	return info.Squelched && time.Now().Before(info.ExpiresAt)
}

// PruneExpired removes expired squelch entries.
func (h *SquelchHandler) PruneExpired() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	removed := 0
	for key, info := range h.squelched {
		if now.After(info.ExpiresAt) {
			delete(h.squelched, key)
			removed++
		}
	}
	return removed
}

// StatusChangeHandler handles node status change messages.
type StatusChangeHandler struct {
	mu     sync.RWMutex
	status map[PeerID]PeerStatus

	// OnStatusChange is called when a status change is received
	OnStatusChange func(ctx context.Context, peerID PeerID, status *message.StatusChange)
}

// PeerStatus stores the last known status of a peer.
type PeerStatus struct {
	Status          message.NodeStatus
	Event           message.NodeEvent
	LedgerSeq       uint32
	LedgerHash      []byte
	NetworkTime     uint64
	FirstSeq        uint32
	LastSeq         uint32
	LastUpdate      time.Time
}

// NewStatusChangeHandler creates a new StatusChangeHandler.
func NewStatusChangeHandler() *StatusChangeHandler {
	return &StatusChangeHandler{
		status: make(map[PeerID]PeerStatus),
	}
}

// HandleMessage handles status change messages.
func (h *StatusChangeHandler) HandleMessage(ctx context.Context, peerID PeerID, msg message.Message) error {
	sc, ok := msg.(*message.StatusChange)
	if !ok {
		return nil
	}

	h.mu.Lock()
	h.status[peerID] = PeerStatus{
		Status:      sc.NewStatus,
		Event:       sc.NewEvent,
		LedgerSeq:   sc.LedgerSeq,
		LedgerHash:  sc.LedgerHash,
		NetworkTime: sc.NetworkTime,
		FirstSeq:    sc.FirstSeq,
		LastSeq:     sc.LastSeq,
		LastUpdate:  time.Now(),
	}
	h.mu.Unlock()

	if h.OnStatusChange != nil {
		h.OnStatusChange(ctx, peerID, sc)
	}

	return nil
}

// GetStatus returns the last known status of a peer.
func (h *StatusChangeHandler) GetStatus(peerID PeerID) (PeerStatus, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	status, exists := h.status[peerID]
	return status, exists
}

// GetAllStatuses returns all peer statuses.
func (h *StatusChangeHandler) GetAllStatuses() map[PeerID]PeerStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make(map[PeerID]PeerStatus)
	for k, v := range h.status {
		result[k] = v
	}
	return result
}

// RemovePeer removes status tracking for a peer.
func (h *StatusChangeHandler) RemovePeer(peerID PeerID) {
	h.mu.Lock()
	delete(h.status, peerID)
	h.mu.Unlock()
}
