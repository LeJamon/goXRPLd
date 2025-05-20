package shamap

import (
	"errors"
)

// SHAMapState defines the state of the SHAMap.
type SHAMapState int

const (
	Modifying SHAMapState = iota
	Immutable
	Synching
	Invalid
)

// SHAMapType defines the map type.
type SHAMapType int

const (
	TxMap SHAMapType = iota
	StateMap
)

// SHAMap is the main structure representing the tree.
type SHAMap struct {
	root      InnerNode  // Root node (must be an inner node)
	mapType   SHAMapType // Type of the map (TxMap or StateMap)
	state     SHAMapState
	ledgerSeq uint32
}

// NewSHAMap creates a new empty SHAMap with the specified type.
func NewSHAMap(mapType SHAMapType) *SHAMap {
	return &SHAMap{
		root:      *NewInnerNode(), // TODO: Implement NewInnerNode() properly
		mapType:   mapType,
		state:     Modifying,
		ledgerSeq: 0,
	}
}

// AddItem inserts or updates an item in the SHAMap.
// TODO: Implement tree traversal, insertion, and update logic.
func (sm *SHAMap) AddItem(item *SHAMapItem) error {
	/*if sm.state == Immutable {
		return errors.New("cannot add item: SHAMap is immutable")
	}
	if item == nil {
		return errors.New("cannot add nil item")
	}

	key := item.Key()
	current := &sm.root
	var path []*InnerNode

	// Traverse down the tree to find the insertion point
	for {
		switch node := (*current).(type) {
		case *InnerNode:
			branch := selectBranch(key, node.prefix)
			child := node.children[branch]
			if child == nil {
				// Insert new leaf directly
				node.children[branch] = &LeafNode{item: item}
				sm.markDirty(path)
				return nil
			}
			path = append(path, node)
			current = &node.children[branch]

		case *LeafNode:
			existingKey := node.item.Key()
			if bytes.Equal(existingKey[:], key[:]) {
				return nil // Item already exists
			}

			// Conflict: convert leaf to inner node
			newInner := NewInnerNodeWithPrefix(commonPrefix(existingKey, key))
			b1 := selectBranch(existingKey, newInner.prefix)
			b2 := selectBranch(key, newInner.prefix)

			newInner.children[b1] = node
			newInner.children[b2] = &LeafNode{item: item}

			*current = newInner
			sm.markDirty(path)
			return nil

		default:
			return errors.New("invalid node encountered in tree")
		}
	}*/
	return nil
}

// DelItem removes an item by key from the SHAMap.
// TODO: Implement deletion logic, handle cases where item doesn't exist.
func (sm *SHAMap) DelItem(key [32]byte) error {
	// TODO: Check if map is Immutable, return error if so
	if sm.state == Immutable {
		return errors.New("cannot add immutable item")
	}
	// TODO: Remove item from tree, rebalance if needed
	return errors.New("DelItem not implemented")
}

// FetchItem returns the item associated with the key, if present.
// TODO: Implement tree traversal to find the item.
func (sm *SHAMap) FetchItem(key [32]byte) (*SHAMapItem, bool) {
	// TODO: Search the tree for the key and return the item and true if found
	return nil, false
}

// HasItem checks if an item with the given key exists.
// TODO: Implement similar to FetchItem but only returns boolean.
func (sm *SHAMap) HasItem(key [32]byte) bool {
	// TODO: Return true if item exists, false otherwise
	return false
}

// VisitLeaves calls fn for every leaf item in the tree.
// TODO: Implement recursive traversal visiting all leaf nodes.
func (sm *SHAMap) VisitLeaves(fn func(*SHAMapItem)) {
	// TODO: Traverse the tree and call fn on each leaf node item
}

// SnapShot creates a shallow copy of the SHAMap.
// If isMutable is false, the returned SHAMap is Immutable.
// TODO: Implement copying of nodes and state management.
func (sm *SHAMap) SnapShot(isMutable bool) *SHAMap {
	// TODO: Deep copy root, preserve mapType
	// TODO: Set state to Immutable if !isMutable, Modifying otherwise
	return nil
}

// SetImmutable sets the SHAMap state to Immutable.
func (sm *SHAMap) SetImmutable() {
	sm.state = Immutable
}

// SetFull marks the map as fully loaded or complete.
// TODO: Implement any logic needed when the SHAMap is fully constructed.
func (sm *SHAMap) SetFull() {
	// TODO: Implement state or flags indicating map is full
}

// SetLedgerSeq sets the ledger sequence number associated with the SHAMap.
func (sm *SHAMap) SetLedgerSeq(seq uint32) {
	sm.ledgerSeq = seq
}

// GetHash returns the root hash of the SHAMap.
func (sm *SHAMap) GetHash() [32]byte {
	return sm.root.Hash()
}

// Compare compares this SHAMap with another and returns the differences.
// TODO: Implement logic to identify differences between two SHAMaps.
// func (sm *SHAMap) Compare(other *SHAMap) []Difference {}

// VisitDifferences calls fn for items that differ between two SHAMaps.
// TODO: Implement traversal comparing two trees, calling fn on differing items.
func (sm *SHAMap) VisitDifferences(other *SHAMap, fn func(*SHAMapItem)) {
	// TODO: Compare trees and invoke fn on differing items
}
