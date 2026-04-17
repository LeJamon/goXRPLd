package peermanagement

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/LeJamon/goXRPLd/internal/peermanagement/message"
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

// Ledger sync constants.
const (
	// DefaultRequestTimeout is the default timeout for ledger data requests.
	DefaultRequestTimeout = 30 * time.Second

	// MaxConcurrentRequests is the maximum number of concurrent requests per peer.
	MaxConcurrentRequests = 5

	// MaxReplayDeltaResponseBytes caps the total uncompressed payload size of
	// a single mtREPLAY_DELTA_RESPONSE we will emit. Rippled does not enforce
	// an upstream cap, but our framing layer enforces its own limit
	// (message.MaxMessageSize = 64 MiB) and any response above that boundary
	// would be dropped at the codec layer. A 16 MiB ceiling leaves comfortable
	// headroom for the wire envelope and protects the event channel from
	// arbitrarily large allocations driven by remote requests.
	MaxReplayDeltaResponseBytes = 16 * 1024 * 1024
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
	Peer         PeerID
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
	// GetReplayDelta returns the serialized ledger header and every
	// transaction leaf blob (in tx-map order) for the given ledger hash.
	// Implementations must only return data for closed/immutable ledgers
	// (mirrors rippled's ledger->isImmutable() check in
	// LedgerReplayMsgHandler::processReplayDeltaRequest). When the ledger
	// is unknown or not yet immutable, return (nil, nil, nil) so the
	// handler can reply with reNO_LEDGER.
	GetReplayDelta(ledgerHash []byte) (header []byte, txLeaves [][]byte, err error)
}

// LedgerSyncHandler handles ledger synchronization messages.
type LedgerSyncHandler struct {
	mu sync.RWMutex

	// Pending requests
	requests map[uint64]*LedgerRequest

	// Request callbacks
	onLedgerData func(ctx context.Context, peerID PeerID, data *message.LedgerData)
	onProofPath  func(ctx context.Context, peerID PeerID, response *message.ProofPathResponse)

	// Data provider for responding to requests
	provider LedgerProvider

	// Request ID counter
	nextRequestID uint64

	// Event channel for sending responses
	events chan<- Event
}

// NewLedgerSyncHandler creates a new ledger sync handler.
func NewLedgerSyncHandler(events chan<- Event) *LedgerSyncHandler {
	return &LedgerSyncHandler{
		requests: make(map[uint64]*LedgerRequest),
		events:   events,
	}
}

// SetLedgerDataCallback sets the callback for incoming ledger data.
func (h *LedgerSyncHandler) SetLedgerDataCallback(fn func(ctx context.Context, peerID PeerID, data *message.LedgerData)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onLedgerData = fn
}

// SetProofPathCallback sets the callback for incoming proof paths.
func (h *LedgerSyncHandler) SetProofPathCallback(fn func(ctx context.Context, peerID PeerID, response *message.ProofPathResponse)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onProofPath = fn
}

// SetProvider sets the ledger data provider for responding to requests.
func (h *LedgerSyncHandler) SetProvider(provider LedgerProvider) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.provider = provider
}

