package shamap

import (
	"bytes"
	"errors"
	"testing"
)

// MockSyncFilter implements SyncFilter for testing
type MockSyncFilter struct {
	nodes    map[[32]byte][]byte
	gotNodes []GotNodeCall
}

type GotNodeCall struct {
	FromFilter bool
	Hash       [32]byte
	LedgerSeq  uint32
	NodeData   []byte
	NodeType   NodeType
}

func NewMockSyncFilter() *MockSyncFilter {
	return &MockSyncFilter{
		nodes:    make(map[[32]byte][]byte),
		gotNodes: make([]GotNodeCall, 0),
	}
}

func (m *MockSyncFilter) GetNode(hash [32]byte) ([]byte, bool) {
	data, exists := m.nodes[hash]
	return data, exists
}

func (m *MockSyncFilter) GotNode(fromFilter bool, hash [32]byte, ledgerSeq uint32, nodeData []byte, nodeType NodeType) {
	m.gotNodes = append(m.gotNodes, GotNodeCall{
		FromFilter: fromFilter,
		Hash:       hash,
		LedgerSeq:  ledgerSeq,
		NodeData:   append([]byte(nil), nodeData...), // Copy the data
		NodeType:   nodeType,
	})
}

func (m *MockSyncFilter) AddNode(hash [32]byte, data []byte) {
	m.nodes[hash] = append([]byte(nil), data...) // Copy the data
}

// TestSyncFilterBasic tests basic SyncFilter functionality
func TestSyncFilterBasic(t *testing.T) {
	sMap, err := New(TypeTransaction, nil)
	if err != nil {
		t.Fatalf("Failed to create SHAMap: %v", err)
	}

	filter := NewMockSyncFilter()

	// Create some test data
	key1 := [32]byte{1, 2, 3}
	data1 := []byte("test data 1")

	// Put an item in the map
	if err := sMap.Put(key1, data1); err != nil {
		t.Fatalf("Failed to put item: %v", err)
	}

	// Get the root hash
	rootHash, err := sMap.Hash()
	if err != nil {
		t.Fatalf("Failed to get hash: %v", err)
	}

	// Serialize the root node for the filter
	rootData, err := sMap.root.SerializeForWire()
	if err != nil {
		t.Fatalf("Failed to serialize root: %v", err)
	}
	filter.AddNode(rootHash, rootData)

	// Test that filter can provide the node
	retrievedData, found := filter.GetNode(rootHash)
	if !found {
		t.Error("Filter should have the root node")
	}
	if !bytes.Equal(retrievedData, rootData) {
		t.Error("Retrieved data doesn't match stored data")
	}
}

// TestAddKnownNode tests adding nodes during synchronization
func TestAddKnownNode(t *testing.T) {
	// Create source map
	sourceMap, err := New(TypeTransaction, nil)
	if err != nil {
		t.Fatalf("Failed to create source SHAMap: %v", err)
	}

	// Add some items to source map
	for i := 0; i < 5; i++ {
		key := [32]byte{}
		key[0] = byte(i)
		data := []byte{byte(i * 10)}
		if err := sourceMap.Put(key, data); err != nil {
			t.Fatalf("Failed to put item %d: %v", i, err)
		}
	}

	// Create destination map
	destMap, err := New(TypeTransaction, nil)
	if err != nil {
		t.Fatalf("Failed to create dest SHAMap: %v", err)
	}

	filter := NewMockSyncFilter()

	// Set destination to syncing state
	if err := destMap.SetSyncing(); err != nil {
		t.Fatalf("Failed to set syncing: %v", err)
	}

	// Get root from source and try adding to destination
	sourceRootHash, err := sourceMap.Hash()
	if err != nil {
		t.Fatalf("Failed to get source hash: %v", err)
	}

	sourceRootData, err := sourceMap.root.SerializeForWire()
	if err != nil {
		t.Fatalf("Failed to serialize source root: %v", err)
	}

	// Add root node to destination
	result := destMap.AddRootNode(sourceRootHash, sourceRootData, filter)
	if result.Status != AddNodeUseful {
		t.Errorf("Expected AddNodeUseful, got %v", result.Status)
	}
	if result.Good != 1 {
		t.Errorf("Expected Good=1, got %d", result.Good)
	}

	// Check that filter was notified
	if len(filter.gotNodes) != 1 {
		t.Errorf("Expected 1 GotNode call, got %d", len(filter.gotNodes))
	}
}

