package shamap

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
)

// Common errors
var (
	ErrImmutable       = errors.New("cannot modify immutable SHAMap")
	ErrNilItem         = errors.New("cannot add nil item")
	ErrItemNotFound    = errors.New("item not found")
	ErrInvalidType     = errors.New("invalid node type")
	ErrNodeNotFound    = errors.New("node not found while traversing tree")
	ErrMaxDepthReached = errors.New("maximum tree depth reached")
	ErrInvalidState    = errors.New("invalid state for operation")
	ErrUnknownNodeType = errors.New("unknown node type")
)

// State defines the state of the SHAMap
type State int

const (
	StateModifying State = iota
	StateImmutable
	StateSyncing
	StateInvalid
)

// String returns a string representation of the state
func (s State) String() string {
	switch s {
	case StateModifying:
		return "modifying"
	case StateImmutable:
		return "immutable"
	case StateSyncing:
		return "syncing"
	case StateInvalid:
		return "invalid"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// Type defines the SHAMap type
type Type int

const (
	TypeTransaction Type = iota
	TypeState
)

// String returns a string representation of the type
func (t Type) String() string {
	switch t {
	case TypeTransaction:
		return "transaction"
	case TypeState:
		return "state"
	default:
		return fmt.Sprintf("unknown(%d)", int(t))
	}
}

// SHAMap is the main structure representing the tree
type SHAMap struct {
	// Synchronization
	mu sync.RWMutex
	
	// Core tree structure  
	root *InnerNode
	
	// Configuration (immutable after creation)
	mapType Type
	
	// Runtime state
	state     State
	ledgerSeq uint32
	
	// Sync/storage state
	full   bool // true when all referenced nodes are loaded
	backed bool // true when backed by persistent storage
}

// New creates a new empty SHAMap with the specified type
func New(mapType Type) (*SHAMap, error) {
	root := NewInnerNode()

	return &SHAMap{
		root:      root,
		mapType:   mapType,
		state:     StateModifying,
		ledgerSeq: 0,
		full:      true,
		backed:    false,
	}, nil
}

// Type returns the map type
func (sm *SHAMap) Type() Type {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.mapType
}

// State returns the current state
func (sm *SHAMap) State() State {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state
}

// SetImmutable sets the SHAMap state to immutable
func (sm *SHAMap) SetImmutable() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state == StateInvalid {
		return errors.New("cannot set invalid map to immutable")
	}

	sm.state = StateImmutable
	return nil
}

// SetFull marks the map as fully loaded
func (sm *SHAMap) SetFull() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.full = true
}

// SetLedgerSeq sets the ledger sequence number
func (sm *SHAMap) SetLedgerSeq(seq uint32) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.ledgerSeq = seq
}

// Hash returns the root hash of the SHAMap
func (sm *SHAMap) Hash() ([32]byte, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.state == StateInvalid {
		return [32]byte{}, errors.New("cannot get hash of invalid map")
	}

	return sm.root.Hash(), nil
}

// pathEntry represents an entry in the traversal path
type pathEntry struct {
	node   Node
	nodeID NodeID
}

// NodeStack holds the path from the root to a node during tree traversal
type NodeStack struct {
	entries []pathEntry
}

// NewNodeStack creates a new empty node stack
func NewNodeStack() *NodeStack {
	return &NodeStack{
		entries: make([]pathEntry, 0, MaxDepth), // Pre-allocate for efficiency
	}
}

// Push adds a node and its ID to the stack
func (s *NodeStack) Push(node Node, id NodeID) {
	s.entries = append(s.entries, pathEntry{node, id})
}

// Pop removes and returns the top node and ID from the stack
func (s *NodeStack) Pop() (Node, NodeID, bool) {
	if len(s.entries) == 0 {
		return nil, NodeID{}, false
	}

	idx := len(s.entries) - 1
	entry := s.entries[idx]
	s.entries = s.entries[:idx]

	return entry.node, entry.nodeID, true
}

// Top returns the top node and ID without removing them
func (s *NodeStack) Top() (Node, NodeID, bool) {
	if len(s.entries) == 0 {
		return nil, NodeID{}, false
	}

	entry := s.entries[len(s.entries)-1]
	return entry.node, entry.nodeID, true
}

// IsEmpty returns true if the stack is empty
func (s *NodeStack) IsEmpty() bool {
	return len(s.entries) == 0
}

// Clear removes all entries from the stack
func (s *NodeStack) Clear() {
	s.entries = s.entries[:0]
}

// Len returns the number of entries in the stack
func (s *NodeStack) Len() int {
	return len(s.entries)
}

// walkToKey traverses the tree toward a specific key
func (sm *SHAMap) walkToKey(key [32]byte, stack *NodeStack) (Node, error) {
	if stack != nil && !stack.IsEmpty() {
		stack.Clear()
	}

	var node Node = sm.root
	nodeID := NewRootNodeID()

	for !node.IsLeaf() {
		if stack != nil {
			stack.Push(node, nodeID)
		}

		inner, ok := node.(*InnerNode)
		if !ok {
			return nil, ErrInvalidType
		}

		branch := SelectBranch(nodeID, key)
		if inner.IsEmptyBranch(int(branch)) {
			return nil, nil // Empty slot
		}

		child, err := inner.Child(int(branch))
		if err != nil {
			return nil, fmt.Errorf("failed to get child: %w", err)
		}
		if child == nil {
			return nil, nil // Empty slot
		}

		node = child
		childNodeID, err := nodeID.ChildNodeID(branch)
		if err != nil {
			return nil, fmt.Errorf("failed to get child node ID: %w", err)
		}
		nodeID = childNodeID
	}

	if stack != nil {
		stack.Push(node, nodeID)
	}

	return node, nil
}

