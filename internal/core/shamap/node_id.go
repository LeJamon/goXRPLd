package shamap

import (
	"bytes"
	"encoding/hex"
	"fmt"
)

// SHAMapNodeID represents a node's position in the SHAMap.
type SHAMapNodeID struct {
	Depth uint8    // How many bits of the hash are relevant
	ID    [32]byte // The key prefix from the leaf's hash
}

// NewSHAMapNodeID creates a new SHAMapNodeID with given depth and hash.
func NewSHAMapNodeID(depth uint8, id [32]byte) SHAMapNodeID {
	return SHAMapNodeID{Depth: depth, ID: id}
}

// IsRoot returns true if this node is the root.
func (n SHAMapNodeID) IsRoot() bool {
	return n.Depth == 0
}

// RawBytes returns the wire format: 32-byte ID + 1-byte depth
func (n SHAMapNodeID) RawBytes() []byte {
	out := make([]byte, 33)
	copy(out[:32], n.ID[:])
	out[32] = n.Depth
	return out
}

// FromRawBytes parses a raw 33-byte SHAMapNodeID.
func FromRawBytes(data []byte) (SHAMapNodeID, error) {
	if len(data) != 33 {
		return SHAMapNodeID{}, fmt.Errorf("invalid SHAMapNodeID length: %d", len(data))
	}
	var id [32]byte
	copy(id[:], data[:32])
	depth := data[32]
	return SHAMapNodeID{Depth: depth, ID: id}, nil
}

// ChildNodeID returns the child node ID for the given branch (0–15).
func (n SHAMapNodeID) ChildNodeID(branch uint8) SHAMapNodeID {
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

	return SHAMapNodeID{Depth: newDepth, ID: newID}
}

// CreateNodeID creates a node ID for a given key and depth.
func CreateNodeID(depth uint8, key [32]byte) SHAMapNodeID {
	var id [32]byte
	copy(id[:], key[:])
	return SHAMapNodeID{Depth: depth, ID: id}
}

// SelectBranch returns which branch of a node would contain the given key.
func SelectBranch(node SHAMapNodeID, key [32]byte) uint8 {
	if node.Depth >= 64 {
		panic("node depth too deep")
	}
	byteIndex := node.Depth / 2
	isHighNibble := node.Depth%2 == 0

	if isHighNibble {
		return key[byteIndex] >> 4
	}
	return key[byteIndex] & 0x0F
}

// String returns a human-readable form of the node ID.
func (n SHAMapNodeID) String() string {
	if n.IsRoot() {
		return "NodeID(root)"
	}
	return fmt.Sprintf("NodeID(depth=%d, id=%s)", n.Depth, hex.EncodeToString(n.ID[:]))
}

// Equal compares two SHAMapNodeID values for equality.
func (n SHAMapNodeID) Equal(other SHAMapNodeID) bool {
	return n.Depth == other.Depth && bytes.Equal(n.ID[:], other.ID[:])
}