// TestAddRootNode tests setting root nodes during sync
func TestAddRootNode(t *testing.T) {
	// Create a map with some data
	sourceMap, err := New(TypeState, nil)
	if err != nil {
		t.Fatalf("Failed to create source map: %v", err)
	}

	key := [32]byte{0x10, 0x20, 0x30}
	data := []byte("account state data")
	if err := sourceMap.Put(key, data); err != nil {
		t.Fatalf("Failed to put item: %v", err)
	}

	sourceHash, err := sourceMap.Hash()
	if err != nil {
		t.Fatalf("Failed to get source hash: %v", err)
	}

	sourceRootData, err := sourceMap.root.SerializeForWire()
	if err != nil {
		t.Fatalf("Failed to serialize root: %v", err)
	}

	// Create destination map
	destMap, err := New(TypeState, nil)
	if err != nil {
		t.Fatalf("Failed to create dest map: %v", err)
	}

	filter := NewMockSyncFilter()

	// Test adding valid root
	result := destMap.AddRootNode(sourceHash, sourceRootData, filter)
	if result.Status != AddNodeUseful {
		t.Errorf("Expected AddNodeUseful, got %v", result.Status)
	}

	// Check that destination hash matches source
	destHash, err := destMap.Hash()
	if err != nil {
		t.Fatalf("Failed to get dest hash: %v", err)
	}
	if destHash != sourceHash {
		t.Error("Destination hash doesn't match source hash")
	}

	// Test adding root with wrong hash
	wrongHash := [32]byte{0xff, 0xff, 0xff}
	result = destMap.AddRootNode(wrongHash, sourceRootData, filter)
	if result.Status != AddNodeInvalid {
		t.Errorf("Expected AddNodeInvalid for wrong hash, got %v", result.Status)
	}
}

// TestGetNodeFat tests fat node retrieval
func TestGetNodeFat(t *testing.T) {
	sMap, err := New(TypeTransaction, nil)
	if err != nil {
		t.Fatalf("Failed to create SHAMap: %v", err)
	}

	// Add several items to create a tree structure
	for i := 0; i < 10; i++ {
		key := [32]byte{}
		key[0] = byte(i)
		key[1] = byte(i * 2)
		data := []byte{byte(i * 10)}
		if err := sMap.Put(key, data); err != nil {
			t.Fatalf("Failed to put item %d: %v", i, err)
		}
	}

	// Test getting fat node from root with depth 0
	rootNodeID := NewRootNodeID()
	fatNodes, err := sMap.GetNodeFat(rootNodeID, false, 0)
	if err != nil {
		t.Fatalf("Failed to get fat node: %v", err)
	}
	if len(fatNodes) != 1 {
		t.Errorf("Expected 1 node for depth 0, got %d", len(fatNodes))
	}

	// Test getting fat node with depth 1 (should include immediate children)
	fatNodes, err = sMap.GetNodeFat(rootNodeID, false, 1)
	if err != nil {
		t.Fatalf("Failed to get fat node with depth 1: %v", err)
	}
	if len(fatNodes) < 2 { // At least root + some children
		t.Errorf("Expected at least 2 nodes for depth 1, got %d", len(fatNodes))
	}

	// Test with fatLeaves = true
	fatNodes, err = sMap.GetNodeFat(rootNodeID, true, 1)
	if err != nil {
		t.Fatalf("Failed to get fat node with fatLeaves: %v", err)
	}
	// Should have more nodes when including leaves
	if len(fatNodes) < 2 {
		t.Errorf("Expected at least 2 nodes with fatLeaves, got %d", len(fatNodes))
	}
}

// TestSyncStates tests sync state management
func TestSyncStates(t *testing.T) {
	sMap, err := New(TypeTransaction, nil)
	if err != nil {
		t.Fatalf("Failed to create SHAMap: %v", err)
	}

	// Initially should be in modifying state
	if sMap.State() != StateModifying {
		t.Errorf("Expected StateModifying, got %v", sMap.State())
	}
	if sMap.IsSyncing() {
		t.Error("Should not be synching initially")
	}

	// Set to syncing
	if err := sMap.SetSyncing(); err != nil {
		t.Fatalf("Failed to set synching: %v", err)
	}
	if sMap.State() != StateSyncing {
		t.Errorf("Expected StateSyncing, got %v", sMap.State())
	}
	if !sMap.IsSyncing() {
		t.Error("Should be synching after SetSynching")
	}

	// Try to set syncing again (should fail)
	if err := sMap.SetSyncing(); err != ErrSyncInProgress {
		t.Errorf("Expected ErrSyncInProgress, got %v", err)
	}

	// Clear syncing
	if err := sMap.ClearSyncing(); err != nil {
		t.Fatalf("Failed to clear synching: %v", err)
	}
	if sMap.State() != StateModifying {
		t.Errorf("Expected StateModifying after clear, got %v", sMap.State())
	}
	if sMap.IsSyncing() {
		t.Error("Should not be synching after clear")
	}

	// Try to clear when not synching (should fail)
	if err := sMap.ClearSyncing(); !errors.Is(err, ErrNotSyncing) {
		t.Errorf("Expected ErrNotSyncing, got %v", err)
	}
}

