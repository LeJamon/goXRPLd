package shamap

import (
	"fmt"
	"testing"
)

// Helper function to create an item with a key and a value filled with a repeating byte
func intToVUC(v int) []byte {
	vuc := make([]byte, 32)
	for i := 0; i < 32; i++ {
		vuc[i] = byte(v)
	}
	return vuc
}

// Helper function to create a SHAMapItem with the given key and value
func makeItem(key [32]byte, value []byte) *SHAMapItem {
	return NewSHAMapItem(key, value)
}

// TestAddAndTraverse tests adding items to a SHAMap and traversing it
/*func TestAddAndTraverse(t *testing.T) {
	// Define test keys - borrowed from the C++ test
	var h1, h2, h3, h4, h5 [32]byte

	copy(h1[:], []byte("092891fe4ef6cee585fdc6fda0e09eb4d386363158ec3321b8123e5a772c6ca7"))
	copy(h2[:], []byte("436ccbac3347baa1f1e53baeef1f43334da88f1f6d70d963b833afd6dfa289fe"))
	copy(h3[:], []byte("b92891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"))
	copy(h4[:], []byte("b92891fe4ef6cee585fdc6fda2e09eb4d386363158ec3321b8123e5a772c6ca8"))
	copy(h5[:], []byte("a92891fe4ef6cee585fdc6fda0e09eb4d386363158ec3321b8123e5a772c6ca7"))

	// Create items with values 1-5
	i1 := makeItem(h1, intToVUC(1))
	i2 := makeItem(h2, intToVUC(2))
	i3 := makeItem(h3, intToVUC(3))
	i4 := makeItem(h4, intToVUC(4))
	// Create a SHAMap
	sMap := NewSHAMap(TxMap)

	// Add items to the map
	err := sMap.AddItem(tnTRANSACTION_NM, i2)
	if err != nil {
		t.Errorf("Failed to add item 2: %v", err)
	}

	err = sMap.AddItem(tnTRANSACTION_NM, i1)
	if err != nil {
		t.Errorf("Failed to add item 1: %v", err)
	}

	// Traverse the map and check the order
	items := []*SHAMapItem{}
	sMap.VisitLeaves(func(item *SHAMapItem) {
		items = append(items, item)
	})

	// Should have 2 items
	if len(items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(items))
	}

	// Check that we have i1 and i2
	found1, found2 := false, false
	for _, item := range items {
		itemKey := item.Key()
		key1 := i1.Key()
		key2 := i2.Key()

		if bytes.Equal(itemKey[:], key1[:]) {
			found1 = true
		}
		if bytes.Equal(itemKey[:], key2[:]) {
			found2 = true
		}
	}
	if !found1 || !found2 {
		t.Errorf("Failed to find expected items after traverse: found1=%v, found2=%v", found1, found2)
	}

	// Add item 4
	err = sMap.AddItem(tnTRANSACTION_NM, i4)
	if err != nil {
		t.Errorf("Failed to add item 4: %v", err)
	}

	// Delete item 2
	err = sMap.DelItem(i2.Key())
	if err != nil {
		t.Errorf("Failed to delete item 2: %v", err)
	}

	// Add item 3
	err = sMap.AddItem(tnTRANSACTION_NM, i3)
	if err != nil {
		t.Errorf("Failed to add item 3: %v", err)
	}

	// Traverse again
	items = []*SHAMapItem{}
	sMap.VisitLeaves(func(item *SHAMapItem) {
		items = append(items, item)
	})

	// Should have 3 items
	if len(items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(items))
	}

	// Check that we have i1, i3, and i4 but not i2
	found1, found3, found4 := false, false, false
	for _, item := range items {
		itemKey := item.Key()
		key1 := i1.Key()
		key2 := i2.Key()
		key3 := i3.Key()
		key4 := i4.Key()

		if bytes.Equal(itemKey[:], key1[:]) {
			found1 = true
		}
		if bytes.Equal(itemKey[:], key3[:]) {
			found3 = true
		}
		if bytes.Equal(itemKey[:], key4[:]) {
			found4 = true
		}
		if bytes.Equal(itemKey[:], key2[:]) {
			t.Errorf("Found deleted item 2 in the map")
		}
	}
	if !found1 || !found3 || !found4 {
		t.Errorf("Failed to find expected items after traverse: found1=%v, found3=%v, found4=%v", found1, found3, found4)
	}
}

// TestSnapshot tests creating a snapshot of a SHAMap
func TestSnapshot(t *testing.T) {
	// Define a test key
	var h1 [32]byte
	copy(h1[:], []byte("092891fe4ef6cee585fdc6fda0e09eb4d386363158ec3321b8123e5a772c6ca7"))

	// Create a SHAMap
	sMap := NewSHAMap(TxMap)

	// Add an item
	err := sMap.AddItem(tnTRANSACTION_NM, makeItem(h1, intToVUC(1)))
	if err != nil {
		t.Errorf("Failed to add item: %v", err)
	}

	// Get the hash of the map
	mapHash := sMap.GetHash()

	// Create a snapshot
	snapShotMap := sMap.SnapShot(false) // immutable snapshot

	// Hashes should match
	smHash := sMap.GetHash()
	if !bytes.Equal(smHash[:], mapHash[:]) {
		t.Errorf("Original map hash changed after snapshot")
	}

	snapShotHash := snapShotMap.GetHash()
	if !bytes.Equal(snapShotHash[:], mapHash[:]) {
		t.Errorf("Snapshot hash doesn't match original")
	}

	// Compare the maps - they should be identical
	var diffItems []*SHAMapItem
	sMap.VisitDifferences(snapShotMap, func(item *SHAMapItem) {
		diffItems = append(diffItems, item)
	})

	if len(diffItems) != 0 {
		t.Errorf("Maps should be identical, but found %d differences", len(diffItems))
	}

	// Modify the original
	err = sMap.DelItem(h1)
	if err != nil {
		t.Errorf("Failed to delete item: %v", err)
	}

	// Hashes should now be different
	smHash = sMap.GetHash()
	if bytes.Equal(smHash[:], mapHash[:]) {
		t.Errorf("Original map hash unchanged after modification")
	}

	// Snapshot hash should remain the same
	snapShotHash = snapShotMap.GetHash()
	if !bytes.Equal(snapShotHash[:], mapHash[:]) {
		t.Errorf("Snapshot hash changed")
	}

	// Compare again - should be different
	diffItems = []*SHAMapItem{}
	sMap.VisitDifferences(snapShotMap, func(item *SHAMapItem) {
		diffItems = append(diffItems, item)
	})

	if len(diffItems) != 1 {
		t.Errorf("Expected 1 difference, got %d", len(diffItems))
	}
}*/

