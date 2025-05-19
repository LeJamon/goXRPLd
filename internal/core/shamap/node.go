package shamap

import (
	"encoding/hex"
	"fmt"
	"github.com/LeJamon/goXRPLd/internal/crypto/common"
)

type SHAMapNodeType int

const (
	tnINNER SHAMapNodeType = iota + 1
	tnTRANSACTION_NM
	tnTRANSACTION_MD
	tnACCOUNT_STATE
)

// SHAMapNode defines the interface all tree nodes must implement.
type SHAMapNode interface {
	IsLeaf() bool
	IsInner() bool
	Hash() [32]byte
	Type() SHAMapNodeType
	UpdateHash()
	SerializeForWire() []byte
	SerializeWithPrefix() []byte
	String(nodeID SHAMapNodeID) string
	Invariants(isRoot bool) error
	Clone() SHAMapNode
}

// BaseNode provides common functionality.
type BaseNode struct {
	hash [32]byte
}

func (b *BaseNode) Hash() [32]byte {
	return b.hash
}

func (b *BaseNode) setHash(data []byte) {
	b.hash = crypto.Sha512Half(data)
}

// Optional helper for display
func (b *BaseNode) String(id SHAMapNodeID) string {
	return fmt.Sprintf("Node ID: %v, Hash: %s", id.String(), hex.EncodeToString(b.hash[:]))
}