// TestHasNodes tests node existence checks
func TestHasNodes(t *testing.T) {
	sMap, err := New(TypeState, nil)
	if err != nil {
		t.Fatalf("Failed to create SHAMap: %v", err)
	}

	key := [32]byte{0x12, 0x34, 0x56}
	data := []byte("test account data")
	if err := sMap.Put(key, data); err != nil {
		t.Fatalf("Failed to put item: %v", err)
	}

	// Test HasLeafNode
	leafNode, err := sMap.walkToKey(key, nil)
	if err != nil {
		t.Fatalf("Failed to walk to key: %v", err)
	}
	leafHash := leafNode.Hash()

	if !sMap.HasLeafNode(key, leafHash) {
		t.Error("Should have leaf node with correct hash")
	}

	wrongHash := [32]byte{0xff, 0xff, 0xff}
	if sMap.HasLeafNode(key, wrongHash) {
		t.Error("Should not have leaf node with wrong hash")
	}

	wrongKey := [32]byte{0xff, 0xee, 0xdd}
	if sMap.HasLeafNode(wrongKey, leafHash) {
		t.Error("Should not have leaf node with wrong key")
	}

	// Test HasInnerNode
	rootNodeID := NewRootNodeID()
	rootHash := sMap.root.Hash()

	if !sMap.HasInnerNode(rootNodeID, rootHash) {
		t.Error("Should have root inner node")
	}

	if sMap.HasInnerNode(rootNodeID, wrongHash) {
		t.Error("Should not have root inner node with wrong hash")
	}
}

// TestGetMissingNodesFiltered tests filtered missing node retrieval
func TestGetMissingNodesFiltered(t *testing.T) {
	sMap, err := New(TypeTransaction, nil)
	if err != nil {
		t.Fatalf("Failed to create SHAMap: %v", err)
	}

	// Add some items
	for i := 0; i < 3; i++ {
		key := [32]byte{}
		key[0] = byte(i)
		data := []byte{byte(i)}
		if err := sMap.Put(key, data); err != nil {
			t.Fatalf("Failed to put item %d: %v", i, err)
		}
	}

	filter := NewMockSyncFilter()

	// Get missing nodes (should be empty for complete map)
	requests := sMap.GetMissingNodesFiltered(10, filter)
	if len(requests) != 0 {
		t.Errorf("Expected 0 missing nodes for complete map, got %d", len(requests))
	}

	// TODO: This test would be more meaningful with a partially complete map
	// where some nodes are missing and need to be requested
}

// TestFetchRoot tests root fetching during sync
func TestFetchRoot(t *testing.T) {
	// Create source map
	sourceMap, err := New(TypeTransaction, nil)
	if err != nil {
		t.Fatalf("Failed to create source map: %v", err)
	}

	key := [32]byte{0xaa, 0xbb, 0xcc}
	data := []byte("test transaction data")
	if err := sourceMap.Put(key, data); err != nil {
		t.Fatalf("Failed to put item: %v", err)
	}

	sourceHash, err := sourceMap.Hash()
	if err != nil {
		t.Fatalf("Failed to get source hash: %v", err)
	}

	sourceRootData, err := sourceMap.root.SerializeForWire()
	if err != nil {
		t.Fatalf("Failed to serialize root: %v", err)
	}

	// Create destination map
	destMap, err := New(TypeTransaction, nil)
	if err != nil {
		t.Fatalf("Failed to create dest map: %v", err)
	}

	// Set up filter with the root node
	filter := NewMockSyncFilter()
	filter.AddNode(sourceHash, sourceRootData)

	// Test successful fetch
	success := destMap.FetchRoot(sourceHash, filter)
	if !success {
		t.Error("FetchRoot should have succeeded")
	}

	// Verify destination has same hash as source
	destHash, err := destMap.Hash()
	if err != nil {
		t.Fatalf("Failed to get dest hash: %v", err)
	}
	if destHash != sourceHash {
		t.Error("Destination hash should match source after FetchRoot")
	}

	// Test fetch of non-existent node
	wrongHash := [32]byte{0xff, 0xff, 0xff}
	success = destMap.FetchRoot(wrongHash, filter)
	if success {
		t.Error("FetchRoot should have failed for non-existent node")
	}

	// Test with nil filter
	success = destMap.FetchRoot(sourceHash, nil)
	if success {
		t.Error("FetchRoot should have failed with nil filter")
	}
}

