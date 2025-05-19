package shamap

import (
	"bytes"
	"testing"
)

// TestItem implements a simple item for testing
type TestItem struct {
	key  [32]byte
	data []byte
}

// SHAMapItem interface implementation for TestItem
func (i *TestItem) Key() [32]byte {
	return i.key
}

func (i *TestItem) Data() []byte {
	return i.data
}

func TestNewSHAMap(t *testing.T) {
	// Create a real Hasher from the shamap package
	hasher := &Sha512HalfHasher{} // Assuming this is the concrete implementation in your package

	// Create a new SHAMap
	m := NewSHAMap(0, hasher)

	if m == nil {
		t.Fatal("NewSHAMap returned nil")
	}

	if m.state != Modifying {
		t.Errorf("Expected state to be Modifying, got %v", m.state)
	}

	if m.mapType != 0 {
		t.Errorf("Expected mapType to be 0, got %v", m.mapType)
	}

	if m.cowID != 1 {
		t.Errorf("Expected cowID to be 1, got %v", m.cowID)
	}

	if m.full != false {
		t.Errorf("Expected full to be false, got %v", m.full)
	}

	if m.hasher != hasher {
		t.Errorf("Expected hasher to be set correctly")
	}

	if m.root == nil {
		t.Errorf("Expected root to be initialized")
	}

	// Check that root is an inner node
	innerNode, ok := m.root.(*InnerNode)
	if !ok {
		t.Errorf("Expected root to be a SHAMapInnerNode")
	} else if !innerNode.dirty {
		t.Errorf("Expected root node to be dirty")
	}
}

/*func TestRootHash(t *testing.T) {
	hasher := &Sha512HalfHasher{} // Use the real hasher from your package
	m := NewSHAMap(0, hasher)

	// Initially, the root hash should be empty or a specific value based on an empty tree
	//initialHash := m.RootHash()

	// Create a second identical map and verify hashes match
	m2 := NewSHAMap(0, hasher)
	//	a := m2.RootHash()
	/*if !bytes.Equal(initialHash, a) {
		t.Errorf("Root hashes of identical empty maps don't match")
	}

	// Add some items to the map (assuming you have an Add method)
	// This part needs to be adapted to match your actual implementation
	/*
		item1 := &TestItem{
			key:  [32]byte{1, 2, 3}, // Some test key
			data: []byte("test data 1"),
		}
		m.Add(item1) // Replace with your actual method for adding items

		// After modification, hash should change
		modifiedHash := m.RootHash()
		if bytes.Equal(initialHash[:], modifiedHash[:]) {
			t.Errorf("Root hash didn't change after modification")
		}
}
*/

func TestCalculateHash(t *testing.T) {
	hasher := &Sha512HalfHasher{} // Use the real hasher from your package
	m := NewSHAMap(0, hasher)

	// Test hash calculation for nil node
	nilHash := m.calculateHash(nil)
	emptyHash := [32]byte{}
	if !bytes.Equal(nilHash[:], emptyHash[:]) {
		t.Errorf("Expected empty hash for nil node")
	}

	// Test hash calculation for leaf node with nil item
	leafNode := &LeafNode{
		item: nil,
	}
	leafHash := m.calculateHash(leafNode)
	if !bytes.Equal(leafHash[:], emptyHash[:]) {
		t.Errorf("Expected empty hash for leaf node with nil item")
	}

	// Test hash calculation for leaf node with item
	testItem := &Item{
		Key:  [32]byte{1, 2, 3}, // Some test key
		Data: []byte("test data"),
	}
	leafNodeWithItem := &LeafNode{
		item: testItem,
	}
	leafHashWithItem := m.calculateHash(leafNodeWithItem)
	println(len(leafHashWithItem))

	// The expected hash would be the hash of the item's key + data
	//expectedData := append(testItem.Key()[:], testItem.Data()...)
	//expectedHash := hasher.Hash(expectedData)

	/*if !bytes.Equal(leafHashWithItem[:], expectedHash[:]) {
		t.Errorf("Leaf node hash calculation incorrect")
	}*/

	// Test inner node hash calculation
	// This would need more detailed implementation based on your specific inner node logic
}