// walkTowardsKey walks towards a key and builds a stack for proof generation
// This matches rippled's walkTowardsKey function used specifically for proof paths
// The stack will contain all nodes from root to target in the order they were visited
func (sm *SHAMap) walkTowardsKey(key [32]byte, stack *NodeStack) (Node, error) {
	if stack != nil && !stack.IsEmpty() {
		stack.Clear()
	}

	var node Node = sm.root
	nodeID := NewRootNodeID()

	// Always push the root node first (this is the key difference from walkToKey)
	if stack != nil {
		stack.Push(node, nodeID)
	}

	// Walk towards the key, pushing all nodes we traverse
	for !node.IsLeaf() {
		inner, ok := node.(*InnerNode)
		if !ok {
			return nil, ErrInvalidType
		}

		branch := SelectBranch(nodeID, key)
		if inner.IsEmptyBranch(int(branch)) {
			// We've gone as far as we can - the key doesn't exist
			// But we still return the path we've taken so far
			return nil, nil
		}

		child, err := inner.Child(int(branch))
		if err != nil {
			return nil, fmt.Errorf("failed to get child: %w", err)
		}
		if child == nil {
			// Empty slot - key doesn't exist
			return nil, nil
		}

		// Move to the child
		node = child
		childNodeID, err := nodeID.ChildNodeID(branch)
		if err != nil {
			return nil, fmt.Errorf("failed to get child node ID: %w", err)
		}
		nodeID = childNodeID

		// Push the child node to the stack
		if stack != nil {
			stack.Push(node, nodeID)
		}
	}

	// We've reached a leaf node - it's already been pushed to the stack above
	return node, nil
}

// findItem returns the item with the specified key, or nil if not found
func (sm *SHAMap) findItem(key [32]byte) (*Item, error) {
	node, err := sm.walkToKey(key, nil)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, nil
	}

	leafNode, ok := node.(LeafNode)
	if !ok {
		return nil, ErrInvalidType
	}

	item := leafNode.Item()
	itemKey := item.Key()
	if !bytes.Equal(itemKey[:], key[:]) {
		return nil, nil
	}

	return item, nil
}

// Has checks if an item with the given key exists
func (sm *SHAMap) Has(key [32]byte) (bool, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	item, err := sm.findItem(key)
	if err != nil {
		return false, err
	}
	return item != nil, nil
}

// Get returns the item associated with the key
func (sm *SHAMap) Get(key [32]byte) (*Item, bool, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	item, err := sm.findItem(key)
	if err != nil {
		return nil, false, err
	}
	return item, item != nil, nil
}

// Put adds or updates an item in the SHAMap
func (sm *SHAMap) Put(key [32]byte, data []byte) error {
	item := NewItem(key, data)
	return sm.PutItem(item)
}

// PutItem adds or updates an item in the SHAMap
func (sm *SHAMap) PutItem(item *Item) error {
	if item == nil {
		return ErrNilItem
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state != StateModifying {
		return ErrImmutable
	}

	return sm.putItemUnsafe(item)
}

// putItemUnsafe adds an item without locking (caller must hold lock)
func (sm *SHAMap) putItemUnsafe(item *Item) error {
	key := item.Key()
	stack := NewNodeStack()

	// Walk towards the key, building stack of inner nodes (excluding leaf)
	node, err := sm.walkToKeyForDirty(key, stack)
	if err != nil {
		return fmt.Errorf("failed to walk to key: %w", err)
	}

	if node == nil {
		// Empty slot - create new leaf
		nodeType, err := sm.getLeafNodeType()
		if err != nil {
			return err
		}

		newLeaf, err := sm.createTypedLeaf(nodeType, item)
		if err != nil {
			return fmt.Errorf("failed to create leaf: %w", err)
		}

		newRoot, err := sm.dirtyUp(stack, key, newLeaf)
		if err != nil {
			return fmt.Errorf("failed to dirty up: %w", err)
		}

		return sm.assignRoot(newRoot, key)
	}

	if !node.IsLeaf() {
		return ErrInvalidType
	}

	leafNode, ok := node.(LeafNode)
	if !ok {
		return ErrInvalidType
	}

	existingItem := leafNode.Item()
	existingKey := existingItem.Key()

	// Case 1: Same key - update existing item
	if bytes.Equal(key[:], existingKey[:]) {
		nodeType, err := sm.getLeafNodeType()
		if err != nil {
			return err
		}

		updatedLeaf, err := sm.createTypedLeaf(nodeType, item)
		if err != nil {
			return fmt.Errorf("failed to create updated leaf: %w", err)
		}

		newRoot, err := sm.dirtyUp(stack, key, updatedLeaf)
		if err != nil {
			return fmt.Errorf("failed to dirty up: %w", err)
		}

		return sm.assignRoot(newRoot, key)
	}

	// Case 2: Different key - need to split
	splitDepth := findSplitDepth(key, existingKey, stack.Len())
	newRoot, err := sm.createSplitStructure(key, existingKey, item, node, splitDepth, stack)
	if err != nil {
		return fmt.Errorf("failed to create split structure: %w", err)
	}

	return sm.assignRoot(newRoot, key)
}

// walkToKeyForDirty walks toward a key but doesn't include the final leaf in the stack
func (sm *SHAMap) walkToKeyForDirty(key [32]byte, stack *NodeStack) (Node, error) {
	if stack != nil && !stack.IsEmpty() {
		stack.Clear()
	}

	var node Node = sm.root
	nodeID := NewRootNodeID()

	for !node.IsLeaf() {
		if stack != nil {
			stack.Push(node, nodeID)
		}

		inner, ok := node.(*InnerNode)
		if !ok {
			return nil, ErrInvalidType
		}

		branch := SelectBranch(nodeID, key)
		if inner.IsEmptyBranch(int(branch)) {
			return nil, nil
		}

		child, err := inner.Child(int(branch))
		if err != nil {
			return nil, fmt.Errorf("failed to get child: %w", err)
		}
		if child == nil {
			return nil, nil
		}

		node = child
		childNodeID, err := nodeID.ChildNodeID(branch)
		if err != nil {
			return nil, fmt.Errorf("failed to get child node ID: %w", err)
		}
		nodeID = childNodeID
	}

	// Don't push the final leaf node to the stack
	return node, nil
}

// dirtyUp updates the tree from leaf to root
func (sm *SHAMap) dirtyUp(stack *NodeStack, target [32]byte, child Node) (Node, error) {
	if sm.state == StateSyncing || sm.state == StateImmutable {
		return nil, ErrInvalidState
	}
	if child == nil {
		return nil, errors.New("dirtyUp called with nil child")
	}

	currentChild := child
	for !stack.IsEmpty() {
		node, nodeID, ok := stack.Pop()
		if !ok {
			return nil, errors.New("stack unexpectedly empty")
		}

		inner, ok := node.(*InnerNode)
		if !ok {
			return nil, errors.New("expected InnerNode on stack")
		}

		branch := SelectBranch(nodeID, target)
		if err := inner.SetChild(int(branch), currentChild); err != nil {
			return nil, fmt.Errorf("failed to set child: %w", err)
		}

		currentChild = inner
	}

	return currentChild, nil
}

// assignRoot safely assigns a new root
func (sm *SHAMap) assignRoot(newRoot Node, key [32]byte) error {
	if innerRoot, ok := newRoot.(*InnerNode); ok {
		sm.root = innerRoot
		return nil
	}

	// If newRoot is a leaf, wrap it in an inner node
	sm.root = NewInnerNode()
	rootNodeID := NewRootNodeID()
	branch := SelectBranch(rootNodeID, key)

	if err := sm.root.SetChild(int(branch), newRoot); err != nil {
		return fmt.Errorf("failed to set child in new root: %w", err)
	}

	return nil
}

// Delete removes the item associated with the given key from the SHAMap.
// It first locates and removes the corresponding leaf node, then reconstructs
// the tree from the leaf's parent up to the root, consolidating as needed.
func (sm *SHAMap) Delete(key [32]byte) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state != StateModifying {
		return ErrImmutable
	}

	stack, _, err := sm.findAndRemoveLeaf(key)
	if err != nil {
		return err
	}

	newRoot, err := sm.consolidateAfterDelete(stack, key)
	if err != nil {
		return err
	}

	if rootInner, ok := newRoot.(*InnerNode); ok {
		sm.root = rootInner
	} else {
		return fmt.Errorf("expected root to be InnerNode, got %T", newRoot)
	}

	return nil
}

