package shamap

import (
	"bytes"
	"errors"
	"fmt"
)

// Common errors
var (
	ErrImmutable    = errors.New("cannot modify immutable SHAMap")
	ErrNilItem      = errors.New("cannot add nil item")
	ErrItemNotFound = errors.New("item not found")
	ErrInvalidType  = errors.New("invalid node type")
	ErrNodeNotFound = errors.New("node not found while traversing tree")
)

// State defines the state of the SHAMap.
type State int

const (
	Modifying State = iota
	Immutable
	Synching
	Invalid
)

// Type defines the shamap type.
type Type int

const (
	TxMap Type = iota
	StateMap
)

// SHAMap is the main structure representing the tree.
type SHAMap struct {
	root      *InnerNode // Root node (must be an inner node)
	mapType   Type       // Type of the map (TxMap or StateMap)
	state     State
	ledgerSeq uint32
	full      bool // Whether this map is fully loaded
}

// NewSHAMap creates a new empty SHAMap with the specified type.
func NewSHAMap(mapType Type) *SHAMap {
	return &SHAMap{
		root:      NewInnerNode(),
		mapType:   mapType,
		state:     Modifying,
		ledgerSeq: 0,
		full:      true,
	}
}

// NodeStack holds the path from the root to a node during tree traversal
type NodeStack struct {
	entries []stackEntry
}

type stackEntry struct {
	node TreeNode
	id   NodeID
}

// NewNodeStack creates a new empty node stack
func NewNodeStack() *NodeStack {
	return &NodeStack{
		entries: make([]stackEntry, 0, 64), // Pre-allocate for efficiency
	}
}

// Push adds a node and its ID to the stack
func (s *NodeStack) Push(node TreeNode, id NodeID) {
	s.entries = append(s.entries, stackEntry{node, id})
}

// Pop removes and returns the top node and ID from the stack
func (s *NodeStack) Pop() (TreeNode, NodeID, bool) {
	if len(s.entries) == 0 {
		return nil, NodeID{}, false
	}

	idx := len(s.entries) - 1
	node := s.entries[idx].node
	id := s.entries[idx].id
	s.entries = s.entries[:idx]

	return node, id, true
}

// Top returns the top node and ID without removing them
func (s *NodeStack) Top() (TreeNode, NodeID, bool) {
	if len(s.entries) == 0 {
		return nil, NodeID{}, false
	}

	idx := len(s.entries) - 1
	return s.entries[idx].node, s.entries[idx].id, true
}

// Empty returns true if the stack is empty
func (s *NodeStack) Empty() bool {
	return len(s.entries) == 0
}

// Clear removes all entries from the stack
func (s *NodeStack) Clear() {
	s.entries = s.entries[:0]
}

// walkTowardsKey traverses the tree toward a specific key
// If stack is provided, it will be filled with the path from root to the reached node
func (sm *SHAMap) walkTowardsKey(id [32]byte, stack *NodeStack) TreeNode {
	// Ensure stack is empty if provided
	if stack != nil && !stack.Empty() {
		stack.Clear()
	}

	var node TreeNode = sm.root
	nodeID := NewNodeID() // Start at root

	for node.IsInner() {
		if stack != nil {
			stack.Push(node, nodeID)
		}

		inner := node.(*InnerNode)
		branch := SelectBranch(nodeID, id)

		fmt.Printf("walkTowardsKey: nodeID: %v, branch: %d\n", nodeID, branch)

		if inner.IsEmptyBranch(int(branch)) {
			fmt.Println("walkTowardsKey: empty branch", branch)
			return nil
		}

		childNode := inner.GetChild(int(branch))
		if childNode == nil {
			fmt.Println("walkTowardsKey: nil child at branch", branch)
			return nil
		}

		node = childNode
		nodeID = nodeID.GetChildNodeID(uint8(branch))
	}

	if stack != nil {
		stack.Push(node, nodeID)
	}

	return node
}

// findKey returns the leaf node containing the specified key, or nil if not found
func (sm *SHAMap) findKey(id [32]byte) TreeNode {
	leaf := sm.walkTowardsKey(id, nil)
	if leaf == nil || !leaf.IsLeaf() {
		return nil
	}

	// Different leaf node types have different methods to access the item
	var key [32]byte

	switch node := leaf.(type) {
	case *AccountStateLeafNode:
		key = node.GetItem().Key()
	case *TxLeafNode:
		key = node.GetItem().Key()
	case *TxPlusMetaLeafNode:
		key = node.GetItem().Key()
	default:
		return nil
	}

	if !bytes.Equal(key[:], id[:]) {
		return nil
	}

	return leaf
}

