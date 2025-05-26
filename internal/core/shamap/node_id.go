package shamap

import (
	"bytes"
	"encoding/hex"
	"fmt"
)

// NodeID represents a node's position in the SHAMap.
type NodeID struct {
	Depth uint8    // How many bits of the hash are relevant
	ID    [32]byte // The key prefix from the leaf's hash
}

// NewSHAMapNodeID creates a new NodeID with given depth and hash.
func NewSHAMapNodeID(depth uint8, id [32]byte) NodeID {
	return NodeID{Depth: depth, ID: id}
}

// IsRoot returns true if this node is the root.
func (n NodeID) IsRoot() bool {
	return n.Depth == 0
}

// RawBytes returns the wire format: 32-byte ID + 1-byte depth
func (n NodeID) RawBytes() []byte {
	out := make([]byte, 33)
	copy(out[:32], n.ID[:])
	out[32] = n.Depth
	return out
}

// FromRawBytes parses a raw 33-byte NodeID.
func FromRawBytes(data []byte) (NodeID, error) {
	if len(data) != 33 {
		return NodeID{}, fmt.Errorf("invalid NodeID length: %d", len(data))
	}
	var id [32]byte
	copy(id[:], data[:32])
	depth := data[32]
	return NodeID{Depth: depth, ID: id}, nil
}

// GetChildNodeID returns the child node ID for the given branch (0–15).
func (n NodeID) GetChildNodeID(branch uint8) NodeID {
	if branch > 15 {
		panic("branch index must be between 0 and 15")
	}
	newDepth := n.Depth + 1
	var newID [32]byte
	copy(newID[:], n.ID[:])

	byteIndex := n.Depth / 2
	isHighNibble := n.Depth%2 == 0

	if isHighNibble {
		newID[byteIndex] = (newID[byteIndex] & 0x0F) | (branch << 4)
	} else {
		newID[byteIndex] = (newID[byteIndex] & 0xF0) | branch
	}

	return NodeID{Depth: newDepth, ID: newID}
}

// CreateNodeID creates a node ID for a given key and depth.
func CreateNodeID(depth uint8, key [32]byte) NodeID {
	var id [32]byte
	copy(id[:], key[:])
	return NodeID{Depth: depth, ID: id}
}

// SelectBranch returns which branch of a node would contain the given key.
func SelectBranch(nodeID NodeID, key [32]byte) int {
	depth := nodeID.Depth // You need to implement this method on NodeID
	byteIndex := depth / 2
	if byteIndex >= 32 {
		return 0
	}

	b := key[byteIndex]
	if depth%2 == 0 {
		return int(b >> 4) // Use upper 4 bits for even depths
	} else {
		return int(b & 0xF) // Use lower 4 bits for odd depths
	}
}

func NewNodeID() NodeID {
	return NodeID{Depth: 0, ID: [32]byte{}}
}

// String returns a human-readable form of the node ID.
func (n NodeID) String() string {
	if n.IsRoot() {
		return "NodeID(root)"
	}
	return fmt.Sprintf("NodeID(depth=%d, id=%s)", n.Depth, hex.EncodeToString(n.ID[:]))
}

// Equal compares two NodeID values for equality.
func (n NodeID) Equal(other NodeID) bool {
	return n.Depth == other.Depth && bytes.Equal(n.ID[:], other.ID[:])
}