// findAndRemoveLeaf walks the SHAMap to locate the leaf node matching the key.
// It verifies the key, removes the leaf from the traversal stack, and returns
// the remaining stack for further processing.
func (sm *SHAMap) findAndRemoveLeaf(key [32]byte) (*NodeStack, LeafNode, error) {
	stack := NewNodeStack()
	_, err := sm.walkToKey(key, stack)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to walk to key: %w", err)
	}

	if stack.IsEmpty() {
		return nil, nil, ErrItemNotFound
	}

	leafNode, _, ok := stack.Pop()
	if !ok || !leafNode.IsLeaf() {
		return nil, nil, ErrItemNotFound
	}

	leaf, ok := leafNode.(LeafNode)
	if !ok {
		return nil, nil, ErrInvalidType
	}

	existingItem := leaf.Item()
	existingKey := existingItem.Key()
	if !bytes.Equal(key[:], existingKey[:]) {
		return nil, nil, ErrItemNotFound
	}

	return stack, leaf, nil
}

// consolidateAfterDelete reconstructs the SHAMap from a given node stack after
// a deletion. It applies bottom-up logic to restructure the tree and optimize
// it where possible (e.g., collapsing single-child inner nodes).
func (sm *SHAMap) consolidateAfterDelete(stack *NodeStack, key [32]byte) (Node, error) {
	var prevNode Node = nil

	for !stack.IsEmpty() {
		node, nodeID, ok := stack.Pop()
		if !ok {
			break
		}

		inner, ok := node.(*InnerNode)
		if !ok {
			return nil, ErrInvalidType
		}

		clonedNode, err := inner.Clone()
		if err != nil {
			return nil, fmt.Errorf("failed to clone inner node: %w", err)
		}

		clonedInner, ok := clonedNode.(*InnerNode)
		if !ok {
			return nil, ErrInvalidType
		}

		branch := SelectBranch(nodeID, key)
		if err := clonedInner.SetChild(int(branch), prevNode); err != nil {
			return nil, fmt.Errorf("failed to set child: %w", err)
		}

		if !nodeID.IsRoot() {
			switch clonedInner.BranchCount() {
			case 0:
				prevNode = nil
			case 1:
				onlyItem, err := sm.onlyBelow(clonedInner)
				if err != nil {
					return nil, fmt.Errorf("failed to check onlyBelow: %w", err)
				}

				if onlyItem != nil {
					nodeType, err := sm.getLeafNodeType()
					if err != nil {
						return nil, err
					}
					newLeaf, err := sm.createTypedLeaf(nodeType, onlyItem)
					if err != nil {
						return nil, fmt.Errorf("failed to create replacement leaf: %w", err)
					}
					prevNode = newLeaf
				} else {
					prevNode = clonedInner
				}
			default:
				prevNode = clonedInner
			}
		} else {
			// Always retain root
			prevNode = clonedInner
		}
	}

	if prevNode == nil {
		return nil, errors.New("unexpected nil root after deletion")
	}

	return prevNode, nil
}