// HasItem checks if an item with the given key exists
func (sm *SHAMap) HasItem(id [32]byte) bool {
	return sm.findKey(id) != nil
}

// FetchItem returns the item associated with the key, if present
func (sm *SHAMap) FetchItem(id [32]byte) (*SHAMapItem, bool) {
	leaf := sm.findKey(id)
	if leaf == nil {
		return nil, false
	}

	// Extract the item from the appropriate leaf node type
	var item *SHAMapItem

	switch node := leaf.(type) {
	case *AccountStateLeafNode:
		item = node.GetItem()
	case *TxLeafNode:
		item = node.GetItem()
	case *TxPlusMetaLeafNode:
		item = node.GetItem()
	default:
		return nil, false
	}

	return item, true
}

// makeTypedLeaf creates a new leaf node with the specified type
func makeTypedLeaf(nodeType SHAMapNodeType, item *SHAMapItem) TreeNode {
	switch nodeType {
	case tnACCOUNT_STATE:
		return NewAccountStateLeafNode(item)
	case tnTRANSACTION_NM:
		return NewTxLeafNode(item)
	case tnTRANSACTION_MD:
		return NewTxPlusMetaLeafNode(item)
	default:
		panic("unknown leaf node type")
	}
}

// dirtyUp updates the hashes of nodes after a modification
func (sm *SHAMap) dirtyUp(stack *NodeStack, target [32]byte, child TreeNode) {
	// Walk back up the tree, updating hashes
	for !stack.Empty() {
		node, nodeID, _ := stack.Pop()

		if !node.IsInner() {
			continue // Skip non-inner nodes
		}

		inner := node.(*InnerNode)
		branch := SelectBranch(nodeID, target)

		// Update the child pointer
		inner.SetChild(int(branch), child)

		// Update the hash
		inner.UpdateHash()

		// This node becomes the child for the next iteration
		child = inner
	}
}

// AddItem inserts or updates an item in the SHAMap
func (sm *SHAMap) AddItem(nodeType SHAMapNodeType, item *SHAMapItem) error {
	if sm.state == Immutable {
		return ErrImmutable
	}

	if item == nil {
		return ErrNilItem
	}

	// Validate node type
	switch nodeType {
	case tnACCOUNT_STATE, tnTRANSACTION_NM, tnTRANSACTION_MD:
		// Valid types
	default:
		return ErrInvalidType
	}

	// Add the item to the tree
	tag := item.Key()

	stack := NewNodeStack()
	node := sm.walkTowardsKey(tag, stack)

	if stack.Empty() {
		// Tree is empty, create root
		leaf := makeTypedLeaf(nodeType, item)
		sm.root = NewInnerNode()
		sm.root.SetChild(0, leaf)
		return nil
	}

	if node == nil {
		// We didn't find a leaf node, so we're at an inner node where
		// the branch for this key is empty
		innerNode, nodeID, _ := stack.Top()
		inner := innerNode.(*InnerNode)
		branch := SelectBranch(nodeID, tag)

		// Just add the leaf to this inner node
		inner.SetChild(int(branch), makeTypedLeaf(nodeType, item))
		inner.UpdateHash()

		// Update the hashes up the tree
		stack.Pop() // Remove the inner node we just modified
		sm.dirtyUp(stack, tag, inner)

		return nil
	}

	// We found a node at the target position
	if node.IsLeaf() {
		// Get the leaf's item
		var existingItem *SHAMapItem
		var existingType SHAMapNodeType

		switch leafNode := node.(type) {
		case *AccountStateLeafNode:
			existingItem = leafNode.GetItem()
			existingType = leafNode.Type()
		case *TxLeafNode:
			existingItem = leafNode.GetItem()
			existingType = leafNode.Type()
		case *TxPlusMetaLeafNode:
			existingItem = leafNode.GetItem()
			existingType = leafNode.Type()
		}

		// Check if we're updating an existing item with the same key
		key := existingItem.Key()
		if bytes.Equal(key[:], tag[:]) {
			// Item already exists with this key, update it
			if existingType != nodeType {
				return errors.New("cannot change node type during update")
			}

			// Update the item
			updated := false
			switch leafNode := node.(type) {
			case *AccountStateLeafNode:
				updated = leafNode.SetItem(item)
			case *TxLeafNode:
				updated = leafNode.SetItem(item)
			case *TxPlusMetaLeafNode:
				updated = leafNode.SetItem(item)
			}

			if updated {
				// Pop the leaf from the stack
				stack.Pop()

				// Update hashes up the tree
				sm.dirtyUp(stack, tag, node)
			}

			return nil
		}

		// Keys are different, need to create a new inner node
		// Remove the leaf from the stack
		stack.Pop()

		// Get the parent inner node
		parentNode, nodeID, ok := stack.Pop()
		if !ok {
			return errors.New("missing parent node")
		}

		parent := parentNode.(*InnerNode)
		parentBranch := SelectBranch(nodeID, tag)

		// Create a new inner node
		newInner := NewInnerNode()

		// Find the branching point
		childID := nodeID.GetChildNodeID(uint8(parentBranch))

		for {
			b1 := SelectBranch(childID, tag)
			b2 := SelectBranch(childID, existingItem.Key())

			if b1 != b2 {
				// Found the branching point
				newInner.SetChild(int(b1), makeTypedLeaf(nodeType, item))
				newInner.SetChild(int(b2), node)
				break
			}

			// Both items go down the same branch, need another level
			nextInner := NewInnerNode()

			newInner.SetChild(int(b1), nextInner)

			childID = childID.GetChildNodeID(uint8(b1))
			newInner = nextInner
		}

		// Update the parent to point to our new inner node
		parent.SetChild(int(parentBranch), newInner)
		parent.UpdateHash()

		// Update hashes up the tree
		sm.dirtyUp(stack, tag, parent)
	} else {
		// This shouldn't happen - walkTowardsKey should either return nil or a leaf node
		return errors.New("unexpected non-leaf node found")
	}

	return nil
}

