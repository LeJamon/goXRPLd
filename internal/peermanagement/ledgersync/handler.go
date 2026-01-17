// Package ledgersync implements ledger synchronization message handlers for XRPL.
// It handles ledger data requests, proof paths, and replay deltas.
package ledgersync

import (
	"context"
	"sync"
	"time"

	"github.com/LeJamon/goXRPLd/internal/peermanagement/message"
	"github.com/LeJamon/goXRPLd/internal/peermanagement/protocol"
)

// LedgerDataType represents the type of ledger data being requested.
type LedgerDataType int

const (
	// LedgerDataTypeUnknown is an unknown data type.
	LedgerDataTypeUnknown LedgerDataType = iota
	// LedgerDataTypeHeader is the ledger header.
	LedgerDataTypeHeader
	// LedgerDataTypeAccountState is account state data.
	LedgerDataTypeAccountState
	// LedgerDataTypeTransactionNode is transaction tree nodes.
	LedgerDataTypeTransactionNode
	// LedgerDataTypeTransactionSetCandidate is transaction set candidates.
	LedgerDataTypeTransactionSetCandidate
)

// RequestState tracks the state of a ledger data request.
type RequestState int

const (
	// RequestStatePending means the request is waiting to be sent.
	RequestStatePending RequestState = iota
	// RequestStateSent means the request has been sent.
	RequestStateSent
	// RequestStateReceived means the response has been received.
	RequestStateReceived
	// RequestStateFailed means the request failed.
	RequestStateFailed
	// RequestStateTimeout means the request timed out.
	RequestStateTimeout
)

const (
	// DefaultRequestTimeout is the default timeout for ledger data requests.
	DefaultRequestTimeout = 30 * time.Second

	// MaxConcurrentRequests is the maximum number of concurrent requests per peer.
	MaxConcurrentRequests = 5
)

// LedgerRequest represents a request for ledger data.
type LedgerRequest struct {
	LedgerHash   []byte
	LedgerSeq    uint32
	DataType     LedgerDataType
	NodeIDs      [][]byte // Specific nodes to request
	State        RequestState
	CreatedAt    time.Time
	SentAt       time.Time
	CompletedAt  time.Time
	Peer         protocol.PeerID
	ResponseData [][]byte
	Error        error
}

// LedgerProvider is called to retrieve ledger data for responses.
type LedgerProvider interface {
	// GetLedgerHeader returns the header for a ledger.
	GetLedgerHeader(hash []byte, seq uint32) ([]byte, error)
	// GetAccountStateNode returns an account state node.
	GetAccountStateNode(ledgerHash []byte, nodeID []byte) ([]byte, error)
	// GetTransactionNode returns a transaction tree node.
	GetTransactionNode(ledgerHash []byte, nodeID []byte) ([]byte, error)
}

// Handler handles ledger synchronization messages.
type Handler struct {
	mu sync.RWMutex

	// Pending requests
	requests map[uint64]*LedgerRequest

	// Request callbacks
	onLedgerData func(ctx context.Context, peerID protocol.PeerID, data *message.LedgerData)
	onProofPath  func(ctx context.Context, peerID protocol.PeerID, response *message.ProofPathResponse)

	// Data provider for responding to requests
	provider LedgerProvider

	// Request ID counter
	nextRequestID uint64
}

// NewHandler creates a new ledger sync handler.
func NewHandler() *Handler {
	return &Handler{
		requests: make(map[uint64]*LedgerRequest),
	}
}

// SetLedgerDataCallback sets the callback for incoming ledger data.
func (h *Handler) SetLedgerDataCallback(fn func(ctx context.Context, peerID protocol.PeerID, data *message.LedgerData)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onLedgerData = fn
}

// SetProofPathCallback sets the callback for incoming proof paths.
func (h *Handler) SetProofPathCallback(fn func(ctx context.Context, peerID protocol.PeerID, response *message.ProofPathResponse)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onProofPath = fn
}

// SetProvider sets the ledger data provider for responding to requests.
func (h *Handler) SetProvider(provider LedgerProvider) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.provider = provider
}

// HandleMessage handles a ledger sync message.
func (h *Handler) HandleMessage(ctx context.Context, peerID protocol.PeerID, msg message.Message) error {
	switch m := msg.(type) {
	case *message.GetLedger:
		return h.handleGetLedger(ctx, peerID, m)
	case *message.LedgerData:
		return h.handleLedgerData(ctx, peerID, m)
	case *message.ProofPathRequest:
		return h.handleProofPathRequest(ctx, peerID, m)
	case *message.ProofPathResponse:
		return h.handleProofPathResponse(ctx, peerID, m)
	case *message.ReplayDeltaRequest:
		return h.handleReplayDeltaRequest(ctx, peerID, m)
	case *message.ReplayDeltaResponse:
		return h.handleReplayDeltaResponse(ctx, peerID, m)
	}
	return nil
}

