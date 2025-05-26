package shamap

import (
	"bytes"
	"errors"
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
func (sm *SHAMap) walkTowardsKey(key [32]byte, stack *NodeStack) TreeNode {
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
		branch := SelectBranch(nodeID, key)
		if inner.IsEmptyBranch(int(branch)) {
			return nil
		}

		childNode := inner.GetChild(int(branch))
		if childNode == nil {
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

func (sm *SHAMap) dirtyUp(stack *NodeStack, target [32]byte, child TreeNode) TreeNode {
	if sm.state == Synching || sm.state == Immutable {
		panic("SHAMap: dirtyUp called in invalid state")
	}
	if child == nil {
		panic("SHAMap: dirtyUp called with nil child")
	}

	// Walk the tree up through the inner nodes to the root
	// Update hashes and links
	// Stack is a path of inner nodes up to, but not including, child
	// child can be an inner node or a leaf
	for !stack.Empty() {
		node, nodeID, ok := stack.Pop()
		if !ok {
			panic("SHAMap: dirtyUp - stack unexpectedly empty")
		}

		inner, ok := node.(*InnerNode)
		if !ok {
			panic("SHAMap: dirtyUp - expected InnerNode on stack")
		}

		branch := SelectBranch(nodeID, target)
		if branch < 0 || branch > 15 {
			panic("SHAMap: dirtyUp - invalid branch")
		}

		// Directly modify the node and update its hash
		inner.SetChild(int(branch), child)
		inner.UpdateHash()

		child = inner
	}

	return child
}

// walkTowardsKeyForDirty is similar to walkTowardsKey but only pushes inner nodes to the stack
// This is specifically for use with dirtyUp which expects only inner nodes on the stack
func (sm *SHAMap) walkTowardsKeyForDirty(key [32]byte, stack *NodeStack) TreeNode {
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
		branch := SelectBranch(nodeID, key)
		if inner.IsEmptyBranch(int(branch)) {
			return nil
		}

		childNode := inner.GetChild(int(branch))
		if childNode == nil {
			return nil
		}

		node = childNode
		nodeID = nodeID.GetChildNodeID(uint8(branch))
	}

	// Don't push the final leaf node - dirtyUp expects only inner nodes
	return node
}

// assignRoot safely assigns a new root, ensuring it's always an InnerNode
func (sm *SHAMap) assignRoot(newRoot TreeNode, key [32]byte) {
	if innerRoot, ok := newRoot.(*InnerNode); ok {
		sm.root = innerRoot
	} else {
		// If newRoot is a leaf, wrap it in an inner node
		// This can happen when the tree has only one item
		sm.root = NewInnerNode()
		branch := SelectBranch(NewNodeID(), key)
		sm.root.SetChild(int(branch), newRoot)
		sm.root.UpdateHash()
	}
}

// collectItemsBelow recursively collects all items below the given node
func (sm *SHAMap) collectItemsBelow(node TreeNode, items *[]*SHAMapItem) {
	if node == nil {
		return
	}

	if node.IsLeaf() {
		item := GetItem(node)
		if item != nil {
			*items = append(*items, item)
		}
		return
	}

	if inner, ok := node.(*InnerNode); ok {
		for i := 0; i < 16; i++ {
			if !inner.IsEmptyBranch(i) {
				child := inner.GetChild(i)
				sm.collectItemsBelow(child, items)

				// Early exit if we already found more than one item
				if len(*items) > 1 {
					return
				}
			}
		}
	}
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

func (sm *SHAMap) AddItem(item *SHAMapItem) error {
	if sm.state != Modifying {
		return ErrImmutable
	}
	if item == nil {
		return ErrNilItem
	}

	key := item.Key()
	stack := NewNodeStack()

	// Walk towards the key, building stack of inner nodes
	node := sm.walkTowardsKeyForDirty(key, stack)

	if node == nil {
		// Empty slot - create new leaf
		nodeType := sm.getLeafNodeType()
		newLeaf := makeTypedLeaf(nodeType, item)
		newRoot := sm.dirtyUp(stack, key, newLeaf)
		sm.assignRoot(newRoot, key)
		return nil
	}

	if !node.IsLeaf() {
		return ErrInvalidType
	}

	// Get the existing item from the leaf
	existingItem := GetItem(node)
	if existingItem == nil {
		return ErrInvalidType
	}

	existingKey := existingItem.Key()

	// Case 1: Same key - update existing item
	if bytes.Equal(key[:], existingKey[:]) {
		nodeType := sm.getLeafNodeType()
		updatedLeaf := makeTypedLeaf(nodeType, item)
		newRoot := sm.dirtyUp(stack, key, updatedLeaf)
		sm.assignRoot(newRoot, key)
		return nil
	}

	// Case 2: Different key - need to split
	// Find the depth at which these keys differ
	splitDepth := sm.findSplitDepth(key, existingKey)

	// Create the split structure
	newRoot, err := sm.createSplitStructure(key, existingKey, item, node, splitDepth, stack)
	if err != nil {
		return err
	}

	sm.assignRoot(newRoot, key)
	return nil
}

// findSplitDepth finds the depth at which two keys first differ
func (sm *SHAMap) findSplitDepth(key1, key2 [32]byte) int {
	for depth := 0; depth < 64; depth++ {
		branch1 := sm.getBranchAtDepth(key1, depth)
		branch2 := sm.getBranchAtDepth(key2, depth)
		if branch1 != branch2 {
			return depth
		}
	}
	// If we get here, the keys are identical (shouldn't happen)
	return 63
}

// getBranchAtDepth gets the branch (0-15) for a key at a specific depth
func (sm *SHAMap) getBranchAtDepth(key [32]byte, depth int) int {
	byteIndex := depth / 2
	if byteIndex >= 32 {
		return 0
	}

	b := key[byteIndex]
	if depth%2 == 0 {
		return int(b >> 4) // Use upper 4 bits
	} else {
		return int(b & 0xF) // Use lower 4 bits
	}
}

// createSplitStructure creates the inner node structure needed to separate two keys
func (sm *SHAMap) createSplitStructure(newKey, existingKey [32]byte, newItem *SHAMapItem, existingNode TreeNode, splitDepth int, stack *NodeStack) (TreeNode, error) {
	if splitDepth >= 64 {
		return nil, errors.New("maximum tree depth reached")
	}

	// Create new leaf for the new item
	nodeType := sm.getLeafNodeType()
	newLeaf := makeTypedLeaf(nodeType, newItem)

	// Create inner node at split depth
	splitInner := NewInnerNode()

	// Get branches at split depth
	newBranch := sm.getBranchAtDepth(newKey, splitDepth)
	existingBranch := sm.getBranchAtDepth(existingKey, splitDepth)

	// Add both nodes to the split inner node
	splitInner.SetChild(newBranch, newLeaf)
	splitInner.SetChild(existingBranch, existingNode)
	splitInner.UpdateHash()

	// If we need to create intermediate inner nodes, do so
	currentNode := TreeNode(splitInner)

	// Work backwards from split depth to current stack depth
	currentDepth := splitDepth - 1
	for currentDepth >= len(stack.entries) && currentDepth >= 0 {
		// Create intermediate inner node
		intermediateInner := NewInnerNode()
		branch := sm.getBranchAtDepth(newKey, currentDepth)
		intermediateInner.SetChild(branch, currentNode)
		intermediateInner.UpdateHash()
		currentNode = intermediateInner
		currentDepth--
	}

	// Now use dirtyUp to propagate changes up the existing stack
	finalRoot := sm.dirtyUp(stack, newKey, currentNode)
	return finalRoot, nil
}

// DeleteItem removes an item from the SHAMap
func (sm *SHAMap) DeleteItem(key [32]byte) error {
	if sm.state != Modifying {
		return ErrImmutable
	}

	stack := NewNodeStack()
	node := sm.walkTowardsKeyForDirty(key, stack)

	if node == nil || !node.IsLeaf() {
		return ErrItemNotFound
	}

	// Verify this is the correct item
	existingItem := GetItem(node)
	if existingItem == nil {
		return ErrItemNotFound
	}

	existingKey := existingItem.Key()
	if !bytes.Equal(key[:], existingKey[:]) {
		return ErrItemNotFound
	}

	// If stack is empty, we're deleting the only item in the tree
	if stack.Empty() {
		sm.root = NewInnerNode()
		return nil
	}

	// Remove the leaf by setting its parent's branch to nil
	parent, parentID, ok := stack.Pop()
	if !ok {
		return errors.New("unexpected empty stack")
	}

	parentInner, ok := parent.(*InnerNode)
	if !ok {
		return ErrInvalidType
	}

	branch := SelectBranch(parentID, key)
	parentInner.SetChild(int(branch), nil)
	parentInner.UpdateHash()

	// Check if we need to consolidate the tree
	consolidatedNode := sm.consolidateUp(parentInner, stack, key)
	if consolidatedNode != nil {
		sm.assignRoot(consolidatedNode, key)
	}

	return nil
}

// consolidateUp consolidates inner nodes that have only one child
func (sm *SHAMap) consolidateUp(startNode *InnerNode, stack *NodeStack, key [32]byte) TreeNode {
	currentNode := TreeNode(startNode)

	// Check if current node needs consolidation
	for {
		if !currentNode.IsInner() {
			break
		}

		inner := currentNode.(*InnerNode)
		childCount := 0
		var onlyChild TreeNode

		// Count non-empty children
		for i := 0; i < 16; i++ {
			if !inner.IsEmptyBranch(i) {
				childCount++
				onlyChild = inner.GetChild(i)
				if childCount > 1 {
					break // No need to count further
				}
			}
		}

		if childCount == 0 {
			// No children - this inner node should be removed
			if stack.Empty() {
				// This was the root, create new empty root
				return NewInnerNode()
			}

			// Get parent and remove this branch
			parent, parentID, ok := stack.Pop()
			if !ok {
				break
			}

			parentInner, ok := parent.(*InnerNode)
			if !ok {
				break
			}

			branch := SelectBranch(parentID, key)
			parentInner.SetChild(int(branch), nil)
			parentInner.UpdateHash()
			currentNode = parentInner
			continue

		} else if childCount == 1 {
			// Only one child - replace this inner node with its child
			if stack.Empty() {
				// This is the root - the child becomes the new root
				return onlyChild
			}

			// Get parent and replace this branch with the only child
			parent, parentID, ok := stack.Pop()
			if !ok {
				break
			}

			parentInner, ok := parent.(*InnerNode)
			if !ok {
				break
			}

			branch := SelectBranch(parentID, key)
			parentInner.SetChild(int(branch), onlyChild)
			parentInner.UpdateHash()
			currentNode = parentInner
			continue

		} else {
			// Multiple children - no consolidation needed
			break
		}
	}

	// Propagate remaining changes up if there's more stack
	if !stack.Empty() {
		return sm.dirtyUp(stack, key, currentNode)
	}

	return currentNode
}

// getLeafNodeType determines the appropriate leaf node type based on map type
func (sm *SHAMap) getLeafNodeType() SHAMapNodeType {
	switch sm.mapType {
	case TxMap:
		return tnTRANSACTION_NM // You might want tnTRANSACTION_MD for transactions with metadata
	case StateMap:
		return tnACCOUNT_STATE
	default:
		return tnACCOUNT_STATE
	}
}