// onlyBelow checks if there's exactly one item below the given node
// Returns the item if found, nil if there are 0 or multiple items
func (sm *SHAMap) onlyBelow(node Node) (*Item, error) {
	if node == nil {
		return nil, nil
	}

	current := node
	for !current.IsLeaf() {
		inner, ok := current.(*InnerNode)
		if !ok {
			return nil, ErrInvalidType
		}

		var nextNode Node = nil
		for i := 0; i < BranchFactor; i++ {
			child, err := inner.Child(i)
			if err != nil {
				return nil, fmt.Errorf("failed to get child %d: %w", i, err)
			}

			if child != nil {
				if nextNode != nil {
					// Found second child - multiple items below
					return nil, nil
				}
				nextNode = child
			}
		}

		if nextNode == nil {
			// No children found
			return nil, nil
		}

		current = nextNode
	}

	// Found exactly one leaf
	leaf, ok := current.(LeafNode)
	if !ok {
		return nil, ErrInvalidType
	}

	return leaf.Item(), nil
}

// Snapshot creates a copy of the SHAMap
func (sm *SHAMap) Snapshot(mutable bool) (*SHAMap, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.state == StateInvalid {
		return nil, errors.New("cannot snapshot invalid map")
	}

	newState := StateImmutable
	if mutable {
		newState = StateModifying
	}

	// Deep clone the root node
	newRoot, err := sm.cloneNodeTree(sm.root)
	if err != nil {
		return nil, fmt.Errorf("failed to clone tree: %w", err)
	}

	return &SHAMap{
		root:      newRoot,
		mapType:   sm.mapType,
		state:     newState,
		ledgerSeq: sm.ledgerSeq,
		full:      sm.full,
	}, nil
}

// ForEach calls fn for every item in the tree
// If fn returns false, iteration stops early
func (sm *SHAMap) ForEach(fn func(*Item) bool) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return sm.forEachUnsafe(sm.root, fn)
}

// forEachUnsafe recursively visits all items (caller must hold lock)
func (sm *SHAMap) forEachUnsafe(node Node, fn func(*Item) bool) error {
	if node == nil {
		return nil
	}

	if node.IsLeaf() {
		leafNode, ok := node.(LeafNode)
		if !ok {
			return ErrInvalidType
		}

		if !fn(leafNode.Item()) {
			return nil // Early termination requested
		}
		return nil
	}

	inner, ok := node.(*InnerNode)
	if !ok {
		return ErrInvalidType
	}

	for i := 0; i < BranchFactor; i++ {
		child, err := inner.Child(i)
		if err != nil {
			return fmt.Errorf("failed to get child %d: %w", i, err)
		}
		if child != nil {
			if err := sm.forEachUnsafe(child, fn); err != nil {
				return err
			}
		}
	}

	return nil
}

// Helper functions

// getLeafNodeType determines the appropriate leaf node type
func (sm *SHAMap) getLeafNodeType() (NodeType, error) {
	switch sm.mapType {
	case TypeTransaction:
		return NodeTypeTransactionNoMeta, nil
	case TypeState:
		return NodeTypeAccountState, nil
	default:
		return NodeType(0), fmt.Errorf("unknown map type: %v", sm.mapType)
	}
}

// createTypedLeaf creates a new leaf node with the specified type
func (sm *SHAMap) createTypedLeaf(nodeType NodeType, item *Item) (LeafNode, error) {
	return CreateLeafNode(nodeType, item)
}

// findSplitDepth finds the depth at which two keys first differ
func findSplitDepth(key1, key2 [32]byte, startDepth int) int {
	for depth := startDepth; depth < MaxDepth; depth++ {
		if getBranchAtDepth(key1, depth) != getBranchAtDepth(key2, depth) {
			return depth
		}
	}
	return MaxDepth - 1
}

// getBranchAtDepth gets the branch (0-15) for a key at a specific depth
func getBranchAtDepth(key [32]byte, depth int) int {
	if depth >= MaxDepth {
		return 0
	}

	byteIndex := depth / 2
	if byteIndex >= 32 {
		return 0
	}

	b := key[byteIndex]
	if depth%2 == 0 {
		return int(b >> 4) // Use upper 4 bits
	}
	return int(b & 0x0F) // Use lower 4 bits
}

// createSplitStructure creates the inner node structure needed to separate two keys
func (sm *SHAMap) createSplitStructure(newKey, existingKey [32]byte, newItem *Item, existingNode Node, splitDepth int, stack *NodeStack) (Node, error) {
	if splitDepth >= MaxDepth {
		return nil, ErrMaxDepthReached
	}

	// Create new leaf for the new item
	nodeType, err := sm.getLeafNodeType()
	if err != nil {
		return nil, err
	}

	newLeaf, err := sm.createTypedLeaf(nodeType, newItem)
	if err != nil {
		return nil, fmt.Errorf("failed to create new leaf: %w", err)
	}

	// Create inner node at split depth
	splitInner := NewInnerNode()

	// Get branches at split depth
	newBranch := getBranchAtDepth(newKey, splitDepth)
	existingBranch := getBranchAtDepth(existingKey, splitDepth)

	// Add both nodes to the split inner node
	if err := splitInner.SetChild(newBranch, newLeaf); err != nil {
		return nil, fmt.Errorf("failed to set new leaf: %w", err)
	}
	if err := splitInner.SetChild(existingBranch, existingNode); err != nil {
		return nil, fmt.Errorf("failed to set existing node: %w", err)
	}

	// Create intermediate inner nodes if needed
	currentNode := Node(splitInner)
	currentDepth := splitDepth - 1

	for currentDepth >= stack.Len() && currentDepth >= 0 {
		intermediateInner := NewInnerNode()
		branch := getBranchAtDepth(newKey, currentDepth)
		if err := intermediateInner.SetChild(branch, currentNode); err != nil {
			return nil, fmt.Errorf("failed to set intermediate node: %w", err)
		}
		currentNode = intermediateInner
		currentDepth--
	}

	// Use dirtyUp to propagate changes up the existing stack
	return sm.dirtyUp(stack, newKey, currentNode)
}