// handleGetLedger handles incoming ledger data requests.
func (h *Handler) handleGetLedger(ctx context.Context, peerID protocol.PeerID, req *message.GetLedger) error {
	h.mu.RLock()
	provider := h.provider
	h.mu.RUnlock()

	if provider == nil {
		// No provider, can't respond
		return nil
	}

	// Build response
	response := &message.LedgerData{
		LedgerSeq:  req.LedgerSeq,
		LedgerHash: req.LedgerHash,
	}

	// Get requested data based on query type
	if req.QueryType == message.QueryTypeLedgerHeader {
		header, err := provider.GetLedgerHeader(req.LedgerHash, req.LedgerSeq)
		if err == nil && header != nil {
			response.Nodes = append(response.Nodes, message.LedgerNode{NodeData: header})
		}
	} else if req.QueryType == message.QueryTypeAccountState {
		for _, nodeID := range req.NodeIDs {
			node, err := provider.GetAccountStateNode(req.LedgerHash, nodeID)
			if err == nil && node != nil {
				response.Nodes = append(response.Nodes, message.LedgerNode{NodeData: node, NodeID: nodeID})
			}
		}
	} else if req.QueryType == message.QueryTypeTransactionData {
		for _, nodeID := range req.NodeIDs {
			node, err := provider.GetTransactionNode(req.LedgerHash, nodeID)
			if err == nil && node != nil {
				response.Nodes = append(response.Nodes, message.LedgerNode{NodeData: node, NodeID: nodeID})
			}
		}
	}

	// TODO: Send response through peer connection
	_ = response
	return nil
}

// handleLedgerData handles incoming ledger data responses.
func (h *Handler) handleLedgerData(ctx context.Context, peerID protocol.PeerID, data *message.LedgerData) error {
	h.mu.RLock()
	callback := h.onLedgerData
	h.mu.RUnlock()

	if callback != nil {
		callback(ctx, peerID, data)
	}

	return nil
}

// handleProofPathRequest handles proof path requests.
func (h *Handler) handleProofPathRequest(ctx context.Context, peerID protocol.PeerID, req *message.ProofPathRequest) error {
	// TODO: Implement proof path generation
	return nil
}

// handleProofPathResponse handles proof path responses.
func (h *Handler) handleProofPathResponse(ctx context.Context, peerID protocol.PeerID, resp *message.ProofPathResponse) error {
	h.mu.RLock()
	callback := h.onProofPath
	h.mu.RUnlock()

	if callback != nil {
		callback(ctx, peerID, resp)
	}

	return nil
}

// handleReplayDeltaRequest handles replay delta requests.
func (h *Handler) handleReplayDeltaRequest(ctx context.Context, peerID protocol.PeerID, req *message.ReplayDeltaRequest) error {
	// TODO: Implement replay delta generation
	return nil
}

// handleReplayDeltaResponse handles replay delta responses.
func (h *Handler) handleReplayDeltaResponse(ctx context.Context, peerID protocol.PeerID, resp *message.ReplayDeltaResponse) error {
	// TODO: Process replay delta
	return nil
}

// CreateRequest creates a new ledger data request.
func (h *Handler) CreateRequest(ledgerHash []byte, ledgerSeq uint32, dataType LedgerDataType) *LedgerRequest {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.nextRequestID++
	req := &LedgerRequest{
		LedgerHash: ledgerHash,
		LedgerSeq:  ledgerSeq,
		DataType:   dataType,
		State:      RequestStatePending,
		CreatedAt:  time.Now(),
	}
	h.requests[h.nextRequestID] = req

	return req
}

// GetPendingRequests returns all pending requests for a peer.
func (h *Handler) GetPendingRequests(peerID protocol.PeerID) []*LedgerRequest {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var result []*LedgerRequest
	for _, req := range h.requests {
		if req.State == RequestStateSent && req.Peer == peerID {
			result = append(result, req)
		}
	}
	return result
}

// CleanupExpiredRequests removes timed-out requests.
func (h *Handler) CleanupExpiredRequests() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	for id, req := range h.requests {
		if req.State == RequestStateSent && now.Sub(req.SentAt) > DefaultRequestTimeout {
			req.State = RequestStateTimeout
			delete(h.requests, id)
		}
	}
}

// PendingRequestCount returns the number of pending requests.
func (h *Handler) PendingRequestCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	count := 0
	for _, req := range h.requests {
		if req.State == RequestStatePending || req.State == RequestStateSent {
			count++
		}
	}
	return count
}
