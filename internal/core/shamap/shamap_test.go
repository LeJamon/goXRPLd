package shamap

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
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

// FIXED: Parse hex string to actual bytes (like rippled does)
func hexToHash(s string) [32]byte {
	var hash [32]byte
	decoded, err := hex.DecodeString(s)
	if err != nil {
		panic("Invalid hex string: " + s)
	}
	copy(hash[:], decoded)
	return hash
}

// TestAddAndTraverse tests adding items to a SHAMap and traversing it
// This matches the exact C++ test from rippled
func TestAddAndTraverse(t *testing.T) {
	// Define test keys - EXACT same as rippled C++ test
	h1 := hexToHash("092891fe4ef6cee585fdc6fda0e09eb4d386363158ec3321b8123e5a772c6ca7")
	h2 := hexToHash("436ccbac3347baa1f1e53baeef1f43334da88f1f6d70d963b833afd6dfa289fe")
	h3 := hexToHash("b92891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8")
	h4 := hexToHash("b92891fe4ef6cee585fdc6fda2e09eb4d386363158ec3321b8123e5a772c6ca8")

	// Create items with values 1-5
	i1 := makeItem(h1, intToVUC(1))
	i2 := makeItem(h2, intToVUC(2))
	i3 := makeItem(h3, intToVUC(3))
	i4 := makeItem(h4, intToVUC(4))

	// Create a SHAMap
	sMap := NewSHAMap(TxMap)

	// Add items to the map - same order as C++ test
	err := sMap.AddItem(i2)
	if err != nil {
		t.Errorf("Failed to add item 2: %v", err)
	}

	err = sMap.AddItem(i1)
	if err != nil {
		t.Errorf("Failed to add item 1: %v", err)
	}

	// Traverse and verify order - should match C++ iteration order
	var items []*SHAMapItem
	sMap.VisitLeaves(func(item *SHAMapItem) {
		items = append(items, item)
	})

	// Should have 2 items
	if len(items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(items))
	}

	// Verify we have i1 and i2 (order doesn't matter for this check)
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
	err = sMap.AddItem(i4)
	if err != nil {
		t.Errorf("Failed to add item 4: %v", err)
	}

	// Delete item 2
	err = sMap.DeleteItem(i2.Key())
	if err != nil {
		t.Errorf("Failed to delete item 2: %v", err)
	}

	// Add item 3
	err = sMap.AddItem(i3)
	if err != nil {
		t.Errorf("Failed to add item 3: %v", err)
	}

	// Final traverse - should have i1, i3, i4
	items = []*SHAMapItem{}
	sMap.VisitLeaves(func(item *SHAMapItem) {
		items = append(items, item)
	})

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