// consolidateUp removes empty inner nodes
func (sm *SHAMap) consolidateUp(startNode *InnerNode, stack *NodeStack, key [32]byte) (Node, error) {
	currentNode := Node(startNode)

	// Only consolidate completely empty nodes
	for {
		if !currentNode.IsInner() {
			break
		}

		inner, ok := currentNode.(*InnerNode)
		if !ok {
			break
		}

		if !inner.HasChildren() {
			// Empty node - remove it
			if stack.IsEmpty() {
				// This was the root
				return NewInnerNode(), nil
			}

			// Remove from parent
			parent, parentID, ok := stack.Pop()
			if !ok {
				break
			}

			parentInner, ok := parent.(*InnerNode)
			if !ok {
				return nil, ErrInvalidType
			}

			branch := SelectBranch(parentID, key)
			if err := parentInner.SetChild(int(branch), nil); err != nil {
				return nil, fmt.Errorf("failed to remove empty branch: %w", err)
			}

			currentNode = parentInner
			continue
		} else {
			// Has children - stop consolidation
			break
		}
	}

	// Propagate remaining changes up if there's more stack
	if !stack.IsEmpty() {
		return sm.dirtyUp(stack, key, currentNode)
	}

	return currentNode, nil
}

// cloneNodeTree deep clones a node and all its children
func (sm *SHAMap) cloneNodeTree(node Node) (*InnerNode, error) {
	if node == nil {
		return NewInnerNode(), nil
	}

	if !node.IsInner() {
		return nil, errors.New("expected inner node")
	}

	inner, ok := node.(*InnerNode)
	if !ok {
		return nil, ErrInvalidType
	}

	clone, err := inner.Clone()
	if err != nil {
		return nil, fmt.Errorf("failed to clone inner node: %w", err)
	}

	clonedInner, ok := clone.(*InnerNode)
	if !ok {
		return nil, ErrInvalidType
	}

	// Clone all children recursively
	for i := 0; i < BranchFactor; i++ {
		child, err := inner.Child(i)
		if err != nil {
			return nil, fmt.Errorf("failed to get child %d: %w", i, err)
		}

		if child != nil {
			var clonedChild Node
			if child.IsInner() {
				clonedChild, err = sm.cloneNodeTree(child)
				if err != nil {
					return nil, fmt.Errorf("failed to clone child tree %d: %w", i, err)
				}
			} else {
				clonedChild, err = child.Clone()
				if err != nil {
					return nil, fmt.Errorf("failed to clone leaf %d: %w", i, err)
				}
			}

			if err := clonedInner.SetChild(i, clonedChild); err != nil {
				return nil, fmt.Errorf("failed to set cloned child %d: %w", i, err)
			}
		}
	}

	return clonedInner, nil
}

// Go 1.23+ range-over function iterators

// All returns an iterator that yields all items in the SHAMap
// Compatible with Go 1.23+ range-over function iterators
func (sm *SHAMap) All() func(func(*Item) bool) {
	return func(yield func(*Item) bool) {
		sm.mu.RLock()
		defer sm.mu.RUnlock()
		sm.forEachUnsafe(sm.root, yield)
	}
}

// Keys returns an iterator that yields all keys in the SHAMap
// Compatible with Go 1.23+ range-over function iterators
func (sm *SHAMap) Keys() func(func([32]byte) bool) {
	return func(yield func([32]byte) bool) {
		sm.mu.RLock()
		defer sm.mu.RUnlock()
		sm.forEachUnsafe(sm.root, func(item *Item) bool {
			return yield(item.Key())
		})
	}
}

// Items returns an iterator that yields key-value pairs in the SHAMap
// Compatible with Go 1.23+ range-over function iterators
func (sm *SHAMap) Items() func(func([32]byte, []byte) bool) {
	return func(yield func([32]byte, []byte) bool) {
		sm.mu.RLock()
		defer sm.mu.RUnlock()
		sm.forEachUnsafe(sm.root, func(item *Item) bool {
			return yield(item.Key(), item.Data())
		})
	}
}

// Nodes returns an iterator that yields all nodes in the SHAMap
// Compatible with Go 1.23+ range-over function iterators
func (sm *SHAMap) Nodes() func(func(Node) bool) {
	return func(yield func(Node) bool) {
		sm.mu.RLock()
		defer sm.mu.RUnlock()
		sm.visitNodesUnsafe(sm.root, yield)
	}
}

// visitNodesUnsafe recursively visits all nodes (caller must hold lock)
func (sm *SHAMap) visitNodesUnsafe(node Node, yield func(Node) bool) bool {
	if node == nil {
		return true
	}

	// Visit this node
	if !yield(node) {
		return false
	}

	// If it's an inner node, visit children
	if node.IsInner() {
		inner, ok := node.(*InnerNode)
		if !ok {
			return false
		}

		for i := 0; i < BranchFactor; i++ {
			child, err := inner.Child(i)
			if err != nil {
				return false
			}
			if child != nil {
				if !sm.visitNodesUnsafe(child, yield) {
					return false
				}
			}
		}
	}

	return true
}

// visitNodes visits every node in this SHAMap
func (sm *SHAMap) VisitNodes(fn func(Node) bool) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if !sm.visitNodesUnsafe(sm.root, fn) {
		return nil // Early termination requested
	}
	return nil
}

// visitLeaves visits every leaf node in this SHAMap
func (sm *SHAMap) VisitLeaves(fn func(*Item)) error {
	return sm.ForEach(func(item *Item) bool {
		fn(item)
		return true // Continue iteration
	})
}

