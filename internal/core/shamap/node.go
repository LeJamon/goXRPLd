package shamap

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/LeJamon/goXRPLd/internal/crypto/common"
)

// NodeType defines the type of SHAMap node
type NodeType int

const (
	NodeTypeInner NodeType = iota + 1
	NodeTypeTransactionNoMeta
	NodeTypeTransactionWithMeta
	NodeTypeAccountState
)

// String returns a string representation of the node type
func (nt NodeType) String() string {
	switch nt {
	case NodeTypeInner:
		return "inner"
	case NodeTypeTransactionNoMeta:
		return "transaction"
	case NodeTypeTransactionWithMeta:
		return "transaction+meta"
	case NodeTypeAccountState:
		return "account_state"
	default:
		return fmt.Sprintf("unknown(%d)", int(nt))
	}
}

// Node defines the interface all tree nodes must implement
type Node interface {
	IsLeaf() bool
	IsInner() bool
	Hash() [32]byte
	Type() NodeType
	UpdateHash() error
	SerializeForWire() ([]byte, error)
	SerializeWithPrefix() ([]byte, error)
	String(nodeID NodeID) string
	Invariants(isRoot bool) error
	Clone() (Node, error)
}

// BaseNode provides common functionality for all node types
type BaseNode struct {
	hash [32]byte
}

// Hash returns the hash of the node
func (b *BaseNode) Hash() [32]byte {
	return b.hash
}

// setHash computes and sets the hash from the provided data
func (b *BaseNode) setHash(data ...[]byte) error {
	if len(data) == 0 {
		return fmt.Errorf("no data provided for hash calculation")
	}

	hash := crypto.Sha512Half(data...)
	b.hash = hash
	return nil
}

// String returns a string representation of the base node
func (b *BaseNode) String(id NodeID) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("NodeID: %s", id.String()))
	sb.WriteString(fmt.Sprintf(", Hash: %s", hex.EncodeToString(b.hash[:])))
	return sb.String()
}

// IsZeroHash returns true if the hash is zero (uninitialized)
func (b *BaseNode) IsZeroHash() bool {
	return b.hash == [32]byte{}
}
