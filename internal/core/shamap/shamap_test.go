package shamap

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// Helper functions for testing

// parseHex converts a hex string to a byte slice
func parseHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// hexToKey converts a hex string to a 32-byte array
func hexToKey(s string) [32]byte {
	var key [32]byte
	b := parseHex(s)
	copy(key[:], b)
	return key
}

// IntToVUC creates a byte slice filled with a specific value
func IntToVUC(v int) []byte {
	vuc := make([]byte, 32)
	for i := range vuc {
		vuc[i] = byte(v)
	}
	return vuc
}

// SHAMapItem definition for testing - assuming it's defined in your implementation
type SHAMapItem struct {
	Key  [32]byte
	Data []byte
}

// Constants for testing - adapt these to match your implementation's constants
const (
	FREE Type = iota
	TRANSACTION_NM
	ACCOUNT_STATE
)

// Test functions
func TestSHAMapAddTraverse(t *testing.T) {
	testCases := []struct {
		name   string
		backed bool
	}{
		{"add/traverse backed", true},
		{"add/traverse unbacked", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Initialize the SHAMap
			sMap := NewSHAMap(FREE)

			if !tc.backed {
				// Implement setUnbacked method in your SHAMap implementation
				// sMap.setUnbacked()
			}

			// Define test keys
			h1 := hexToKey("092891fe4ef6cee585fdc6fda0e09eb4d386363158ec3321b8123e5a772c6ca7")
			h2 := hexToKey("436ccbac3347baa1f1e53baeef1f43334da88f1f6d70d963b833afd6dfa289fe")
			h3 := hexToKey("b92891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8")
			h4 := hexToKey("b92891fe4ef6cee585fdc6fda2e09eb4d386363158ec3321b8123e5a772c6ca8")

			// Create items
			i1 := &SHAMapItem{Key: h1, Data: IntToVUC(1)}
			i2 := &SHAMapItem{Key: h2, Data: IntToVUC(2)}
			i3 := &SHAMapItem{Key: h3, Data: IntToVUC(3)}
			i4 := &SHAMapItem{Key: h4, Data: IntToVUC(4)}

			// Add items and verify
			t.Log("Adding items to map")
			if !sMap.AddItem(TRANSACTION_NM, i2) {
				t.Fatal("Failed to add item i2")
			}
			// sMap.Invariants()

			if !sMap.AddItem(TRANSACTION_NM, i1) {
				t.Fatal("Failed to add item i1")
			}
			// sMap.Invariants()

			// Implement traversal verification once you have iterator methods
			t.Log("Verifying map traversal")
			// iter := sMap.Begin()
			// end := sMap.End()
			//
			// if iter == end || !bytes.Equal(iter.Key(), i1.Key[:]) {
			//     t.Fatal("bad traverse: expected i1")
			// }
			//
			// iter.Next()
			// if iter == end || !bytes.Equal(iter.Key(), i2.Key[:]) {
			//     t.Fatal("bad traverse: expected i2")
			// }
			//
			// iter.Next()
			// if iter != end {
			//     t.Fatal("bad traverse: expected end")
			// }

			// Add more items and modify the map
			t.Log("Modifying map")
			sMap.AddItem(TRANSACTION_NM, i4)
			// sMap.Invariants()

			sMap.DelItem(i2.Key)
			// sMap.Invariants()

			sMap.AddItem(TRANSACTION_NM, i3)
			// sMap.Invariants()

			// Implement verification of final state once you have iterator methods
			t.Log("Test passed")
		})
	}
}

