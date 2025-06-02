package shamap

import (
	"encoding/hex"
	"fmt"
	"math/bits"

	"github.com/LeJamon/goXRPLd/internal/protocol"
)

const branchFactor = 16

type InnerNode struct {
	BaseNode
	children [branchFactor]TreeNode
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
func (n *InnerNode) GetChild(index int) TreeNode {
	return n.children[index]
}

// SetChild sets the child node at the given branch index and updates tracking info.
func (n *InnerNode) SetChild(index int, child TreeNode) {
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
// CRITICAL: Must include ALL 16 child hashes in order, using zero hash for empty branches
func (n *InnerNode) UpdateHash() {
	if n.isBranch == 0 {
		// Empty node - hash is zero
		n.hash = [32]byte{}
		return
	}

	var data [][]byte

	// Add inner node prefix
	data = append(data, protocol.HashPrefixInnerNode[:])

	// CRITICAL: Include ALL 16 child hashes in order
	// Empty branches contribute zero hash (32 zero bytes)
	for i := 0; i < branchFactor; i++ {
		if n.isBranch&(1<<i) != 0 {
			child := n.GetChild(i)
			if child != nil {
				childHash := child.Hash()
				data = append(data, childHash[:])
			} else {
				// This shouldn't happen if isBranch is correct, but handle it
				data = append(data, make([]byte, 32))
			}
		} else {
			// Empty branch: contribute 32 zero bytes
			data = append(data, make([]byte, 32))
		}
	}

	n.setHash(data...)
}

// SerializeForWire (placeholder): serialize the node for wire transmission.
func (n *InnerNode) SerializeForWire() []byte {
	//TODO implement me
	panic("implement me")
}

// SerializeWithPrefix (placeholder): serialize with type prefix.
func (n *InnerNode) SerializeWithPrefix() []byte {
	//TODO implement me
	panic("implement me")
}

// String returns a human-readable representation of the node.
func (n *InnerNode) String(id NodeID) string {
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
func (n *InnerNode) Clone() TreeNode {
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
