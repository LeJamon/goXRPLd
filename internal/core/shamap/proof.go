package shamap

import (
	"bytes"
	"errors"
	"fmt"
)

// ProofPath represents a proof path for a key in the SHAMap
type ProofPath struct {
	Key   [32]byte  // The key being proven
	Path  [][]byte  // The proof path (leaf to root order)
	Found bool      // Whether the key was found
}

// GetProofPath generates a proof path for the given key
// Returns a ProofPath with Found=true if the key exists, Found=false otherwise
func (sm *SHAMap) GetProofPath(key [32]byte) (*ProofPath, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Use walkToKey to find the leaf and build the path
	stack := NewNodeStack()
	leaf, err := sm.walkToKey(key, stack)
	if err != nil {
		return nil, fmt.Errorf("failed to walk to key: %w", err)
	}


	// If no leaf found, return unfound proof
	if leaf == nil {
		return &ProofPath{
			Key:   key,
			Path:  nil,
			Found: false,
		}, nil
	}

	// Verify this is actually a leaf node with our target key
	if !leaf.IsLeaf() {
		return nil, fmt.Errorf("walkToKey returned non-leaf node")
	}

	leafNode, ok := leaf.(LeafNode)
	if !ok {
		return nil, fmt.Errorf("leaf node does not implement LeafNode interface")
	}

	item := leafNode.Item()
	if item == nil {
		return nil, fmt.Errorf("leaf node has nil item")
	}

	leafKey := item.Key()
	if !bytes.Equal(leafKey[:], key[:]) {
		// Key not found - this is a valid case where we walked to a different leaf
		return &ProofPath{
			Key:   key,
			Path:  nil,
			Found: false,
		}, nil
	}

	// Build proof path from the stack (leaf to root order)
	path := make([][]byte, 0, stack.Len())
	
	// The stack contains nodes from root to leaf (walkToKey pushes root first, leaf last)
	// We need to build the path in leaf-to-root order, which means we pop from the stack directly
	for !stack.IsEmpty() {
		node, _, ok := stack.Pop()
		if !ok {
			return nil, fmt.Errorf("stack corruption during proof building")
		}
		
		serialized, err := node.SerializeForWire()
		if err != nil {
			return nil, fmt.Errorf("failed to serialize node in proof path: %w", err)
		}
		path = append(path, serialized)
	}

	return &ProofPath{
		Key:   key,
		Path:  path,
		Found: true,
	}, nil
}

// VerifyProofPath verifies a proof path against a root hash
// The proof path should be in leaf-to-root order as returned by GetProofPath
func VerifyProofPath(rootHash [32]byte, key [32]byte, path [][]byte) error {
	if len(path) == 0 {
		return errors.New("empty proof path")
	}

	if len(path) > 65 {
		return errors.New("proof path too long")
	}

	// Start with the root hash
	currentHash := rootHash

	// Process path from root to leaf (reverse iteration since path is leaf-to-root)
	// This matches the rippled implementation which uses reverse iterators
	for i := len(path) - 1; i >= 0; i-- {
		nodeData := path[i]

		// Deserialize the node from wire format
		node, err := DeserializeNodeFromWire(nodeData)
		if err != nil {
			return fmt.Errorf("failed to deserialize node at position %d: %w", i, err)
		}

		// Update the node's hash - this recalculates it from the node's contents
		if err := node.UpdateHash(); err != nil {
			return fmt.Errorf("failed to update hash for node at position %d: %w", i, err)
		}

		// Verify that this node's hash matches what we expect
		nodeHash := node.Hash()
		if nodeHash != currentHash {
			return fmt.Errorf("hash mismatch at position %d: expected %x, got %x", 
				i, currentHash, nodeHash)
		}

		// Calculate the depth of this node in the tree based on distance from root
		depth := len(path) - 1 - i

		if node.IsInner() {
			// This is an inner node - follow the branch toward our key
			innerNode, ok := node.(*InnerNode)
			if !ok {
				return fmt.Errorf("node claims to be inner but type assertion failed at position %d", i)
			}

			// Create node ID for this depth and key to determine which branch to follow
			nodeID, err := CreateNodeID(uint8(depth), key)
			if err != nil {
				return fmt.Errorf("failed to create node ID at depth %d: %w", depth, err)
			}

			// Determine which branch to follow
			branch := SelectBranch(nodeID, key)

			// Check if this branch is empty
			if innerNode.IsEmptyBranch(int(branch)) {
				return fmt.Errorf("required branch %d is empty at position %d", branch, i)
			}

			// Get the hash of the child we should follow
			childHash, err := innerNode.ChildHash(int(branch))
			if err != nil {
				return fmt.Errorf("failed to get child hash for branch %d at position %d: %w", branch, i, err)
			}

			// Move to the child hash for the next iteration
			currentHash = childHash

		} else if node.IsLeaf() {
			// This should be the final leaf node (first in path, last in verification)
			if i != 0 {
				return fmt.Errorf("leaf node found before end of path at position %d", i)
			}

			// Verify this leaf contains our target key
			leafNode, ok := node.(LeafNode)
			if !ok {
				return fmt.Errorf("node claims to be leaf but doesn't implement LeafNode interface")
			}

			item := leafNode.Item()
			if item == nil {
				return errors.New("leaf node has nil item")
			}

			leafKey := item.Key()
			if !bytes.Equal(leafKey[:], key[:]) {
				return fmt.Errorf("leaf key doesn't match target key: expected %x, got %x", key, leafKey)
			}

			// Successfully reached and verified the target leaf
			return nil

		} else {
			return fmt.Errorf("node is neither inner nor leaf at position %d", i)
		}
	}

	return errors.New("path verification failed - did not reach leaf")
}