func TestSHAMapSnapshot(t *testing.T) {
	testCases := []struct {
		name   string
		backed bool
	}{
		{"snapshot backed", true},
		{"snapshot unbacked", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sMap := NewSHAMap(FREE)

			if !tc.backed {
				// sMap.setUnbacked()
			}

			// Define test keys
			h1 := hexToKey("092891fe4ef6cee585fdc6fda0e09eb4d386363158ec3321b8123e5a772c6ca7")
			h2 := hexToKey("436ccbac3347baa1f1e53baeef1f43334da88f1f6d70d963b833afd6dfa289fe")
			h3 := hexToKey("b92891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8")

			// Add items to map
			sMap.AddItem(TRANSACTION_NM, &SHAMapItem{Key: h1, Data: IntToVUC(1)})
			sMap.AddItem(TRANSACTION_NM, &SHAMapItem{Key: h2, Data: IntToVUC(2)})
			sMap.AddItem(TRANSACTION_NM, &SHAMapItem{Key: h3, Data: IntToVUC(3)})

			// Get hash before snapshot
			mapHash := sMap.RootHash()

			// Create snapshot - implement this method in your SHAMap
			t.Log("Creating snapshot")
			// map2 := sMap.SnapShot(false)
			// map2.Invariants()

			// Verify hashes match
			if !bytes.Equal(sMap.RootHash()[:], mapHash[:]) {
				t.Fatal("Original map hash changed unexpectedly")
			}

			// if !bytes.Equal(map2.RootHash()[:], mapHash[:]) {
			//     t.Fatal("Snapshot hash doesn't match original")
			// }

			// Modify original map
			t.Log("Modifying original map")
			// firstKey := h1 // In a real implementation, this would come from sMap.Begin().Key()
			if !sMap.DelItem(h1) {
				t.Fatal("Failed to delete item")
			}
			// sMap.Invariants()

			// Verify hashes differ now
			if bytes.Equal(sMap.RootHash()[:], mapHash[:]) {
				t.Fatal("Original map hash should change after deletion")
			}

			// if !bytes.Equal(map2.RootHash()[:], mapHash[:]) {
			//     t.Fatal("Snapshot hash changed unexpectedly")
			// }

			// Compare the maps once you implement the Compare method
			// delta := make(map[[32]byte]struct{
			//     First  *SHAMapItem
			//     Second *SHAMapItem
			// })
			// if !sMap.Compare(map2, delta, 100) {
			//     t.Fatal("comparison should succeed with differences")
			// }
			//
			// if len(delta) != 1 {
			//     t.Fatal("delta should contain one item")
			// }

			t.Log("Test passed")
		})
	}
}

func TestSHAMapBuildTear(t *testing.T) {
	testCases := []struct {
		name   string
		backed bool
	}{
		{"build/tear backed", true},
		{"build/tear unbacked", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sMap := NewSHAMap(FREE)

			if !tc.backed {
				// sMap.setUnbacked()
			}

			// Check initial hash is zero
			emptyHash := [32]byte{}
			if !bytes.Equal(sMap.RootHash()[:], emptyHash[:]) {
				t.Fatal("initial hash should be zero")
			}

			// Keys from C++ test
			keys := []string{
				"b92891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8",
				"b92881fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8",
				"b92691fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8",
				"b92791fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8",
				"b91891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8",
				"b99891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8",
				"f22891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8",
				"292891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8",
			}

			// Expected hashes after each addition - uncomment once you've implemented the hash computation logic
			// hashes := []string{
			//     "B7387CFEA0465759ADC718E8C42B52D2309D179B326E239EB5075C64B6281F7F",
			//     "FBC195A9592A54AB44010274163CB6BA95F497EC5BA0A883184546F7FB2ECE266",
			//     "4E7D2684B65DFD48937FFB775E20175C43AF0C94066F7D5679F51AE756795B75",
			//     "7A2F312EB203695FFD164E038E281839EEF06A1B99BFC263F3CECC6C74F93E07",
			//     "395A6691A372387A703FB0F2C6D2C405DAF307D0817F8F0E207596462B0E3A3E",
			//     "D044C0A696DE3169CC70AE216A1564D69DE96582865796142CE7D98A84D9DDE4",
			//     "76DCC77C4027309B5A91AD164083264D70B77B5E43E08AEDA5EBF94361143615",
			//     "DF4220E93ADC6F5569063A01B4DC79F8DB9553B6A3222ADE23DEA02BBE7230E5",
			// }

			// Add items
			t.Log("Adding items to map")
			for k, keyStr := range keys {
				key := hexToKey(keyStr)
				if !sMap.AddItem(TRANSACTION_NM, &SHAMapItem{Key: key, Data: IntToVUC(k)}) {
					t.Fatalf("Failed to add item %d", k)
				}
				// sMap.Invariants()

				// Verify hashes if you have the hash logic implemented
				// expectedHash := hexToKey(hashes[k])
				// if !bytes.Equal(sMap.RootHash()[:], expectedHash[:]) {
				//     t.Fatalf("hash mismatch after adding item %d, expected %x, got %x",
				//         k, expectedHash, sMap.RootHash())
				// }
			}

			// Store the final hash
			finalHash := sMap.RootHash()

			// Remove items in reverse order
			t.Log("Removing items from map")
			for k := len(keys) - 1; k >= 0; k-- {
				key := hexToKey(keys[k])
				if !sMap.DelItem(key) {
					t.Fatalf("Failed to delete item %d", k)
				}
				// sMap.Invariants()

				// Verify intermediate hashes if you have the hash logic implemented
				// if k > 0 {
				//     expectedHash := hexToKey(hashes[k-1])
				//     if !bytes.Equal(sMap.RootHash()[:], expectedHash[:]) {
				//         t.Fatalf("hash mismatch after removing item %d", k)
				//     }
				// }
			}

			// Hash should be zero again
			if !bytes.Equal(sMap.RootHash()[:], emptyHash[:]) {
				t.Fatal("final hash should be zero")
			}

			t.Log("Test passed")
		})
	}
}