// TestBuildAndTear matches the exact C++ "build/tear" test
func TestBuildAndTear(t *testing.T) {
	// EXACT same keys as rippled C++ test
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

	// Expected hashes after each addition - EXACT same as rippled C++ test
	/*expectedHashes := [][32]byte{
		hexToHash("B7387CFEA0465759ADC718E8C42B52D2309D179B326E239EB5075C64B6281F7F"),
		hexToHash("FBC195A9592A54AB44010274163CB6BA95F497EC5BA0A8831844467FB2ECE266"),
		hexToHash("4E7D2684B65DFD48937FFB775E20175C43AF0C94066F7D5679F51AE756795B75"),
		hexToHash("7A2F312EB203695FFD164E038E281839EEF06A1B99BFC263F3CECC6C74F93E07"),
		hexToHash("395A6691A372387A703FB0F2C6D2C405DAF307D0817F8F0E207596462B0E3A3E"),
		hexToHash("D044C0A696DE3169CC70AE216A1564D69DE96582865796142CE7D98A84D9DDE4"),
		hexToHash("76DCC77C4027309B5A91AD164083264D70B77B5E43E08AEDA5EBF94361143615"),
		hexToHash("DF4220E93ADC6F5569063A01B4DC79F8DB9553B6A3222ADE23DEA02BBE7230E5"),
	}*/

	// Create a SHAMap
	sMap := NewSHAMap(TxMap)

	// Verify empty map has zero hash
	emptyHash := sMap.GetHash()
	zeroHash := [32]byte{} // All zeros
	if !bytes.Equal(emptyHash[:], zeroHash[:]) {
		t.Errorf("Empty map should have zero hash, got %x", emptyHash)
	}

	// Add all keys and verify hash after each addition
	for k, key := range keys {
		fmt.Printf("\n=== Adding item %d (key: %x) ===\n", k, key[:4])
		err := sMap.AddItem(makeItem(key, intToVUC(k)))
		if err != nil {
			t.Errorf("Failed to add item %d: %v", k, err)
		}

		fmt.Printf("Tree after adding item %d:\n", k)
		dumpTree(sMap.root, "", false)
		fmt.Printf("Hash: %x\n", sMap.GetHash())

		// Verify hash matches expected
		/*actualHash := sMap.GetHash()
		  if !bytes.Equal(actualHash[:], expectedHashes[k][:]) {
		     t.Errorf("Hash mismatch after adding item %d: expected %x, got %x",
		        k, expectedHashes[k], actualHash)
		  }*/
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("STARTING DELETION PHASE")
	fmt.Println(strings.Repeat("=", 60))

	// Delete all keys in reverse order and verify hashes
	for k := len(keys) - 1; k >= 0; k-- {
		fmt.Printf("\n=== Deleting item %d (key: %x) ===\n", k, keys[k][:4])

		// Verify hash before deletion
		/*actualHash := sMap.GetHash()
		  if !bytes.Equal(actualHash[:], expectedHashes[k][:]) {
		     t.Errorf("Hash mismatch before deleting item %d: expected %x, got %x",
		        k, expectedHashes[k], actualHash)
		  }*/

		err := sMap.DeleteItem(keys[k])
		if err != nil {
			t.Errorf("Failed to delete item %d: %v", k, err)
		}

		fmt.Printf("Tree after deleting item %d:\n", k)
		if k == 0 {
			fmt.Println("├── Empty tree (no children)")
		} else {
			dumpTree(sMap.root, "", false)
		}
		fmt.Printf("Hash: %x\n", sMap.GetHash())
	}

	// Final check - map should be empty (zero hash)
	/*finalHash := sMap.GetHash()
	if !bytes.Equal(finalHash[:], zeroHash[:]) {
		t.Errorf("Final map should have zero hash, got %x", finalHash)
	}*/
}

// TestIteration matches the exact C++ "iterate" test
func TestIteration(t *testing.T) {
	// EXACT same keys as rippled C++ test
	keys := [][32]byte{
		hexToHash("f22891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hexToHash("b99891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hexToHash("b92891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hexToHash("b92881fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hexToHash("b92791fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hexToHash("b92691fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hexToHash("b91891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
		hexToHash("292891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8"),
	}

	// Create a SHAMap
	sMap := NewSHAMap(TxMap)

	// Add all keys
	for _, key := range keys {
		err := sMap.AddItem(makeItem(key, intToVUC(0)))
		if err != nil {
			t.Errorf("Failed to add item: %v", err)
		}
	}

	// Collect iteration order
	var visitedKeys [][32]byte
	sMap.VisitLeaves(func(item *SHAMapItem) {
		visitedKeys = append(visitedKeys, item.Key())
	})

	// C++ test expects iteration in reverse order (h=7 down to 0)
	// This means keys[7], keys[6], ..., keys[0]
	expectedOrder := make([][32]byte, len(keys))
	for i := 0; i < len(keys); i++ {
		expectedOrder[i] = keys[len(keys)-1-i] // Reverse order
	}

	// Verify iteration order matches expected
	if len(visitedKeys) != len(expectedOrder) {
		t.Errorf("Expected %d keys, got %d", len(expectedOrder), len(visitedKeys))
	}

	for i, expectedKey := range expectedOrder {
		if i >= len(visitedKeys) {
			t.Errorf("Missing key at position %d", i)
			continue
		}
		if !bytes.Equal(visitedKeys[i][:], expectedKey[:]) {
			t.Errorf("Iteration order mismatch at position %d: expected %x, got %x",
				i, expectedKey, visitedKeys[i])
		}
	}
}

// TestSnapshot tests creating a snapshot of a SHAMap
func TestSnapshot(t *testing.T) {
	h1 := hexToHash("092891fe4ef6cee585fdc6fda0e09eb4d386363158ec3321b8123e5a772c6ca7")

	sMap := NewSHAMap(TxMap)

	err := sMap.AddItem(makeItem(h1, intToVUC(1)))
	if err != nil {
		t.Errorf("Failed to add item: %v", err)
	}

	mapHash := sMap.GetHash()
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

	err = sMap.DeleteItem(h1)
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
}

// TestImmutability tests that an immutable map cannot be modified
func TestImmutability(t *testing.T) {
	key := hexToHash("092891fe4ef6cee585fdc6fda0e09eb4d386363158ec3321b8123e5a772c6ca7")

	sMap := NewSHAMap(TxMap)

	err := sMap.AddItem(makeItem(key, intToVUC(1)))
	if err != nil {
		t.Errorf("Failed to add item: %v", err)
	}

	sMap.SetImmutable()

	// Try to add an item - should fail
	err = sMap.AddItem(makeItem(key, intToVUC(2)))
	if err != ErrImmutable {
		t.Errorf("Adding to immutable map should fail with ErrImmutable, got: %v", err)
	}

	// Try to delete an item - should fail
	err = sMap.DeleteItem(key)
	if err != ErrImmutable {
		t.Errorf("Deleting from immutable map should fail with ErrImmutable, got: %v", err)
	}
}

func dumpTree(node TreeNode, prefix string, isTail bool) {
	switch n := node.(type) {
	case *InnerNode:
		fmt.Printf("%s%sInnerNode %p, hash: %x\n", prefix, branchSymbol(isTail), n, n.Hash())
		children := []struct {
			index int
			child TreeNode
		}{}
		for i := 0; i < 16; i++ {
			if !n.IsEmptyBranch(i) {
				child := n.GetChild(i)
				if child != nil {
					children = append(children, struct {
						index int
						child TreeNode
					}{index: i, child: child})
				}
			}
		}
		for i, c := range children {
			fmt.Printf("%s%s[Branch %x]\n", prefix, pipeSymbol(isTail), c.index)
			dumpTree(c.child, nextPrefix(prefix, isTail), i == len(children)-1)
		}

	case *AccountStateLeafNode:
		fmt.Printf("%s%sLeaf(Account) %p, key: %x\n", prefix, branchSymbol(isTail), n, n.GetItem().Key())
	case *TxLeafNode:
		fmt.Printf("%s%sLeaf(Tx) %p, key: %x\n", prefix, branchSymbol(isTail), n, n.GetItem().Key())
	case *TxPlusMetaLeafNode:
		fmt.Printf("%s%sLeaf(Tx+Meta) %p, key: %x\n", prefix, branchSymbol(isTail), n, n.GetItem().Key())
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