// DelItem removes the item with the given key from the SHAMap.
// Returns true if the item was found and removed, false otherwise.
func (sm *SHAMap) DelItem(id [32]byte) bool {
	if sm.state == Immutable {
		panic("cannot delete from immutable SHAMap")
	}

	stack := NewNodeStack()
	leaf := sm.walkTowardsKey(id, stack)

	if stack.Empty() {
		// No path to the item found
		return false
	}

	// Confirm we found a matching leaf
	var existingItem *SHAMapItem
	var leafType SHAMapNodeType

	switch ln := leaf.(type) {
	case *AccountStateLeafNode:
		existingItem = ln.GetItem()
		leafType = ln.Type()
	case *TxLeafNode:
		existingItem = ln.GetItem()
		leafType = ln.Type()
	case *TxPlusMetaLeafNode:
		existingItem = ln.GetItem()
		leafType = ln.Type()
	default:
		return false
	}

	key := existingItem.Key()
	if !bytes.Equal(key[:], id[:]) {
		// Leaf found, but key doesn't match
		return false
	}

	// Remove leaf from stack
	stack.Pop()

	var prev TreeNode = nil

	for !stack.Empty() {
		node, nodeID, _ := stack.Pop()
		inner, ok := node.(*InnerNode)
		if !ok {
			panic("expected inner node")
		}

		branch := SelectBranch(nodeID, id)
		inner.SetChild(int(branch), prev)

		if nodeID.IsRoot() {
			// Always keep the root
			sm.root = inner
			inner.UpdateHash()
			return true
		}

		branchCount := inner.GetBranchCount()
		if branchCount == 0 {
			// No children left, delete this node
			prev = nil
		} else if branchCount == 1 {
			// Try to collapse into a single leaf node
			var single TreeNode
			var singleBranch int
			for i := 0; i < 16; i++ {
				if !inner.IsEmptyBranch(i) {
					single = inner.GetChild(i)
					singleBranch = i
					break
				}
			}

			// Attempt to promote the leaf if present
			if single != nil && single.IsLeaf() {
				item := GetItem(single)
				if item != nil {
					// Disconnect it from current inner
					inner.SetChild(singleBranch, nil)
					prev = makeTypedLeaf(leafType, item)
				} else {
					prev = inner
				}
			} else {
				prev = inner
			}
		} else {
			// Node is still valid with multiple children
			prev = inner
		}

		inner.UpdateHash()
	}

	// Update the root
	if prev != nil && prev.IsInner() {
		sm.root = prev.(*InnerNode)
	} else {
		sm.root = NewInnerNode()
	}

	return true
}

// SnapShot creates a copy of the SHAMap
// If isMutable is false, the returned SHAMap is Immutable
func (sm *SHAMap) SnapShot(isMutable bool) *SHAMap {
	newState := Immutable
	if isMutable {
		newState = Modifying
	}

	// Deep clone the root node
	newRoot := cloneNodeTree(sm.root)

	return &SHAMap{
		root:      newRoot,
		mapType:   sm.mapType,
		state:     newState,
		ledgerSeq: sm.ledgerSeq,
		full:      sm.full,
	}
}

