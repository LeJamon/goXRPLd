package shamap

import (
	"errors"
	"fmt"
)

// SyncFilter provides callbacks for node handling during synchronization operations.
// This interface allows the SHAMap to interact with higher-level synchronization
// systems that can provide missing nodes or cache received nodes.
type SyncFilter interface {
	// GetNode retrieves a node by hash from the filter's cache or network.
	// Returns the node data and true if found, or nil and false if not available.
	GetNode(hash [32]byte) ([]byte, bool)

	// GotNode is called when a node is received or processed.
	// fromFilter indicates if the node came from this filter.
	// ledgerSeq is the ledger sequence this node belongs to.
	GotNode(fromFilter bool, hash [32]byte, ledgerSeq uint32, nodeData []byte, nodeType NodeType)
}

// AddNodeStatus represents the result of adding a node during synchronization
type AddNodeStatus int

const (
	// AddNodeUseful indicates the node was successfully added and was needed
	AddNodeUseful AddNodeStatus = iota
	// AddNodeDuplicate indicates the node was already present in the map
	AddNodeDuplicate
	// AddNodeInvalid indicates the node data was invalid or couldn't be processed
	AddNodeInvalid
)

// String returns a human-readable representation of the AddNodeStatus
func (s AddNodeStatus) String() string {
	switch s {
	case AddNodeUseful:
		return "useful"
	case AddNodeDuplicate:
		return "duplicate"
	case AddNodeInvalid:
		return "invalid"
	default:
		return "unknown"
	}
}

// AddNodeResult tracks the results of node addition operations during sync
type AddNodeResult struct {
	// Status indicates the primary result of the operation
	Status AddNodeStatus
	// Good tracks the number of successfully processed nodes
	Good int
	// Bad tracks the number of invalid or rejected nodes
	Bad int
	// Duplicate tracks the number of already-present nodes
	Duplicate int
}

// String returns a human-readable representation of the AddNodeResult
func (r AddNodeResult) String() string {
	return fmt.Sprintf("AddNodeResult{status=%s, good=%d, bad=%d, duplicate=%d}",
		r.Status, r.Good, r.Bad, r.Duplicate)
}

// NodeData represents a node and its identifier for batch operations
type NodeData struct {
	NodeID NodeID
	Data   []byte
}

// MissingNodeRequest represents a request for a missing node during sync
type MissingNodeRequest struct {
	NodeID NodeID
	Hash   [32]byte
}

// Sync-related errors
var (
	ErrSyncInProgress = errors.New("synchronization already in progress")
	ErrNotSyncing     = errors.New("map is not in syncing state")
	ErrInvalidRoot    = errors.New("invalid root node")
	ErrNodeMismatch   = errors.New("node hash doesn't match expected value")
	ErrInvalidNodeID  = errors.New("invalid node ID for this position")
)