func TestSHAMapIterate(t *testing.T) {
	testCases := []struct {
		name   string
		backed bool
	}{
		{"iterate backed", true},
		{"iterate unbacked", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sMap := NewSHAMap(FREE)

			if !tc.backed {
				// sMap.setUnbacked()
			}

			// Keys from C++ test
			keys := []string{
				"f22891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8",
				"b99891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8",
				"b92891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8",
				"b92881fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8",
				"b92791fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8",
				"b92691fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8",
				"b91891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8",
				"292891fe4ef6cee585fdc6fda1e09eb4d386363158ec3321b8123e5a772c6ca8",
			}

			// Add items
			t.Log("Adding items to map")
			for _, keyStr := range keys {
				key := hexToKey(keyStr)
				sMap.AddItem(TRANSACTION_NM, &SHAMapItem{Key: key, Data: IntToVUC(0)})
				// sMap.Invariants()
			}

			// Verify iteration order when you implement iterator methods
			// h := 7
			// for iter := sMap.Begin(); iter != sMap.End(); iter.Next() {
			//     expectedKey := hexToKey(keys[h])
			//     if !bytes.Equal(iter.Key(), expectedKey[:]) {
			//         t.Fatalf("iteration order mismatch at position %d", h)
			//     }
			//     h--
			// }

			t.Log("Test passed")
		})
	}
}

func TestSHAMapPathProof(t *testing.T) {
	sMap := NewSHAMap(FREE)
	// sMap.setUnbacked()

	t.Log("Testing path proofs")

	// Create and verify paths - once you implement GetProofPath and VerifyProofPath
	for c := byte(1); c < 100; c++ {
		var key [32]byte
		key[0] = c

		sMap.AddItem(ACCOUNT_STATE, &SHAMapItem{Key: key, Data: key[:]})
		// sMap.Invariants()

		// rootHash := sMap.RootHash()
		// path, exists := sMap.GetProofPath(key)
		//
		// if !exists {
		//     t.Fatalf("Failed to get proof path for key %d", c)
		// }
		//
		// if !sMap.VerifyProofPath(rootHash, key, path) {
		//     t.Fatalf("Failed to verify proof path for key %d", c)
		// }
		//
		// // Special cases for first and last items
		// if c == 1 {
		//     // Test invalid path (extra node)
		//     invalidPath := append([][]byte{path[0]}, path...)
		//     if sMap.VerifyProofPath(rootHash, key, invalidPath) {
		//         t.Fatal("Should not verify invalid path with extra node")
		//     }
		//
		//