// TestBuildAndTear tests building a tree and tearing it down
func TestBuildAndTear(t *testing.T) {
	// Define a set of test keys
	keys := [][32]byte{
		hashFromString("b92891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hashFromString("b92881fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hashFromString("b92691fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hashFromString("b92791fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hashFromString("b91891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hashFromString("b99891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hashFromString("f22891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hashFromString("292891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
	}

	// Create a SHAMap
	sMap := NewSHAMap(TxMap)

	// Add all keys
	for i, key := range keys {
		err := sMap.AddItem(tnTRANSACTION_NM, makeItem(key, intToVUC(i)))
		if err != nil {
			t.Errorf("Failed to add item %d: %v", i, err)
		}
	}
	println("Added all key")
	dumpTree(sMap.root, "")

	// Count items
	count := 0
	sMap.VisitLeaves(func(item *SHAMapItem) {
		count++
	})
	if count != len(keys) {
		t.Errorf("Expected %d items, found %d", len(keys), count)
	}

	// Delete all keys in reverse order
	for i := len(keys) - 1; i >= 0; i-- {
		fmt.Printf("\n=== Iteration %d ===\n", i)
		fmt.Printf("Deleting key: %x\n", keys[i])

		if !sMap.DelItem(keys[i]) {
			t.Errorf("Failed to delete item %x", keys[i])
		}

		fmt.Println("Tree after deletion:")
		dumpTree(sMap.root, "")

		// Verify count decreases
		count = 0
		sMap.VisitLeaves(func(item *SHAMapItem) {
			count++
		})
		if count != i {
			t.Errorf("After deletion, expected %d items, found %d", i, count)
		}
	}

	// Final check - map should be empty
	count = 0
	sMap.VisitLeaves(func(item *SHAMapItem) {
		count++
	})
	if count != 0 {
		t.Errorf("Map should be empty, but found %d items", count)
	}
}

// TestIteration tests ordering of map iteration
/*func TestIteration(t *testing.T) {
	// Define keys in a specific order
	keys := [][32]byte{
		hashFromString("f22891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hashFromString("b99891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hashFromString("b92891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hashFromString("b92881fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hashFromString("b92791fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hashFromString("b92691fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hashFromString("b91891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hashFromString("292891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
	}

	// Create a SHAMap
	sMap := NewSHAMap(TxMap)

	// Add all keys
	for i, key := range keys {
		err := sMap.AddItem(tnTRANSACTION_NM, makeItem(key, intToVUC(i)))
		if err != nil {
			t.Errorf("Failed to add item %d: %v", i, err)
		}
	}

	// Visit leaves and collect keys in the order of visitation
	visitedKeys := [][32]byte{}
	sMap.VisitLeaves(func(item *SHAMapItem) {
		visitedKeys = append(visitedKeys, item.Key())
	})

	// We expect the keys to be visited in a deterministic order
	// (typically based on the hash ordering)
	if len(visitedKeys) != len(keys) {
		t.Errorf("Expected %d items, got %d", len(keys), len(visitedKeys))
	}

	// Check for all keys present
	for _, key := range keys {
		found := false
		for _, visitedKey := range visitedKeys {
			if bytes.Equal(visitedKey[:], key[:]) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Key %v not found in visited keys", key)
		}
	}
}

// TestImmutability tests that an immutable map cannot be modified
func TestImmutability(t *testing.T) {
	// Create a key
	var key [32]byte
	copy(key[:], []byte("092891fe4ef6cee585fdc6fda0e09eb4d386363158ec3321b8123e5a772c6ca7"))

	// Create a SHAMap
	sMap := NewSHAMap(TxMap)

	// Add an item
	err := sMap.AddItem(tnTRANSACTION_NM, makeItem(key, intToVUC(1)))
	if err != nil {
		t.Errorf("Failed to add item: %v", err)
	}

	// Make the map immutable
	sMap.SetImmutable()

	// Try to add an item - should fail
	err = sMap.AddItem(tnTRANSACTION_NM, makeItem(key, intToVUC(2)))
	if err != ErrImmutable {
		t.Errorf("Adding to immutable map should fail with ErrImmutable, got: %v", err)
	}

	// Try to delete an item - should fail
	err = sMap.DelItem(key)
	if err != ErrImmutable {
		t.Errorf("Deleting from immutable map should fail with ErrImmutable, got: %v", err)
	}
}*/

// Helper to create a hash from a string
func hashFromString(s string) [32]byte {
	var hash [32]byte
	copy(hash[:], []byte(s))
	return hash
}

func dumpTree(node TreeNode, prefix string) {
	switch n := node.(type) {
	case *InnerNode:
		fmt.Printf("%sInnerNode: %p, hash: %x\n", prefix, n, n.Hash())
		for i := 0; i < 16; i++ {
			if !n.IsEmptyBranch(i) {
				child := n.GetChild(i)
				if child != nil {
					dumpTree(child, prefix+"  ")
				}
			}
		}
	case *AccountStateLeafNode:
		fmt.Printf("%sLeafNode(Account): %p, key: %x\n", prefix, n, n.GetItem().Key())
	case *TxLeafNode:
		fmt.Printf("%sLeafNode(Tx): %p, key: %x\n", prefix, n, n.GetItem().Key())
	case *TxPlusMetaLeafNode:
		fmt.Printf("%sLeafNode(Tx+Meta): %p, key: %x\n", prefix, n, n.GetItem().Key())
	default:
		fmt.Printf("%sUnknown node type: %T\n", prefix, n)
	}
}