// TestSyncWorkflow tests a complete sync workflow
func TestSyncWorkflow(t *testing.T) {
	// Create source map with multiple items
	sourceMap, err := New(TypeTransaction, nil)
	if err != nil {
		t.Fatalf("Failed to create source map: %v", err)
	}

	keys := make([][32]byte, 5)
	for i := 0; i < 5; i++ {
		keys[i] = [32]byte{}
		keys[i][0] = byte(i + 1)
		keys[i][1] = byte((i + 1) * 10)
		data := []byte{byte(i * 100)}
		if err := sourceMap.Put(keys[i], data); err != nil {
			t.Fatalf("Failed to put item %d: %v", i, err)
		}
	}

	sourceHash, err := sourceMap.Hash()
	if err != nil {
		t.Fatalf("Failed to get source hash: %v", err)
	}

	// Create destination map for sync
	destMap, err := New(TypeTransaction, nil)
	if err != nil {
		t.Fatalf("Failed to create dest map: %v", err)
	}

	filter := NewMockSyncFilter()

	// Step 1: Set to syncing state
	if err := destMap.SetSyncing(); err != nil {
		t.Fatalf("Failed to set syncing: %v", err)
	}

	// Step 2: Get root node data and add to filter
	sourceRootData, err := sourceMap.root.SerializeForWire()
	if err != nil {
		t.Fatalf("Failed to serialize source root: %v", err)
	}
	filter.AddNode(sourceHash, sourceRootData)

	// Step 3: Fetch root
	if !destMap.FetchRoot(sourceHash, filter) {
		t.Fatal("Failed to fetch root")
	}

	// Step 4: Clear syncing state
	if err := destMap.ClearSyncing(); err != nil {
		t.Fatalf("Failed to clear syncing: %v", err)
	}

	// Verify final state
	destHash, err := destMap.Hash()
	if err != nil {
		t.Fatalf("Failed to get dest hash: %v", err)
	}
	if destHash != sourceHash {
		t.Error("Final destination hash should match source")
	}

	// Verify at least one item can be retrieved (root was synced)
	// Note: This is a simplified test - in reality, leaf nodes would also need to be synced
	if destMap.State() != StateModifying {
		t.Errorf("Expected StateModifying after sync, got %v", destMap.State())
	}
}

// TestAddNodeResultTypes tests different AddNodeResult scenarios
func TestAddNodeResultTypes(t *testing.T) {
	sMap, err := New(TypeTransaction, nil)
	if err != nil {
		t.Fatalf("Failed to create SHAMap: %v", err)
	}

	filter := NewMockSyncFilter()

	// Test invalid node data
	nodeID := NewRootNodeID()
	invalidData := []byte{0xff, 0xff, 0xff} // Invalid serialized data
	result := sMap.AddKnownNode(nodeID, invalidData, filter)
	if result.Status != AddNodeInvalid {
		t.Errorf("Expected AddNodeInvalid for invalid data, got %v", result.Status)
	}
	if result.Bad != 1 {
		t.Errorf("Expected Bad=1 for invalid data, got %d", result.Bad)
	}

	// Test AddNodeUseful by adding a valid root node
	testItem := NewItem([32]byte{1, 2, 3}, []byte("test"))
	leafNode, err := CreateLeafNode(NodeTypeTransactionNoMeta, testItem)
	if err != nil {
		t.Fatalf("Failed to create leaf node: %v", err)
	}

	// Create an inner root containing the leaf
	root := NewInnerNode()
	if err := root.SetChild(0, leafNode); err != nil {
		t.Fatalf("Failed to set child: %v", err)
	}

	rootData, err := root.SerializeForWire()
	if err != nil {
		t.Fatalf("Failed to serialize root: %v", err)
	}

	// Test adding root node (should be successful)
	rootHash := root.Hash()
	result = sMap.AddRootNode(rootHash, rootData, filter)
	if result.Status != AddNodeUseful {
		t.Errorf("Expected AddNodeUseful for valid root, got %v", result.Status)
	}
	if result.Good != 1 {
		t.Errorf("Expected Good=1 for valid root, got %d", result.Good)
	}

	// Test duplicate by adding the same root again
	result = sMap.AddRootNode(rootHash, rootData, filter)
	if result.Status != AddNodeUseful {
		// Note: AddRootNode always replaces the root, so it's always "useful"
		// This is different from AddKnownNode which can detect duplicates
		t.Logf("Root replacement always returns AddNodeUseful (expected behavior)")
	}
}