// visitDifferences visits every node in this SHAMap that is not present in the other SHAMap
func (sm *SHAMap) VisitDifferences(other *SHAMap, fn func(Node) bool) error {
	if other == nil {
		return sm.VisitNodes(fn)
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()
	other.mu.RLock()
	defer other.mu.RUnlock()

	return sm.visitDifferencesUnsafe(sm.root, other.root, fn)
}

// visitDifferencesUnsafe recursively compares nodes and visits differences
func (sm *SHAMap) visitDifferencesUnsafe(ourNode, theirNode Node, fn func(Node) bool) error {
	// If their node is nil, our entire subtree is different
	if theirNode == nil {
		return sm.visitSubtreeUnsafe(ourNode, fn)
	}

	// If our node is nil, nothing to visit
	if ourNode == nil {
		return nil
	}

	// If hashes match, subtrees are identical
	if ourNode.Hash() == theirNode.Hash() {
		return nil
	}

	// Hashes differ - visit this node
	if !fn(ourNode) {
		return nil // Early termination requested
	}

	// If both are leaves, we're done (already visited the differing node)
	if ourNode.IsLeaf() || theirNode.IsLeaf() {
		return nil
	}

	// Both are inner nodes - compare children
	ourInner, ok1 := ourNode.(*InnerNode)
	theirInner, ok2 := theirNode.(*InnerNode)
	if !ok1 || !ok2 {
		return ErrInvalidType
	}

	for i := 0; i < BranchFactor; i++ {
		ourChild, err1 := ourInner.Child(i)
		theirChild, err2 := theirInner.Child(i)

		if err1 != nil || err2 != nil {
			continue
		}

		if err := sm.visitDifferencesUnsafe(ourChild, theirChild, fn); err != nil {
			return err
		}
	}

	return nil
}

// visitSubtreeUnsafe visits all nodes in a subtree
func (sm *SHAMap) visitSubtreeUnsafe(node Node, fn func(Node) bool) error {
	if node == nil {
		return nil
	}

	if !fn(node) {
		return nil // Early termination requested
	}

	if node.IsInner() {
		inner, ok := node.(*InnerNode)
		if !ok {
			return ErrInvalidType
		}

		for i := 0; i < BranchFactor; i++ {
			child, err := inner.Child(i)
			if err != nil {
				continue
			}
			if child != nil {
				if err := sm.visitSubtreeUnsafe(child, fn); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// MissingNode represents a missing node needed for synchronization
type MissingNode struct {
	NodeID NodeID
	Hash   [32]byte
}

// GetMissingNodes traverses the SHAMap to find nodes that are referenced but not available
// This is used for synchronization to determine what nodes need to be fetched
func (sm *SHAMap) GetMissingNodes(maxNodes int) ([]MissingNode, error) {
	if maxNodes <= 0 {
		maxNodes = 100 // Default reasonable limit
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var missingNodes []MissingNode
	visited := make(map[[32]byte]bool)

	err := sm.findMissingNodesUnsafe(sm.root, NewRootNodeID(), visited, &missingNodes, &maxNodes)
	if err != nil {
		return nil, err
	}

	return missingNodes, nil
}

// findMissingNodesUnsafe recursively finds missing nodes
func (sm *SHAMap) findMissingNodesUnsafe(node Node, nodeID NodeID, visited map[[32]byte]bool, missingNodes *[]MissingNode, maxNodes *int) error {
	if *maxNodes <= 0 {
		return nil // Reached limit
	}

	if node == nil {
		return nil
	}

	nodeHash := node.Hash()

	// Skip if already visited this hash
	if visited[nodeHash] {
		return nil
	}
	visited[nodeHash] = true

	// For inner nodes, check children
	if node.IsInner() {
		inner, ok := node.(*InnerNode)
		if !ok {
			return ErrInvalidType
		}

		for i := 0; i < BranchFactor; i++ {
			if inner.IsEmptyBranch(i) {
				continue
			}

			// Get child hash
			childHash, exists := inner.GetChildHash(i)
			if !exists {
				continue
			}

			// Get actual child node
			child, err := inner.Child(i)
			if err != nil {
				return err
			}

			if child == nil {
				// Child is missing - add to missing list
				childNodeID, err := nodeID.ChildNodeID(uint8(i))
				if err != nil {
					return err
				}

				*missingNodes = append(*missingNodes, MissingNode{
					NodeID: childNodeID,
					Hash:   childHash,
				})
				(*maxNodes)--

				if *maxNodes <= 0 {
					return nil
				}
			} else {
				// Child exists - recurse
				childNodeID, err := nodeID.ChildNodeID(uint8(i))
				if err != nil {
					return err
				}

				err = sm.findMissingNodesUnsafe(child, childNodeID, visited, missingNodes, maxNodes)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// WalkMap traverses the entire SHAMap and reports any missing nodes
// This is a comprehensive check for map integrity
func (sm *SHAMap) WalkMap() ([]MissingNode, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var missingNodes []MissingNode
	visited := make(map[[32]byte]bool)

	err := sm.walkMapUnsafe(sm.root, NewRootNodeID(), visited, &missingNodes)
	if err != nil {
		return nil, err
	}

	return missingNodes, nil
}

// walkMapUnsafe performs the actual traversal work
func (sm *SHAMap) walkMapUnsafe(node Node, nodeID NodeID, visited map[[32]byte]bool, missingNodes *[]MissingNode) error {
	if node == nil {
		return nil
	}

	nodeHash := node.Hash()

	// Skip if already visited this hash
	if visited[nodeHash] {
		return nil
	}
	visited[nodeHash] = true

	// For inner nodes, check all children
	if node.IsInner() {
		inner, ok := node.(*InnerNode)
		if !ok {
			return ErrInvalidType
		}

		for i := 0; i < BranchFactor; i++ {
			if inner.IsEmptyBranch(i) {
				continue
			}

			// Get child
			child, err := inner.Child(i)
			if err != nil {
				return err
			}

			childNodeID, err := nodeID.ChildNodeID(uint8(i))
			if err != nil {
				return err
			}

			if child == nil {
				// Child is missing - add to missing list
				childHash, _ := inner.GetChildHash(i)
				*missingNodes = append(*missingNodes, MissingNode{
					NodeID: childNodeID,
					Hash:   childHash,
				})
			} else {
				// Child exists - recurse
				err = sm.walkMapUnsafe(child, childNodeID, visited, missingNodes)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// ============================================================================
// Synchronization Methods
// ============================================================================

// SetSyncing sets the map to syncing state
func (sm *SHAMap) SetSyncing() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state == StateSyncing {
		return ErrSyncInProgress
	}

	sm.state = StateSyncing
	return nil
}

// ClearSyncing clears the syncing state and sets to modifying
func (sm *SHAMap) ClearSyncing() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state != StateSyncing {
		return ErrNotSyncing
	}

	sm.state = StateModifying
	return nil
}

// IsSyncing returns true if the map is in syncing state
func (sm *SHAMap) IsSyncing() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state == StateSyncing
}

// LedgerSeq returns the ledger sequence number
func (sm *SHAMap) LedgerSeq() uint32 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.ledgerSeq
}

// IsFull returns true if the map is marked as full
func (sm *SHAMap) IsFull() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.full
}

// SetBacked marks the map as backed by persistent storage
func (sm *SHAMap) SetBacked() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.backed = true
}

// IsBacked returns true if the map is backed by persistent storage
func (sm *SHAMap) IsBacked() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.backed
}

// AddKnownNode adds a node received from a peer during synchronization
func (sm *SHAMap) AddKnownNode(nodeID NodeID, nodeData []byte, filter SyncFilter) AddNodeResult {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	result := AddNodeResult{}

	// Deserialize the node
	node, err := DeserializeNodeFromWire(nodeData)
	if err != nil {
		result.Status = AddNodeInvalid
		result.Bad = 1
		return result
	}

	// Verify the node's hash matches what we expect
	nodeHash := node.Hash()
	expectedHash := sm.getExpectedHashUnsafe(nodeID)
	if expectedHash != [32]byte{} && nodeHash != expectedHash {
		result.Status = AddNodeInvalid
		result.Bad = 1
		return result
	}

	// Try to add the node to the tree
	added, err := sm.addKnownNodeUnsafe(nodeID, node)
	if err != nil {
		result.Status = AddNodeInvalid
		result.Bad = 1
		return result
	}

	if !added {
		result.Status = AddNodeDuplicate
		result.Duplicate = 1
		return result
	}

	result.Status = AddNodeUseful
	result.Good = 1

	// Notify the filter
	if filter != nil {
		filter.GotNode(false, nodeHash, sm.ledgerSeq, nodeData, node.Type())
	}

	return result
}

// AddRootNode sets the root node during synchronization
func (sm *SHAMap) AddRootNode(hash [32]byte, nodeData []byte, filter SyncFilter) AddNodeResult {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	result := AddNodeResult{}

	// Deserialize the root node
	node, err := DeserializeNodeFromWire(nodeData)
	if err != nil {
		result.Status = AddNodeInvalid
		result.Bad = 1
		return result
	}

	// Verify the hash matches
	if node.Hash() != hash {
		result.Status = AddNodeInvalid
		result.Bad = 1
		return result
	}

	// Root must be an inner node (in rippled, roots are always inner nodes)
	if !node.IsInner() {
		// If we have a single leaf, wrap it in an inner node
		innerRoot := NewInnerNode()
		rootNodeID := NewRootNodeID()

		// For single leaf, we need to determine which branch it should go in
		if leafNode, ok := node.(LeafNode); ok {
			item := leafNode.Item()
			if item != nil {
				key := item.Key()
				branch := SelectBranch(rootNodeID, key)
				if err := innerRoot.SetChild(int(branch), node); err != nil {
					result.Status = AddNodeInvalid
					result.Bad = 1
					return result
				}
			}
		}
		sm.root = innerRoot
	} else {
		sm.root = node.(*InnerNode)
	}

	result.Status = AddNodeUseful
	result.Good = 1

	// Mark as not full since we just set a new root
	sm.full = false

	// Notify the filter
	if filter != nil {
		filter.GotNode(false, hash, sm.ledgerSeq, nodeData, node.Type())
	}

	return result
}

// FetchRoot initializes the map with a root node from the sync filter
func (sm *SHAMap) FetchRoot(hash [32]byte, filter SyncFilter) bool {
	if filter == nil {
		return false
	}

	nodeData, found := filter.GetNode(hash)
	if !found {
		return false
	}

	result := sm.AddRootNode(hash, nodeData, filter)
	return result.Status == AddNodeUseful
}

// GetNodeFat retrieves a node and optionally its descendants for efficient transmission
func (sm *SHAMap) GetNodeFat(nodeID NodeID, fatLeaves bool, depth uint32) ([]NodeData, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var result []NodeData

	// Find the requested node
	node, err := sm.getNodeUnsafe(nodeID)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, ErrNodeNotFound
	}

	// Serialize the main node
	nodeData, err := node.SerializeForWire()
	if err != nil {
		return nil, err
	}

	result = append(result, NodeData{
		NodeID: nodeID,
		Data:   nodeData,
	})

	// If depth > 0, include children
	if depth > 0 && node.IsInner() {
		inner, ok := node.(*InnerNode)
		if !ok {
			return result, nil
		}

		err = sm.addChildrenToFatNode(inner, nodeID, fatLeaves, depth-1, &result)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

// GetMissingNodesFiltered returns missing nodes with filter support
func (sm *SHAMap) GetMissingNodesFiltered(maxNodes int, filter SyncFilter) []MissingNodeRequest {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var requests []MissingNodeRequest
	var missingNodes []MissingNode

	// Get missing nodes using existing functionality
	_ = sm.walkMapUnsafe(sm.root, NewRootNodeID(), make(map[[32]byte]bool), &missingNodes)

	// Convert to requests and limit count
	for i, missing := range missingNodes {
		if i >= maxNodes {
			break
		}

		// Check if filter can provide this node
		if filter != nil {
			if nodeData, found := filter.GetNode(missing.Hash); found {
				// Add the node using the retrieved data
				sm.addKnownNodeByHashUnsafe(missing.NodeID, missing.Hash, nodeData, filter)
				continue
			}
		}

		requests = append(requests, MissingNodeRequest{
			NodeID: missing.NodeID,
			Hash:   missing.Hash,
		})
	}

	return requests
}

// HasInnerNode checks if a specific inner node exists with the expected hash
func (sm *SHAMap) HasInnerNode(nodeID NodeID, expectedHash [32]byte) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	node, err := sm.getNodeUnsafe(nodeID)
	if err != nil || node == nil {
		return false
	}

	return node.IsInner() && node.Hash() == expectedHash
}

// HasLeafNode checks if a leaf node exists for the given key with the expected hash
func (sm *SHAMap) HasLeafNode(key [32]byte, expectedHash [32]byte) bool {
	_, found, err := sm.Get(key)
	if err != nil || !found {
		return false
	}

	// Find the leaf node containing this item
	leaf, err := sm.walkToKey(key, nil)
	if err != nil || leaf == nil {
		return false
	}

	return leaf.IsLeaf() && leaf.Hash() == expectedHash
}

// ============================================================================
// Helper methods for sync operations
// ============================================================================

// getExpectedHashUnsafe returns the expected hash for a node at the given position
func (sm *SHAMap) getExpectedHashUnsafe(nodeID NodeID) [32]byte {
	if nodeID.IsRoot() {
		return sm.root.Hash()
	}

	// Navigate to parent and get the expected child hash
	parentID := nodeID.Parent()
	parentNode, err := sm.getNodeUnsafe(parentID)
	if err != nil || parentNode == nil || !parentNode.IsInner() {
		return [32]byte{}
	}

	inner := parentNode.(*InnerNode)
	branch := nodeID.GetBranch()
	hash, exists := inner.GetChildHash(int(branch))
	if !exists {
		return [32]byte{}
	}

	return hash
}

// addKnownNodeUnsafe adds a node to the tree at the specified position
func (sm *SHAMap) addKnownNodeUnsafe(nodeID NodeID, node Node) (bool, error) {
	if nodeID.IsRoot() {
		// Setting root node
		if node.IsInner() {
			sm.root = node.(*InnerNode)
			return true, nil
		}
		return false, ErrInvalidRoot
	}

	// Navigate to parent
	parentID := nodeID.Parent()
	parentNode, err := sm.getNodeUnsafe(parentID)
	if err != nil {
		return false, err
	}
	if parentNode == nil || !parentNode.IsInner() {
		return false, ErrInvalidNodeID
	}

	inner := parentNode.(*InnerNode)
	branch := int(nodeID.GetBranch())

	// Check if already exists
	existing, _ := inner.Child(branch)
	if existing != nil && existing.Hash() == node.Hash() {
		return false, nil // Duplicate
	}

	// Set the child
	return true, inner.SetChild(branch, node)
}

// addKnownNodeByHashUnsafe adds a node by deserializing the provided data
func (sm *SHAMap) addKnownNodeByHashUnsafe(nodeID NodeID, expectedHash [32]byte, nodeData []byte, filter SyncFilter) error {
	node, err := DeserializeNodeFromWire(nodeData)
	if err != nil {
		return err
	}

	if node.Hash() != expectedHash {
		return ErrNodeMismatch
	}

	_, err = sm.addKnownNodeUnsafe(nodeID, node)
	if err != nil {
		return err
	}

	// Notify filter
	if filter != nil {
		filter.GotNode(true, expectedHash, sm.ledgerSeq, nodeData, node.Type())
	}

	return nil
}

// addChildrenToFatNode recursively adds children to the fat node response
func (sm *SHAMap) addChildrenToFatNode(inner *InnerNode, nodeID NodeID, fatLeaves bool, depth uint32, result *[]NodeData) error {
	for i := 0; i < BranchFactor; i++ {
		if inner.IsEmptyBranch(i) {
			continue
		}

		child, err := inner.Child(i)
		if err != nil {
			continue
		}
		if child == nil {
			continue
		}

		childNodeID, err := nodeID.ChildNodeID(uint8(i))
		if err != nil {
			continue
		}

		// Include this child if it's an inner node, or if it's a leaf and fatLeaves is true
		if child.IsInner() || fatLeaves {
			childData, err := child.SerializeForWire()
			if err != nil {
				continue
			}

			*result = append(*result, NodeData{
				NodeID: childNodeID,
				Data:   childData,
			})

			// Recurse if there's more depth and this is an inner node
			if depth > 0 && child.IsInner() {
				childInner := child.(*InnerNode)
				err = sm.addChildrenToFatNode(childInner, childNodeID, fatLeaves, depth-1, result)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// getNodeUnsafe retrieves a node by its NodeID (assumes lock is held)
func (sm *SHAMap) getNodeUnsafe(nodeID NodeID) (Node, error) {
	if nodeID.IsRoot() {
		return sm.root, nil
	}

	// Navigate from root to the target node
	current := Node(sm.root)
	depth := uint8(0)

	for depth < nodeID.GetDepth() {
		if !current.IsInner() {
			return nil, ErrNodeNotFound
		}

		inner := current.(*InnerNode)
		branch := nodeID.GetBranchAtDepth(depth)

		child, err := inner.Child(int(branch))
		if err != nil {
			return nil, err
		}
		if child == nil {
			return nil, ErrNodeNotFound
		}

		current = child
		depth++
	}

	return current, nil
}
