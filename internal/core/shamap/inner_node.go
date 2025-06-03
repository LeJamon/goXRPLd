package shamap

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/bits"
	"strings"
	"sync"

	"github.com/LeJamon/goXRPLd/internal/protocol"
)

const BranchFactor = 16

var (
	ErrInvalidBranch = errors.New("invalid branch index")
	ErrEmptyNonRoot  = errors.New("non-root inner node cannot be empty")
)

// InnerNode represents an inner node in the SHAMap tree
type InnerNode struct {
	BaseNode
	mu       sync.RWMutex
	children [BranchFactor]Node
	hashes   [BranchFactor][32]byte
	isBranch uint16
}

// NewInnerNode creates a new empty inner node
func NewInnerNode() *InnerNode {
	return &InnerNode{}
}

// IsLeaf returns false - inner nodes are never leaves
func (n *InnerNode) IsLeaf() bool {
	return false
}

// IsInner returns true - this is an inner node
func (n *InnerNode) IsInner() bool {
	return true
}

// Type returns the node type
func (n *InnerNode) Type() NodeType {
	return NodeTypeInner
}

// IsEmpty returns true if the node has no active branches
func (n *InnerNode) IsEmpty() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.isBranch == 0
}

// IsEmptyBranch returns true if the given branch index is empty
func (n *InnerNode) IsEmptyBranch(index int) bool {
	if index < 0 || index >= BranchFactor {
		return true
	}

	n.mu.RLock()
	defer n.mu.RUnlock()
	return (n.isBranch & (1 << index)) == 0
}

// BranchCount returns the number of active branches
func (n *InnerNode) BranchCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return bits.OnesCount16(n.isBranch)
}

// Child returns the child node at the given branch index
func (n *InnerNode) Child(index int) (Node, error) {
	if index < 0 || index >= BranchFactor {
		return nil, ErrInvalidBranch
	}

	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.children[index], nil
}

// ChildUnsafe returns the child without bounds checking or locking
// Use only when you're certain the index is valid and you hold the lock
func (n *InnerNode) ChildUnsafe(index int) Node {
	return n.children[index]
}

// SetChild sets the child node at the given branch index
func (n *InnerNode) SetChild(index int, child Node) error {
	if index < 0 || index >= BranchFactor {
		return ErrInvalidBranch
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	n.children[index] = child
	if child != nil {
		n.hashes[index] = child.Hash()
		n.isBranch |= 1 << index
	} else {
		n.hashes[index] = [32]byte{}
		n.isBranch &= ^(1 << index)
	}

	return n.updateHashUnsafe()
}

// ChildHash returns the hash at a given branch index
func (n *InnerNode) ChildHash(index int) ([32]byte, error) {
	if index < 0 || index >= BranchFactor {
		return [32]byte{}, ErrInvalidBranch
	}

	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.hashes[index], nil
}

// ChildHashUnsafe returns the hash without bounds checking or locking
func (n *InnerNode) ChildHashUnsafe(index int) [32]byte {
	return n.hashes[index]
}

// UpdateHash recalculates the node's hash from its children
func (n *InnerNode) UpdateHash() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.updateHashUnsafe()
}

// updateHashUnsafe updates hash without locking (caller must hold lock)
func (n *InnerNode) updateHashUnsafe() error {
	if n.isBranch == 0 {
		// Empty node - hash is zero
		n.hash = [32]byte{}
		return nil
	}

	var data [][]byte

	// Add inner node prefix
	data = append(data, protocol.HashPrefixInnerNode[:])

	// Include ALL 16 child hashes in order
	// Empty branches contribute zero hash (32 zero bytes)
	zeroHash := make([]byte, 32)
	for i := 0; i < BranchFactor; i++ {
		if n.isBranch&(1<<i) != 0 {
			child := n.children[i]
			if child != nil {
				childHash := child.Hash()
				data = append(data, childHash[:])
			} else {
				// This shouldn't happen if isBranch is correct, but handle it
				data = append(data, zeroHash)
			}
		} else {
			// Empty branch: contribute 32 zero bytes
			data = append(data, zeroHash)
		}
	}

	n.setHash(data...)
	return nil
}

// SerializeForWire serializes the node for wire transmission
func (n *InnerNode) SerializeForWire() ([]byte, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// TODO: Implement serialization logic
	return nil, errors.New("SerializeForWire not implemented")
}

// SerializeWithPrefix serializes with type prefix
func (n *InnerNode) SerializeWithPrefix() ([]byte, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// TODO: Implement serialization logic
	return nil, errors.New("SerializeWithPrefix not implemented")
}

// String returns a human-readable representation of the node
func (n *InnerNode) String(id NodeID) string {
	n.mu.RLock()
	defer n.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("InnerNode ID: %s\n", id.String()))
	sb.WriteString(fmt.Sprintf("Hash: %s\n", hex.EncodeToString(n.hash[:])))
	sb.WriteString("Branches:\n")

	for i := 0; i < BranchFactor; i++ {
		if n.isBranch&(1<<i) != 0 {
			sb.WriteString(fmt.Sprintf("  %d: %s\n", i, hex.EncodeToString(n.hashes[i][:])))
		}
	}

	return sb.String()
}

// Invariants performs internal consistency checks
func (n *InnerNode) Invariants(isRoot bool) error {
	n.mu.RLock()
	defer n.mu.RUnlock()

	count := 0
	for i := 0; i < BranchFactor; i++ {
		hasChild := n.children[i] != nil
		hasBit := (n.isBranch & (1 << i)) != 0

		if hasChild != hasBit {
			return fmt.Errorf("branch %d inconsistency: child != bit", i)
		}

		if hasChild {
			count++
			// Verify child hash matches stored hash
			childHash := n.children[i].Hash()
			if childHash != n.hashes[i] {
				return fmt.Errorf("branch %d hash mismatch", i)
			}
		}
	}

	if count == 0 && !isRoot {
		return ErrEmptyNonRoot
	}

	// Verify hash is correct
	if !n.IsZeroHash() {
		// Create a temporary copy to verify hash
		temp := &InnerNode{
			isBranch: n.isBranch,
			hashes:   n.hashes,
			children: n.children,
		}
		if err := temp.updateHashUnsafe(); err != nil {
			return fmt.Errorf("failed to verify hash: %w", err)
		}
		if temp.hash != n.hash {
			return fmt.Errorf("stored hash does not match computed hash")
		}
	}

	return nil
}

// Clone returns a deep copy of the node
func (n *InnerNode) Clone() (Node, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	clone := &InnerNode{
		BaseNode: BaseNode{hash: n.hash},
		isBranch: n.isBranch,
		hashes:   n.hashes, // Copy the array
	}

	// Deep clone children
	for i := 0; i < BranchFactor; i++ {
		if n.children[i] != nil {
			childClone, err := n.children[i].Clone()
			if err != nil {
				return nil, fmt.Errorf("failed to clone child at branch %d: %w", i, err)
			}
			clone.children[i] = childClone
		}
	}

	return clone, nil
}

// ForEachChild calls fn for each non-nil child with its branch index
// If fn returns false, iteration stops early
func (n *InnerNode) ForEachChild(fn func(index int, child Node) bool) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	for i := 0; i < BranchFactor; i++ {
		if n.children[i] != nil {
			if !fn(i, n.children[i]) {
				break
			}
		}
	}
}

// HasChildren returns true if the node has any children
func (n *InnerNode) HasChildren() bool {
	return !n.IsEmpty()
}