// HandleMessage handles a ledger sync message.
func (h *LedgerSyncHandler) HandleMessage(ctx context.Context, peerID PeerID, msg message.Message) error {
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
func (h *LedgerSyncHandler) handleGetLedger(ctx context.Context, peerID PeerID, req *message.GetLedger) error {
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

	// Send response via events channel
	if h.events != nil && len(response.Nodes) > 0 {
		encoded, err := message.Encode(response)
		if err == nil {
			h.events <- Event{
				Type:    EventLedgerResponse,
				PeerID:  peerID,
				Payload: encoded,
			}
		}
	}

	return nil
}

// handleLedgerData handles incoming ledger data responses.
func (h *LedgerSyncHandler) handleLedgerData(ctx context.Context, peerID PeerID, data *message.LedgerData) error {
	h.mu.RLock()
	callback := h.onLedgerData
	h.mu.RUnlock()

	if callback != nil {
		callback(ctx, peerID, data)
	}

	return nil
}

// handleProofPathRequest handles proof path requests.
func (h *LedgerSyncHandler) handleProofPathRequest(ctx context.Context, peerID PeerID, req *message.ProofPathRequest) error {
	// TODO: Implement proof path generation
	return nil
}

// handleProofPathResponse handles proof path responses.
func (h *LedgerSyncHandler) handleProofPathResponse(ctx context.Context, peerID PeerID, resp *message.ProofPathResponse) error {
	h.mu.RLock()
	callback := h.onProofPath
	h.mu.RUnlock()

	if callback != nil {
		callback(ctx, peerID, resp)
	}

	return nil
}

// handleReplayDeltaRequest serves an inbound mtREPLAY_DELTA_REQUEST.
//
// Mirrors rippled's LedgerReplayMsgHandler::processReplayDeltaRequest
// (rippled/src/xrpld/app/ledger/detail/LedgerReplayMsgHandler.cpp:179-219):
//  1. Validate ledger_hash length == 32, else reply with reBAD_REQUEST.
//  2. Look up the ledger and require it to be immutable, else reply with
//     reNO_LEDGER. Both checks are folded into LedgerProvider.GetReplayDelta.
//  3. Pack the ledger header (addRaw on LedgerInfo) and every leaf blob in
//     the tx map, in tx-map iteration order.
//  4. Defensive size cap: if the response payload would exceed
//     MaxReplayDeltaResponseBytes, reply with reBAD_REQUEST (closest match
//     in TMReplyError, which has no reTOO_BUSY) and drop the populated
//     transaction list. Logged at warn level.
//
// The encoded response is pushed onto the events channel as
// EventLedgerResponse so the overlay can ship it to the requesting peer
// (mirrors handleGetLedger).
func (h *LedgerSyncHandler) handleReplayDeltaRequest(ctx context.Context, peerID PeerID, req *message.ReplayDeltaRequest) error {
	_ = ctx

	// Validate ledger_hash length first — this check is independent of any
	// configured provider, matching the rippled ordering.
	if len(req.LedgerHash) != 32 {
		return h.sendReplayDeltaResponse(peerID, &message.ReplayDeltaResponse{
			LedgerHash: req.LedgerHash,
			Error:      message.ReplyErrorBadRequest,
		})
	}

	h.mu.RLock()
	provider := h.provider
	h.mu.RUnlock()

	if provider == nil {
		// No provider wired: silently drop (matches handleGetLedger).
		return nil
	}

	header, txLeaves, err := provider.GetReplayDelta(req.LedgerHash)
	if err != nil || len(header) == 0 {
		return h.sendReplayDeltaResponse(peerID, &message.ReplayDeltaResponse{
			LedgerHash: req.LedgerHash,
			Error:      message.ReplyErrorNoLedger,
		})
	}

	// Defensive size cap: refuse to encode a response above our ceiling.
	total := len(header)
	for _, tx := range txLeaves {
		total += len(tx)
	}
	if total > MaxReplayDeltaResponseBytes {
		slog.Warn("ReplayDelta response oversized; refusing",
			"t", "LedgerSync",
			"peer", peerID,
			"size", total,
			"limit", MaxReplayDeltaResponseBytes,
		)
		return h.sendReplayDeltaResponse(peerID, &message.ReplayDeltaResponse{
			LedgerHash: req.LedgerHash,
			Error:      message.ReplyErrorBadRequest,
		})
	}

	return h.sendReplayDeltaResponse(peerID, &message.ReplayDeltaResponse{
		LedgerHash:   req.LedgerHash,
		LedgerHeader: header,
		Transactions: txLeaves,
	})
}

// sendReplayDeltaResponse encodes the response and pushes it onto the
// events channel for the overlay to deliver. Returns nil on a full or nil
// channel — callers treat send failures as best-effort, mirroring
// handleGetLedger which silently drops when the channel is unavailable.
func (h *LedgerSyncHandler) sendReplayDeltaResponse(peerID PeerID, resp *message.ReplayDeltaResponse) error {
	if h.events == nil {
		return nil
	}
	encoded, err := message.Encode(resp)
	if err != nil {
		slog.Warn("ReplayDelta encode failed", "t", "LedgerSync", "peer", peerID, "err", err)
		return nil
	}
	h.events <- Event{
		Type:    EventLedgerResponse,
		PeerID:  peerID,
		Payload: encoded,
	}
	return nil
}

// handleReplayDeltaResponse handles replay delta responses.
func (h *LedgerSyncHandler) handleReplayDeltaResponse(ctx context.Context, peerID PeerID, resp *message.ReplayDeltaResponse) error {
	// TODO: Process replay delta
	return nil
}

// CreateRequest creates a new ledger data request.
func (h *LedgerSyncHandler) CreateRequest(ledgerHash []byte, ledgerSeq uint32, dataType LedgerDataType) *LedgerRequest {
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
func (h *LedgerSyncHandler) GetPendingRequests(peerID PeerID) []*LedgerRequest {
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
func (h *LedgerSyncHandler) CleanupExpiredRequests() {
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
func (h *LedgerSyncHandler) PendingRequestCount() int {
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
