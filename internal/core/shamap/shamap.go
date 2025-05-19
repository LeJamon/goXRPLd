package shamap

import (
	"sync"
)

// BranchFactor is the number of children each inner node can have.
const BranchFactor = 16

// Type indicates the type of data stored.
type Type int

// State represents the mutability state of the SHAMap.
type State int

const (
	Modifying State = iota
	Immutable
)

// SHAMap is a hash tree (Merkle Trie) structure.
type SHAMap struct {
	root    TreeNode
	state   State
	mapType Type
	cowID   uint32
	full    bool
	hasher  Hasher
	mutex   sync.RWMutex
}

// NewSHAMap constructs a new SHAMap with a given type and hasher.
func NewSHAMap(mapType Type, hasher Hasher) *SHAMap {
	m := &SHAMap{
		state:   Modifying,
		mapType: mapType,
		cowID:   1,
		full:    false,
		hasher:  hasher,
	}
	m.root = &InnerNode{
		children: [BranchFactor]TreeNode{},
		dirty:    true,
	}
	return m
}

// RootHash returns the hash of the root node of the SHAMap.
func (m *SHAMap) RootHash() [32]byte {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	if m.root == nil {
		return [32]byte{}
	}

	return m.calculateHash(m.root)
}

func (m *SHAMap) calculateHash(node TreeNode) [32]byte {
	if node == nil {
		return [32]byte{}
	}

	if !node.IsLeaf() {
		inner, ok := node.(*InnerNode)
		if !ok {
			return [32]byte{}
		}

		inner.mutex.RLock()
		defer inner.mutex.RUnlock()

		if !inner.dirty {
			return inner.hash
		}

		var blob []byte
		for i := 0; i < BranchFactor; i++ {
			if child := inner.children[i]; child != nil {
				h := m.calculateHash(child)
				blob = append(blob, byte(i))
				blob = append(blob, h[:]...)
			}
		}

		inner.hash = m.hasher.Hash(blob)
		inner.dirty = false
		return inner.hash
	}

	leaf, ok := node.(*LeafNode)
	if !ok || leaf.item == nil {
		return [32]byte{}
	}

	if leaf.hash == ([32]byte{}) {
		data := append(leaf.item.Key[:], leaf.item.Data...)
		leaf.hash = m.hasher.Hash(data)
	}

	return leaf.hash
}