// cloneNodeTree deep clones a node and all its children
func cloneNodeTree(node TreeNode) *InnerNode {
	if node == nil {
		return NewInnerNode()
	}

	if !node.IsInner() {
		panic("expected inner node")
	}

	inner := node.(*InnerNode)
	clone := inner.Clone().(*InnerNode)

	// Clone all the children recursively
	for i := 0; i < 16; i++ { // Assuming branch factor of 16
		if !inner.IsEmptyBranch(i) {
			child := inner.GetChild(i)
			if child.IsInner() {
				clone.SetChild(i, cloneNodeTree(child))
			} else {
				clone.SetChild(i, child.Clone())
			}
		}
	}

	return clone
}

// SetImmutable sets the SHAMap state to Immutable
func (sm *SHAMap) SetImmutable() {
	sm.state = Immutable
}

// SetFull marks the map as fully loaded
func (sm *SHAMap) SetFull() {
	sm.full = true
}

// SetLedgerSeq sets the ledger sequence number associated with the SHAMap
func (sm *SHAMap) SetLedgerSeq(seq uint32) {
	sm.ledgerSeq = seq
}

// VisitLeaves calls fn for every leaf item in the tree
func (sm *SHAMap) VisitLeaves(fn func(*SHAMapItem)) {
	var visitNode func(TreeNode)

	visitNode = func(node TreeNode) {
		if node == nil {
			return
		}

		if node.IsLeaf() {
			// Extract the item based on the leaf node type
			var item *SHAMapItem

			switch leafNode := node.(type) {
			case *AccountStateLeafNode:
				item = leafNode.GetItem()
			case *TxLeafNode:
				item = leafNode.GetItem()
			case *TxPlusMetaLeafNode:
				item = leafNode.GetItem()
			default:
				return
			}

			fn(item)
			return
		}

		inner := node.(*InnerNode)
		for i := 0; i < 16; i++ { // Assuming branch factor of 16
			if !inner.IsEmptyBranch(i) {
				visitNode(inner.GetChild(i))
			}
		}
	}

	visitNode(sm.root)
}

// VisitDifferences calls fn for items that differ between two SHAMaps
func (sm *SHAMap) VisitDifferences(other *SHAMap, fn func(*SHAMapItem)) {
	if other == nil {
		// If other is nil, treat all items in this map as different
		sm.VisitLeaves(fn)
		return
	}

	// Fast path: if root hashes match, maps are identical
	smHash := sm.GetHash()
	otherHash := other.GetHash()
	if bytes.Equal(smHash[:], otherHash[:]) {
		return
	}

	// Visit all leaves in this map
	sm.VisitLeaves(func(item *SHAMapItem) {
		// Check if item exists in other map
		otherItem, found := other.FetchItem(item.Key())

		// Call fn for items that don't exist in other map or have different data
		if !found || !bytes.Equal(item.Data(), otherItem.Data()) {
			fn(item)
		}
	})

	// Visit all leaves in other map that don't exist in this map
	other.VisitLeaves(func(item *SHAMapItem) {
		if !sm.HasItem(item.Key()) {
			fn(item)
		}
	})
}

// GetHash returns the root hash of the SHAMap
func (sm *SHAMap) GetHash() [32]byte {
	return sm.root.Hash()
}

func GetItem(node TreeNode) *SHAMapItem {
	if node == nil || !node.IsLeaf() {
		return nil
	}

	switch leaf := node.(type) {
	case *AccountStateLeafNode:
		return leaf.GetItem()
	case *TxLeafNode:
		return leaf.GetItem()
	case *TxPlusMetaLeafNode:
		return leaf.GetItem()
	default:
		return nil
	}
}

// selectBranch determines which branch a key follows at a given node ID
/*func selectBranch(nodeID NodeID, key [32]byte) int {
	depth := nodeID.Depth

	// Calculate byte index and a bit of position
	byteIdx := depth / 2
	bitPos := (depth % 2) * 4

	// If we're beyond the key length, return 0
	if byteIdx >= 32 {
		return 0
	}

	// Get the byte from the key
	keyByte := key[byteIdx]

	// Extract the correct nibble
	if bitPos == 0 {
		// Use high nibble (bits 4-7)
		return int(keyByte>>4) & 0x0F
	} else {
		// Use low nibble (bits 0-3)
		return int(keyByte & 0x0F)
	}
}*/
