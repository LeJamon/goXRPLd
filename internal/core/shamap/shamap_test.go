package shamap

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"
)

// Helper function to create a byte slice filled with a repeating byte
func intToBytes(v int) []byte {
	data := make([]byte, 32)
	for i := 0; i < 32; i++ {
		data[i] = byte(v)
	}
	return data
}

// Helper function to create a SHAMapItem with the given key and value
func makeItem(key [32]byte, value []byte) *Item {
	return NewItem(key, value)
}

// Parse hex string to actual bytes (like rippled does)
func hexToHash(s string) [32]byte {
	var hash [32]byte
	decoded, err := hex.DecodeString(s)
	if err != nil {
		panic(fmt.Sprintf("Invalid hex string: %s - %v", s, err))
	}
	if len(decoded) != 32 {
		panic(fmt.Sprintf("Hex string is not 32 bytes: %s (got %d bytes)", s, len(decoded)))
	}
	copy(hash[:], decoded)
	return hash
}

// TestAddAndTraverse tests adding items to a SHAMap and traversing it
func TestAddAndTraverse(t *testing.T) {
	// Define test keys - same as rippled C++ test
	h1 := hexToHash("092891fe4ef6cee585fdc6fda0e09eb4d386363158ec3321b8123e5a772c6ca7")
	h2 := hexToHash("436ccbac3347baa1f1e53baeef1f43334da88f1f6d70d963b833afd6dfa289fe")
	h3 := hexToHash("b92891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8")
	h4 := hexToHash("b92891fe4ef6cee585fdc6fda2e09eb4d386363158ec3321b8123e5a772c6ca8")

	// Create items with values 1-4
	i1 := makeItem(h1, intToBytes(1))
	i2 := makeItem(h2, intToBytes(2))
	i3 := makeItem(h3, intToBytes(3))
	i4 := makeItem(h4, intToBytes(4))

	// Create a SHAMap
	sMap, err := New(TypeTransaction)
	if err != nil {
		t.Fatalf("Failed to create SHAMap: %v", err)
	}

	// Add items to the map - same order as C++ test
	if err := sMap.PutItem(i2); err != nil {
		t.Fatalf("Failed to add item 2: %v", err)
	}

	if err := sMap.PutItem(i1); err != nil {
		t.Fatalf("Failed to add item 1: %v", err)
	}

	// Verify we can retrieve the items
	retrievedI1, found, err := sMap.Get(h1)
	if err != nil {
		t.Fatalf("Error getting item 1: %v", err)
	}
	if !found {
		t.Error("Item 1 not found")
	}
	if !bytes.Equal(retrievedI1.Data(), i1.Data()) {
		t.Error("Item 1 data mismatch")
	}

	retrievedI2, found, err := sMap.Get(h2)
	if err != nil {
		t.Fatalf("Error getting item 2: %v", err)
	}
	if !found {
		t.Error("Item 2 not found")
	}
	if !bytes.Equal(retrievedI2.Data(), i2.Data()) {
		t.Error("Item 2 data mismatch")
	}

	// Traverse and verify order
	var items []*Item
	err = sMap.ForEach(func(item *Item) bool {
		items = append(items, item)
		return true // Continue iteration
	})
	if err != nil {
		t.Fatalf("Error during traversal: %v", err)
	}

	// Should have 2 items
	if len(items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(items))
	}

	// Verify we have i1 and i2
	found1, found2 := false, false
	for _, item := range items {
		itemKey := item.Key()
		if itemKey == h1 {
			found1 = true
		}
		if itemKey == h2 {
			found2 = true
		}
	}
	if !found1 || !found2 {
		t.Errorf("Failed to find expected items after traverse: found1=%v, found2=%v", found1, found2)
	}

	// Add item 4
	if err := sMap.PutItem(i4); err != nil {
		t.Fatalf("Failed to add item 4: %v", err)
	}

	// Delete item 2
	if err := sMap.Delete(h2); err != nil {
		t.Fatalf("Failed to delete item 2: %v", err)
	}

	// Verify item 2 is gone
	_, found, err = sMap.Get(h2)
	if err != nil {
		t.Fatalf("Error checking for deleted item: %v", err)
	}
	if found {
		t.Error("Item 2 should have been deleted")
	}

	// Add item 3
	if err := sMap.PutItem(i3); err != nil {
		t.Fatalf("Failed to add item 3: %v", err)
	}

	// Final traverse - should have i1, i3, i4
	items = nil
	err = sMap.ForEach(func(item *Item) bool {
		items = append(items, item)
		return true
	})
	if err != nil {
		t.Fatalf("Error during final traversal: %v", err)
	}

	if len(items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(items))
	}

	// Check that we have i1, i3, and i4 but not i2
	found1, found3, found4 := false, false, false
	for _, item := range items {
		itemKey := item.Key()
		switch itemKey {
		case h1:
			found1 = true
		case h2:
			t.Error("Found deleted item 2 in the map")
		case h3:
			found3 = true
		case h4:
			found4 = true
		}
	}
	if !found1 || !found3 || !found4 {
		t.Errorf("Failed to find expected items: found1=%v, found3=%v, found4=%v", found1, found3, found4)
	}
}

