package shamap

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/bits"
)

const branchFactor = 16

type InnerNode struct {
	BaseNode
	children [branchFactor]SHAMapNode
	hashes   [branchFactor][32]byte
	isBranch uint16
}

func NewInnerNode() *InnerNode {
	return &InnerNode{}
}

// IsLeaf returns false — inner nodes are never leaves.
func (n *InnerNode) IsLeaf() bool {
	return false
}

// IsInner returns true — this is an inner node.
func (n *InnerNode) IsInner() bool {
	return true
}

// Type returns the SHAMapNodeType for this node.
func (n *InnerNode) Type() SHAMapNodeType {
	return tnINNER
}

// IsEmpty returns true if the node has no active branches.
func (n *InnerNode) IsEmpty() bool {
	return n.isBranch == 0
}

// IsEmptyBranch returns true if the given branch index is empty.
func (n *InnerNode) IsEmptyBranch(index int) bool {
	return (n.isBranch & (1 << index)) == 0
}

// GetBranchCount returns the number of active branches.
func (n *InnerNode) GetBranchCount() int {
	return bits.OnesCount16(n.isBranch)
}

// GetChild returns the child node at the given branch index.
func (n *InnerNode) GetChild(index int) SHAMapNode {
	return n.children[index]
}

// SetChild sets the child node at the given branch index and updates tracking info.
func (n *InnerNode) SetChild(index int, child SHAMapNode) {
	n.children[index] = child
	if child != nil {
		n.hashes[index] = child.Hash()
		n.isBranch |= 1 << index
	} else {
		n.hashes[index] = [32]byte{}
		n.isBranch &= ^(1 << index)
	}
	n.UpdateHash()
}

// GetChildHash returns the hash at a given branch index.
func (n *InnerNode) GetChildHash(index int) [32]byte {
	return n.hashes[index]
}

// UpdateHash recalculates the node's hash from its children.
func (n *InnerNode) UpdateHash() {
	var buffer bytes.Buffer
	for i := 0; i < branchFactor; i++ {
		if n.isBranch&(1<<i) != 0 {
			buffer.Write(n.hashes[i][:])
		}
	}
	n.setHash(buffer.Bytes())
}

// SerializeForWire (placeholder): serialize the node for wire transmission.
func (n *InnerNode) SerializeForWire() []byte {
	// Placeholder – wire format not defined yet.
	return nil
}

// SerializeWithPrefix (placeholder): serialize with type prefix.
func (n *InnerNode) SerializeWithPrefix() []byte {
	// Placeholder – same as above.
	return nil
}

// String returns a human-readable representation of the node.
func (n *InnerNode) String(id SHAMapNodeID) string {
	s := fmt.Sprintf("InnerNode ID: %s\nHash: %s\nBranches:\n", id.String(), hex.EncodeToString(n.hash[:]))
	for i := 0; i < branchFactor; i++ {
		if n.isBranch&(1<<i) != 0 {
			s += fmt.Sprintf("  %d: %s\n", i, hex.EncodeToString(n.hashes[i][:]))
		}
	}
	return s
}

// Invariants performs internal consistency checks.
func (n *InnerNode) Invariants(isRoot bool) error {
	count := 0
	for i := 0; i < branchFactor; i++ {
		hasChild := n.children[i] != nil
		hasBit := (n.isBranch & (1 << i)) != 0
		if hasChild != hasBit {
			return fmt.Errorf("branch %d inconsistency: child != bit", i)
		}
		if hasChild {
			count++
		}
	}
	if count == 0 && !isRoot {
		return fmt.Errorf("non-root inner node is empty")
	}
	return nil
}

// Clone returns a deep copy of the node.
func (n *InnerNode) Clone() SHAMapNode {
	copyNode := &InnerNode{
		isBranch: n.isBranch,
	}
	copy(copyNode.hash[:], n.hash[:])
	copy(copyNode.hashes[:], n.hashes[:])
	for i := 0; i < branchFactor; i++ {
		if n.children[i] != nil {
			copyNode.children[i] = n.children[i].Clone()
		}
	}
	return copyNode
}
