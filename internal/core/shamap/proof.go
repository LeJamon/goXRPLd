package shamap

import (
	"errors"
	"fmt"
)

type ProofPath struct {
	Key   [32]byte
	Path  [][]byte
	Found bool
}

func (sm *SHAMap) GetProofPath(key [32]byte) (*ProofPath, error) {
	stack := NewNodeStack()
	leaf, err := sm.walkToKey(key, stack)
	if err != nil {
		return nil, err
	}

	if leaf == nil {
		return &ProofPath{Key: key, Found: false}, nil
	}

	// Build and return proof
	return sm.buildProofFromStack(key, stack)
}

func (sm *SHAMap) buildProofFromStack(key [32]byte, stack *NodeStack) (*ProofPath, error) {
	if stack.IsEmpty() {
		return &ProofPath{Key: key, Found: false}, nil
	}

	// Get the leaf node (top of stack) and verify it contains our key
	topNode, _, ok := stack.Top()
	if !ok {
		return nil, errors.New("empty stack")
	}

	// Check if it's a leaf node using the interface
	leafNode, ok := topNode.(LeafNode)
	if !ok {
		return nil, errors.New("top of stack is not a leaf node")
	}

	// Verify this leaf contains the target key
	item := leafNode.Item()
	if item == nil {
		return nil, errors.New("leaf node has nil item")
	}

	leafKey := item.Key()
	if leafKey != key {
		return &ProofPath{Key: key, Found: false}, nil
	}

	// Build proof path by serializing all nodes in the stack
	path := make([][]byte, 0, stack.Len())

	// Iterate through stack entries (leaf to root order)
	for i := 0; i < stack.Len(); i++ {
		node := stack.entries[i].node

		// Serialize the node for wire transmission
		serialized, err := node.SerializeForWire()
		if err != nil {
			return nil, fmt.Errorf("failed to serialize node at depth %d: %w", i, err)
		}

		path = append(path, serialized)
	}

	return &ProofPath{
		Key:   key,
		Path:  path,
		Found: true,
	}, nil
}

/*func VerifyProofPath(rootHash [32]byte, key [32]byte, path [][]byte) error {
	if len(path) == 0 {
		return errors.New("empty proof path")
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

		// Update the node's hash and verify it matches expected
		if err := node.UpdateHash(); err != nil {
			return fmt.Errorf("failed to update hash for node at position %d: %w", i, err)
		}

		nodeHash := node.Hash()
		if nodeHash != currentHash {
			return fmt.Errorf("hash mismatch at position %d", i)
		}

		depth := len(path) - 1 - i

		if node.IsInner() {
			// This should be an inner node, follow the branch toward our key
			innerNode, ok := node.(*InnerNode)
			if !ok {
				return fmt.Errorf("node claims to be inner but type assertion failed at position %d", i)
			}

			// Calculate which branch to follow
			nodeID := CreateNodeIDAtDepth(depth, key)
			branch := SelectBranch(nodeID, key)

			// Get the hash of the child we should follow
			childHash, exists := innerNode.GetChildHash(int(branch))
			if !exists {
				return fmt.Errorf("required branch %d is empty at position %d", branch, i)
			}

			currentHash = childHash
		} else if node.IsLeaf() {
			// This should be the final leaf node
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
			if leafKey != key {
				return errors.New("leaf key doesn't match target key")
			}

			// Successfully reached the target leaf
			return nil
		} else {
			return fmt.Errorf("node is neither inner nor leaf at position %d", i)
		}
	}

	return errors.New("path verification failed - did not reach leaf")
}
*/