// TestBuildAndTear matches the exact C++ "build/tear" test
func TestBuildAndTear(t *testing.T) {
	// Same keys as rippled C++ test
	keys := [][32]byte{
		hexToHash("b92891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hexToHash("b92881fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hexToHash("b92691fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hexToHash("b92791fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hexToHash("b91891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hexToHash("b99891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hexToHash("f22891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hexToHash("292891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
	}

	// Expected hashes after each addition - same as rippled C++ test
	expectedHashes := [][32]byte{
		hexToHash("B7387CFEA0465759ADC718E8C42B52D2309D179B326E239EB5075C64B6281F7F"),
		hexToHash("FBC195A9592A54AB44010274163CB6BA95F497EC5BA0A8831845467FB2ECE266"),
		hexToHash("4E7D2684B65DFD48937FFB775E20175C43AF0C94066F7D5679F51AE756795B75"),
		hexToHash("7A2F312EB203695FFD164E038E281839EEF06A1B99BFC263F3CECC6C74F93E07"),
		hexToHash("395A6691A372387A703FB0F2C6D2C405DAF307D0817F8F0E207596462B0E3A3E"),
		hexToHash("D044C0A696DE3169CC70AE216A1564D69DE96582865796142CE7D98A84D9DDE4"),
		hexToHash("76DCC77C4027309B5A91AD164083264D70B77B5E43E08AEDA5EBF94361143615"),
		hexToHash("DF4220E93ADC6F5569063A01B4DC79F8DB9553B6A3222ADE23DEA02BBE7230E5"),
	}

	// Create a SHAMap
	sMap, err := New(TypeTransaction)
	if err != nil {
		t.Fatalf("Failed to create SHAMap: %v", err)
	}

	// Verify empty map has zero hash
	emptyHash, err := sMap.Hash()
	if err != nil {
		t.Fatalf("Failed to get empty map hash: %v", err)
	}
	zeroHash := [32]byte{} // All zeros
	if emptyHash != zeroHash {
		t.Errorf("Empty map should have zero hash, got %x", emptyHash)
	}

	// Add all keys and verify hash after each addition
	for k, key := range keys {

		if err := sMap.Put(key, intToBytes(k)); err != nil {
			t.Fatalf("Failed to add item %d: %v", k, err)
		}

		// Verify hash matches expected
		actualHash, err := sMap.Hash()
		if err != nil {
			t.Fatalf("Failed to get hash after adding item %d: %v", k, err)
		}

		if actualHash != expectedHashes[k] {
			t.Errorf("Tree dump after adding item %d (hash mismatch):", k)
			dumpTree(sMap.root, "", false)
			t.Errorf("Hash mismatch after adding item %d: expected %x, got %x",
				k, expectedHashes[k], actualHash)
		}
	}

	// Delete all keys in reverse order and verify hashes
	// Delete all keys in reverse order and verify hashes
	for k := len(keys) - 1; k >= 0; k-- {
		// Verify hash BEFORE deletion matches expected
		actualHash, err := sMap.Hash()
		if err != nil {
			t.Fatalf("Failed to get hash before deleting item %d: %v", k, err)
		}
		if actualHash != expectedHashes[k] {
			t.Errorf("Tree dump after adding item %d (hash mismatch):", k)
			dumpTree(sMap.root, "", false)
			t.Errorf("Hash mismatch after adding item %d: expected %x, got %x",
				k, expectedHashes[k], actualHash)
		}

		if err := sMap.Delete(keys[k]); err != nil {
			t.Fatalf("Failed to delete item %d: %v", k, err)
		}

		// Verify item is actually deleted
		_, found, err := sMap.Get(keys[k])
		if err != nil {
			t.Fatalf("Error checking deleted item %d: %v", k, err)
		}
		if found {
			t.Errorf("Item %d should have been deleted", k)
		}

		// TODO
		//
		//Check invariants if you have that method
		// if err := sMap.Invariants(); err != nil {
		//     t.Fatalf("Invariants check failed after deleting item %d: %v", k, err)
		// }
	}

	// Final check - map should be empty (zero hash)
	finalHash, err := sMap.Hash()
	if err != nil {
		t.Fatalf("Failed to get final hash: %v", err)
	}
	if finalHash != zeroHash {
		t.Errorf("Final map should have zero hash, got %x", finalHash)
	}
}

func TestIteration(t *testing.T) {
	keys := [][32]byte{
		hexToHash("f22891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"), // keys[0]
		hexToHash("b99891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"), // keys[1]
		hexToHash("b92891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"), // keys[2]
		hexToHash("b92881fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"), // keys[3]
		hexToHash("b92791fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"), // keys[4]
		hexToHash("b92691fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"), // keys[5]
		hexToHash("b91891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"), // keys[6]
		hexToHash("292891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"), // keys[7]
	}

	sMap, err := New(TypeTransaction)
	if err != nil {
		t.Fatalf("Failed to create SHAMap: %v", err)
	}

	// Add all keys in order (keys[0] through keys[7])
	for i, key := range keys {
		if err := sMap.Put(key, intToBytes(0)); err != nil {
			t.Fatalf("Failed to add item %d: %v", i, err)
		}
	}

	// Collect iteration order
	var visitedKeys [][32]byte
	err = sMap.ForEach(func(item *Item) bool {
		visitedKeys = append(visitedKeys, item.Key())
		return true
	})
	if err != nil {
		t.Fatalf("Error during iteration: %v", err)
	}

	// Verify we got all keys
	if len(visitedKeys) != len(keys) {
		t.Errorf("Expected exactly %d keys, got %d", len(keys), len(visitedKeys))
		return
	}

	// Check each position matches the expected reverse order
	// C++ test expects: keys[7], keys[6], keys[5], keys[4], keys[3], keys[2], keys[1], keys[0]
	for pos := 0; pos < len(keys); pos++ {
		expectedIndex := len(keys) - 1 - pos // 7, 6, 5, 4, 3, 2, 1, 0
		expectedKey := keys[expectedIndex]

		if visitedKeys[pos] != expectedKey {
			t.Errorf("Iteration position %d: expected keys[%d] (%x), got (%x)",
				pos, expectedIndex, expectedKey[:4], visitedKeys[pos][:4])
		}
	}
}

// TestSnapshot tests creating a snapshot of a SHAMap
func TestSnapshot(t *testing.T) {
	h1 := hexToHash("092891fe4ef6cee585fdc6fda0e09eb4d386363158ec3321b8123e5a772c6ca7")

	sMap, err := New(TypeTransaction)
	if err != nil {
		t.Fatalf("Failed to create SHAMap: %v", err)
	}

	if err := sMap.Put(h1, intToBytes(1)); err != nil {
		t.Fatalf("Failed to add item: %v", err)
	}

	mapHash, err := sMap.Hash()
	if err != nil {
		t.Fatalf("Failed to get map hash: %v", err)
	}

	snapShotMap, err := sMap.Snapshot(false) // immutable snapshot
	if err != nil {
		t.Fatalf("Failed to create snapshot: %v", err)
	}

	// Hashes should match
	smHash, err := sMap.Hash()
	if err != nil {
		t.Fatalf("Failed to get original map hash: %v", err)
	}
	if smHash != mapHash {
		t.Error("Original map hash changed after snapshot")
	}

	snapShotHash, err := snapShotMap.Hash()
	if err != nil {
		t.Fatalf("Failed to get snapshot hash: %v", err)
	}
	if snapShotHash != mapHash {
		t.Error("Snapshot hash doesn't match original")
	}

	// Verify both maps have the same item
	originalItem, found, err := sMap.Get(h1)
	if err != nil {
		t.Fatalf("Error getting item from original: %v", err)
	}
	if !found {
		t.Error("Item not found in original map")
	}

	snapshotItem, found, err := snapShotMap.Get(h1)
	if err != nil {
		t.Fatalf("Error getting item from snapshot: %v", err)
	}
	if !found {
		t.Error("Item not found in snapshot map")
	}

	if !originalItem.Equal(snapshotItem) {
		t.Error("Items should be equal between original and snapshot")
	}

	// Modify original map
	if err := sMap.Delete(h1); err != nil {
		t.Fatalf("Failed to delete item: %v", err)
	}

	// Hashes should now be different
	smHash, err = sMap.Hash()
	if err != nil {
		t.Fatalf("Failed to get modified map hash: %v", err)
	}
	if smHash == mapHash {
		t.Error("Original map hash unchanged after modification")
	}

	// Snapshot hash should remain the same
	snapShotHash, err = snapShotMap.Hash()
	if err != nil {
		t.Fatalf("Failed to get snapshot hash after modification: %v", err)
	}
	if snapShotHash != mapHash {
		t.Error("Snapshot hash changed")
	}

	// Verify snapshot still has the item
	_, found, err = snapShotMap.Get(h1)
	if err != nil {
		t.Fatalf("Error getting item from snapshot after original deletion: %v", err)
	}
	if !found {
		t.Error("Item should still exist in snapshot after deletion from original")
	}
}

// TestImmutability tests that an immutable map cannot be modified
func TestImmutability(t *testing.T) {
	key := hexToHash("092891fe4ef6cee585fdc6fda0e09eb4d386363158ec3321b8123e5a772c6ca7")

	sMap, err := New(TypeTransaction)
	if err != nil {
		t.Fatalf("Failed to create SHAMap: %v", err)
	}

	if err := sMap.Put(key, intToBytes(1)); err != nil {
		t.Fatalf("Failed to add item: %v", err)
	}

	if err := sMap.SetImmutable(); err != nil {
		t.Fatalf("Failed to set immutable: %v", err)
	}

	// Try to add an item - should fail
	err = sMap.Put(key, intToBytes(2))
	if err != ErrImmutable {
		t.Errorf("Adding to immutable map should fail with ErrImmutable, got: %v", err)
	}

	// Try to delete an item - should fail
	err = sMap.Delete(key)
	if err != ErrImmutable {
		t.Errorf("Deleting from immutable map should fail with ErrImmutable, got: %v", err)
	}
}

// TestErrorHandling tests various error conditions
func TestErrorHandling(t *testing.T) {
	sMap, err := New(TypeTransaction)
	if err != nil {
		t.Fatalf("Failed to create SHAMap: %v", err)
	}

	// Test adding nil item
	err = sMap.PutItem(nil)
	if err != ErrNilItem {
		t.Errorf("Expected ErrNilItem, got: %v", err)
	}

	// Test getting non-existent item
	key := hexToHash("092891fe4ef6cee585fdc6fda0e09eb4d386363158ec3321b8123e5a772c6ca7")
	_, found, err := sMap.Get(key)
	if err != nil {
		t.Errorf("Getting non-existent item should not error: %v", err)
	}
	if found {
		t.Error("Should not find non-existent item")
	}

	// Test deleting non-existent item
	err = sMap.Delete(key)
	if err != ErrItemNotFound {
		t.Errorf("Expected ErrItemNotFound, got: %v", err)
	}
}

// TestConcurrency tests concurrent access to the SHAMap
func TestConcurrency(t *testing.T) {
	sMap, err := New(TypeState)
	if err != nil {
		t.Fatalf("Failed to create SHAMap: %v", err)
	}

	// Add some initial data
	for i := 0; i < 10; i++ {
		key := [32]byte{}
		key[0] = byte(i)
		if err := sMap.Put(key, intToBytes(i)); err != nil {
			t.Fatalf("Failed to add initial item %d: %v", i, err)
		}
	}

	// Create immutable snapshot for concurrent reading
	snapshot, err := sMap.Snapshot(false)
	if err != nil {
		t.Fatalf("Failed to create snapshot: %v", err)
	}

	// Test concurrent reads on snapshot (should be safe)
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()

			key := [32]byte{}
			key[0] = byte(id)

			item, found, err := snapshot.Get(key)
			if err != nil {
				t.Errorf("Concurrent read %d failed: %v", id, err)
				return
			}
			if !found {
				t.Errorf("Concurrent read %d: item not found", id)
				return
			}
			if !bytes.Equal(item.Data(), intToBytes(id)) {
				t.Errorf("Concurrent read %d: data mismatch", id)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

// Benchmarks

func BenchmarkPut(b *testing.B) {
	sMap, err := New(TypeTransaction)
	if err != nil {
		b.Fatalf("Failed to create SHAMap: %v", err)
	}

	keys := make([][32]byte, b.N)
	for i := 0; i < b.N; i++ {
		// Create pseudo-random keys
		copy(keys[i][:], fmt.Sprintf("%032d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := sMap.Put(keys[i], intToBytes(i)); err != nil {
			b.Fatalf("Failed to put item %d: %v", i, err)
		}
	}
}

func BenchmarkGet(b *testing.B) {
	sMap, err := New(TypeTransaction)
	if err != nil {
		b.Fatalf("Failed to create SHAMap: %v", err)
	}

	// Pre-populate the map
	keys := make([][32]byte, 1000)
	for i := 0; i < 1000; i++ {
		copy(keys[i][:], fmt.Sprintf("%032d", i))
		if err := sMap.Put(keys[i], intToBytes(i)); err != nil {
			b.Fatalf("Failed to put item %d: %v", i, err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := keys[i%1000]
		_, _, err := sMap.Get(key)
		if err != nil {
			b.Fatalf("Failed to get item: %v", err)
		}
	}
}

func BenchmarkSnapshot(b *testing.B) {
	sMap, err := New(TypeTransaction)
	if err != nil {
		b.Fatalf("Failed to create SHAMap: %v", err)
	}

	// Pre-populate the map
	for i := 0; i < 1000; i++ {
		key := [32]byte{}
		copy(key[:], fmt.Sprintf("%032d", i))
		if err := sMap.Put(key, intToBytes(i)); err != nil {
			b.Fatalf("Failed to put item %d: %v", i, err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := sMap.Snapshot(false)
		if err != nil {
			b.Fatalf("Failed to create snapshot: %v", err)
		}
	}
}

// Helper function for debugging - simplified tree dump
func dumpTree(node Node, prefix string, isTail bool) {
	switch n := node.(type) {
	case *InnerNode:
		fmt.Printf("%s%sInnerNode %p, hash: %x\n", prefix, branchSymbol(isTail), n, n.Hash())

		// Get all non-empty children
		var children []struct {
			index int
			child Node
		}
		for i := 0; i < BranchFactor; i++ {
			if !n.IsEmptyBranch(i) {
				if child, err := n.Child(i); err == nil && child != nil {
					children = append(children, struct {
						index int
						child Node
					}{index: i, child: child})
				}
			}
		}

		for i, c := range children {
			fmt.Printf("%s%s[Branch %x]\n", prefix, pipeSymbol(isTail), c.index)
			dumpTree(c.child, nextPrefix(prefix, isTail), i == len(children)-1)
		}

	case *AccountStateLeafNode:
		fmt.Printf("%s%sLeaf(Account) %p, key: %x\n", prefix, branchSymbol(isTail), n, n.Item().Key())
	case *TransactionLeafNode:
		fmt.Printf("%s%sLeaf(Tx) %p, key: %x\n", prefix, branchSymbol(isTail), n, n.Item().Key())
	case *TransactionWithMetaLeafNode:
		fmt.Printf("%s%sLeaf(Tx+Meta) %p, key: %x\n", prefix, branchSymbol(isTail), n, n.Item().Key())
	default:
		fmt.Printf("%s%sUnknown node type: %T\n", prefix, branchSymbol(isTail), n)
	}
}

func branchSymbol(isTail bool) string {
	if isTail {
		return "└── "
	}
	return "├── "
}

func pipeSymbol(isTail bool) string {
	if isTail {
		return "    "
	}
	return "│   "
}

func nextPrefix(current string, isTail bool) string {
	if isTail {
		return current + "    "
	}
	return current + "│   "
}

// TestProofPath tests Merkle proof generation and verification - matches rippled SHAMapPathProof_test
func TestProofPath(t *testing.T) {
	sMap, err := New(TypeState)
	if err != nil {
		t.Fatalf("Failed to create SHAMap: %v", err)
	}

	var key [32]byte
	var rootHash [32]byte
	var goodPath [][]byte

	// Add items 1-99, same as rippled test
	for c := byte(1); c < 100; c++ {
		var k [32]byte
		k[0] = c // Create key with first byte = c, rest zeros (matches rippled's uint256(c))

		// Create data as the key bytes (matches rippled's Slice{k.data(), k.size()})
		data := make([]byte, 32)
		copy(data, k[:])

		// Add item to map
		if err := sMap.Put(k, data); err != nil {
			t.Fatalf("Failed to add item %d: %v", c, err)
		}

		// Get root hash
		root, err := sMap.Hash()
		if err != nil {
			t.Fatalf("Failed to get root hash: %v", err)
		}

		// Get proof path
		proofPath, err := sMap.GetProofPath(k)
		if err != nil {
			t.Fatalf("Failed to get proof path for item %d: %v", c, err)
		}
		if proofPath == nil || !proofPath.Found {
			t.Fatalf("Got nil or unfound proof path for item %d", c)
		}

		// Verify proof path using existing VerifyProofPath function
		if err := VerifyProofPath(root, k, proofPath.Path); err != nil {
			t.Fatalf("Failed to verify proof path for item %d: %v", c, err)
		}

		if c == 1 {
			// Test extra node (should fail)
			extraPath := make([][]byte, len(proofPath.Path)+1)
			extraPath[0] = proofPath.Path[0] // Duplicate first node
			copy(extraPath[1:], proofPath.Path)

			if VerifyProofPath(root, k, extraPath) == nil {
				t.Error("Proof verification should fail with extra node")
			}

			// Test wrong key (should return unfound proof)
			wrongKey := [32]byte{}
			wrongKey[0] = c + 1
			wrongProofPath, err := sMap.GetProofPath(wrongKey)
			if err != nil {
				t.Errorf("Error getting proof for non-existent key: %v", err)
			} else if wrongProofPath != nil && wrongProofPath.Found {
				t.Error("Should not find proof path for non-existent key")
			}
		}

		if c == 99 {
			// Save for later tests
			key = k
			rootHash = root
			goodPath = proofPath.Path
		}
	}

	// Test that good path is still valid
	if VerifyProofPath(rootHash, key, goodPath) != nil {
		t.Error("Good path should still be valid")
	}

	// Test empty path (should fail)
	emptyPath := [][]byte{}
	if VerifyProofPath(rootHash, key, emptyPath) == nil {
		t.Error("Empty path should fail verification")
	}

	// Test path too long (should fail)
	tooLongPath := make([][]byte, len(goodPath)+1)
	copy(tooLongPath, goodPath)
	tooLongPath[len(goodPath)] = goodPath[len(goodPath)-1] // Duplicate last node

	if VerifyProofPath(rootHash, key, tooLongPath) == nil {
		t.Error("Path that's too long should fail verification")
	}

	// Test bad node data (should fail)
	if len(goodPath) > 0 {
		badDataPath := make([][]byte, 1)
		badDataPath[0] = make([]byte, 100) // Fill with invalid data
		for i := range badDataPath[0] {
			badDataPath[0][i] = 100
		}

		if VerifyProofPath(rootHash, key, badDataPath) == nil {
			t.Error("Bad node data should fail verification")
		}
	}

	// Test bad node type (should fail)
	if len(goodPath) > 0 {
		badTypePath := make([][]byte, 1)
		badTypePath[0] = make([]byte, len(goodPath[0]))
		copy(badTypePath[0], goodPath[0])
		// Change node type (flip the last bit)
		if len(badTypePath[0]) > 0 {
			badTypePath[0][len(badTypePath[0])-1]--
		}

		if VerifyProofPath(rootHash, key, badTypePath) == nil {
			t.Error("Bad node type should fail verification")
		}
	}

	// Test all inner nodes (missing leaf, should fail)
	if len(goodPath) > 1 {
		allInnerPath := goodPath[1:] // Remove first node (should be leaf)

		if VerifyProofPath(rootHash, key, allInnerPath) == nil {
			t.Error("Path with all inner nodes should fail verification")
		}
	}
}