// GetProofPathForMissingKey generates a proof path demonstrating that a key does not exist
// This is useful for proving non-inclusion in the tree
func (sm *SHAMap) GetProofPathForMissingKey(key [32]byte) (*ProofPath, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Walk to where the key would be if it existed
	stack := NewNodeStack()
	leaf, err := sm.walkToKey(key, stack)
	if err != nil {
		return nil, fmt.Errorf("failed to walk to key: %w", err)
	}

	// If we found a leaf, check if it's the target key
	if leaf != nil && leaf.IsLeaf() {
		leafNode, ok := leaf.(LeafNode)
		if ok {
			item := leafNode.Item()
			if item != nil {
				leafKey := item.Key()
				if bytes.Equal(leafKey[:], key[:]) {
					// Key exists, so this is not a missing key proof
					return &ProofPath{
						Key:   key,
						Path:  nil,
						Found: true,
					}, nil
				}
			}
		}
	}

	// Key doesn't exist - build proof path showing what we found instead
	if stack.IsEmpty() {
		// Empty tree
		return &ProofPath{
			Key:   key,
			Path:  nil,
			Found: false,
		}, nil
	}

	// Build proof path from the stack to show the path we took
	path := make([][]byte, 0, stack.Len())
	
	// The stack contains nodes from root to leaf (walkToKey pushes root first, leaf last)
	// We need to build the path in leaf-to-root order, which means we pop from the stack directly
	for !stack.IsEmpty() {
		node, _, ok := stack.Pop()
		if !ok {
			return nil, fmt.Errorf("stack corruption during proof building")
		}
		
		serialized, err := node.SerializeForWire()
		if err != nil {
			return nil, fmt.Errorf("failed to serialize node in proof path: %w", err)
		}
		path = append(path, serialized)
	}

	return &ProofPath{
		Key:   key,
		Path:  path,
		Found: false,
	}, nil
}

// VerifyProofPathForMissingKey verifies that a proof path correctly demonstrates
// that a key does not exist in the tree
func VerifyProofPathForMissingKey(rootHash [32]byte, key [32]byte, path [][]byte) error {
	if len(path) == 0 {
		// Empty path can represent an empty tree
		return nil
	}

	// First verify the path is structurally valid
	if err := VerifyProofPath(rootHash, key, path); err != nil {
		// If the path verification fails, it might be because we hit an empty branch
		// or reached a leaf with a different key - both are valid for non-inclusion proofs
		
		// Try to verify that we can walk the path but end up somewhere else
		return verifyNonInclusionPath(rootHash, key, path)
	}

	// If VerifyProofPath succeeded, it means we found the key, so this is not a valid
	// non-inclusion proof
	return errors.New("proof path leads to the target key, not a valid non-inclusion proof")
}

// verifyNonInclusionPath verifies that a path demonstrates non-inclusion by showing
// that we either hit an empty branch or reached a different leaf
func verifyNonInclusionPath(rootHash [32]byte, key [32]byte, path [][]byte) error {
	if len(path) == 0 {
		return errors.New("empty proof path for non-inclusion")
	}

	if len(path) > 65 {
		return errors.New("proof path too long")
	}

	currentHash := rootHash

	// Process path from root to leaf (reverse iteration since path is leaf-to-root)
	for i := len(path) - 1; i >= 0; i-- {
		nodeData := path[i]

		// Deserialize the node from wire format
		node, err := DeserializeNodeFromWire(nodeData)
		if err != nil {
			return fmt.Errorf("failed to deserialize node at position %d: %w", i, err)
		}

		// Update the node's hash
		if err := node.UpdateHash(); err != nil {
			return fmt.Errorf("failed to update hash for node at position %d: %w", i, err)
		}

		// Verify that this node's hash matches what we expect
		nodeHash := node.Hash()
		if nodeHash != currentHash {
			return fmt.Errorf("hash mismatch at position %d", i)
		}

		depth := len(path) - 1 - i

		if node.IsInner() {
			innerNode, ok := node.(*InnerNode)
			if !ok {
				return fmt.Errorf("node claims to be inner but type assertion failed at position %d", i)
			}

			// Create node ID for this depth and key
			nodeID, err := CreateNodeID(uint8(depth), key)
			if err != nil {
				return fmt.Errorf("failed to create node ID at depth %d: %w", depth, err)
			}

			branch := SelectBranch(nodeID, key)

			// Check if this branch is empty - this would prove non-inclusion
			if innerNode.IsEmptyBranch(int(branch)) {
				// This proves the key doesn't exist - we hit an empty branch
				return nil
			}

			// Get the hash of the child
			childHash, err := innerNode.ChildHash(int(branch))
			if err != nil {
				return fmt.Errorf("failed to get child hash for branch %d at position %d: %w", branch, i, err)
			}

			currentHash = childHash

		} else if node.IsLeaf() {
			// We reached a leaf - it should have a different key to prove non-inclusion
			if i != 0 {
				return fmt.Errorf("leaf node found before end of path at position %d", i)
			}

			leafNode, ok := node.(LeafNode)
			if !ok {
				return fmt.Errorf("node claims to be leaf but doesn't implement LeafNode interface")
			}

			item := leafNode.Item()
			if item == nil {
				return errors.New("leaf node has nil item")
			}

			leafKey := item.Key()
			if bytes.Equal(leafKey[:], key[:]) {
				return errors.New("leaf key matches target key - this proves inclusion, not non-inclusion")
			}

			// Reached a leaf with a different key - this proves non-inclusion
			return nil

		} else {
			return fmt.Errorf("node is neither inner nor leaf at position %d", i)
		}
	}

	return errors.New("path verification failed for non-inclusion proof")
}